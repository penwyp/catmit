package template

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
)

// 预定义错误
var (
	ErrTemplateNotFound = errors.New(
		errors.ErrTypeConfig,
		"PR模板未找到",
	).WithSuggestion("创建模板文件，如 .github/PULL_REQUEST_TEMPLATE.md")
	
	ErrTemplateReadError = errors.NewRetryable(
		errors.ErrTypeConfig,
		"读取模板文件失败",
	)
)

// templatePaths 定义各个provider的模板搜索路径
var templatePaths = map[string][]string{
	"github": {
		".github/PULL_REQUEST_TEMPLATE.md",
		".github/pull_request_template.md",
		".github/PULL_REQUEST_TEMPLATE/*.md",
		".github/pull_request_template/*.md",
		"PULL_REQUEST_TEMPLATE.md",
		"pull_request_template.md",
	},
	"gitlab": {
		".gitlab/merge_request_templates/*.md",
		".gitlab/merge_request_templates/Default.md",
		".gitlab/merge_request_templates/default.md",
	},
	"gitea": {
		".gitea/PULL_REQUEST_TEMPLATE.md",
		".gitea/pull_request_template.md",
		".gitea/PULL_REQUEST_TEMPLATE/*.md",
		".gitea/pull_request_template/*.md",
		"PULL_REQUEST_TEMPLATE.md",
		"pull_request_template.md",
	},
	"bitbucket": {
		".bitbucket/PULLREQUEST_TEMPLATE.md",
		".bitbucket/pullrequest_template.md",
		"PULLREQUEST_TEMPLATE.md",
		"pullrequest_template.md",
	},
}

// FileLoader 基于文件系统的模板加载器
type FileLoader struct {
	basePath string // 仓库根目录
	log      logger.Logger
}

// NewFileLoader 创建文件加载器
func NewFileLoader(basePath string) *FileLoader {
	return &FileLoader{
		basePath: basePath,
		log:      logger.NewDefault(),
	}
}

// Load 加载指定provider的模板
func (l *FileLoader) Load(ctx context.Context, provider string) (*Template, error) {
	l.log.Debugf("Loading template for provider: %s", provider)
	
	// 获取该provider的搜索路径
	paths, ok := templatePaths[strings.ToLower(provider)]
	if !ok {
		// 未知provider，尝试通用路径
		paths = templatePaths["github"]
	}
	
	// 遍历搜索路径
	for _, pathPattern := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		// 处理通配符路径
		if strings.Contains(pathPattern, "*") {
			templates, err := l.loadGlobTemplates(pathPattern)
			if err != nil {
				l.log.Debugf("Failed to load templates from %s: %v", pathPattern, err)
				continue
			}
			if len(templates) > 0 {
				// 返回第一个找到的模板（可以后续优化为选择default或让用户选择）
				return templates[0], nil
			}
		} else {
			// 单个文件路径
			tmpl, err := l.loadSingleTemplate(pathPattern)
			if err != nil {
				l.log.Debugf("Failed to load template from %s: %v", pathPattern, err)
				continue
			}
			return tmpl, nil
		}
	}
	
	return nil, ErrTemplateNotFound
}

// ListTemplates 列出所有可用模板
func (l *FileLoader) ListTemplates(ctx context.Context, provider string) ([]*Template, error) {
	l.log.Debugf("Listing templates for provider: %s", provider)
	
	var allTemplates []*Template
	
	paths, ok := templatePaths[strings.ToLower(provider)]
	if !ok {
		paths = templatePaths["github"]
	}
	
	for _, pathPattern := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		if strings.Contains(pathPattern, "*") {
			templates, err := l.loadGlobTemplates(pathPattern)
			if err != nil {
				continue
			}
			allTemplates = append(allTemplates, templates...)
		} else {
			tmpl, err := l.loadSingleTemplate(pathPattern)
			if err != nil {
				continue
			}
			allTemplates = append(allTemplates, tmpl)
		}
	}
	
	// 去重：基于文件路径去重（考虑大小写不敏感的文件系统）
	seen := make(map[string]bool)
	var uniqueTemplates []*Template
	for _, tmpl := range allTemplates {
		// 将路径转换为小写以处理大小写不敏感的文件系统
		normalizedPath := strings.ToLower(tmpl.Path)
		if !seen[normalizedPath] {
			seen[normalizedPath] = true
			uniqueTemplates = append(uniqueTemplates, tmpl)
		}
	}
	
	return uniqueTemplates, nil
}

// loadSingleTemplate 加载单个模板文件
func (l *FileLoader) loadSingleTemplate(relativePath string) (*Template, error) {
	fullPath := filepath.Join(l.basePath, relativePath)
	
	// 检查文件是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, errors.Wrapf(errors.ErrTypeConfig, "template file not found: %s", err, fullPath)
	}
	
	// 读取文件内容
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "读取模板文件失败", err)
	}
	
	// 创建模板对象
	tmpl := &Template{
		Path:    fullPath,
		Name:    extractTemplateName(relativePath),
		Content: string(content),
	}
	
	// 根据路径推断provider
	tmpl.Provider = inferProviderFromPath(relativePath)
	
	return tmpl, nil
}

// loadGlobTemplates 加载匹配通配符的模板文件
func (l *FileLoader) loadGlobTemplates(pattern string) ([]*Template, error) {
	fullPattern := filepath.Join(l.basePath, pattern)
	
	// 获取匹配的文件
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to match template files", err)
	}
	
	var templates []*Template
	
	for _, match := range matches {
		// 计算相对路径
		relativePath, err := filepath.Rel(l.basePath, match)
		if err != nil {
			continue
		}
		
		// 读取文件内容
		content, err := os.ReadFile(match)
		if err != nil {
			l.log.Debugf("Failed to read template %s: %v", match, err)
			continue
		}
		
		tmpl := &Template{
			Path:     match,
			Name:     extractTemplateName(relativePath),
			Content:  string(content),
			Provider: inferProviderFromPath(relativePath),
		}
		
		templates = append(templates, tmpl)
	}
	
	return templates, nil
}

// extractTemplateName 从文件路径提取模板名称
func extractTemplateName(path string) string {
	// 获取文件名（不含扩展名）
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	
	// 规范化名称
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.Title(strings.ToLower(name))
	
	// 如果是默认模板，返回"Default"
	if strings.ToLower(name) == "pull request template" || 
	   strings.ToLower(name) == "merge request template" ||
	   strings.ToLower(name) == "pullrequest template" {
		return "Default"
	}
	
	return name
}

// inferProviderFromPath 从路径推断provider类型
func inferProviderFromPath(path string) string {
	path = strings.ToLower(path)
	
	switch {
	case strings.Contains(path, ".github"):
		return "github"
	case strings.Contains(path, ".gitlab"):
		return "gitlab"
	case strings.Contains(path, ".gitea"):
		return "gitea"
	case strings.Contains(path, ".bitbucket"):
		return "bitbucket"
	default:
		// 根据文件名判断
		if strings.Contains(path, "merge_request") {
			return "gitlab"
		}
		if strings.Contains(path, "pullrequest") {
			return "bitbucket"
		}
		// 默认假定为GitHub
		return "github"
	}
}

// CachedLoader 带缓存的模板加载器
type CachedLoader struct {
	loader Loader
	cache  map[string]*Template
	log    logger.Logger
}

// NewCachedLoader 创建带缓存的加载器
func NewCachedLoader(loader Loader) *CachedLoader {
	return &CachedLoader{
		loader: loader,
		cache:  make(map[string]*Template),
		log:    logger.NewDefault(),
	}
}

// Load 加载模板（优先从缓存）
func (c *CachedLoader) Load(ctx context.Context, provider string) (*Template, error) {
	// 检查缓存
	cacheKey := fmt.Sprintf("default:%s", provider)
	if tmpl, ok := c.cache[cacheKey]; ok {
		c.log.Debugf("Loading template from cache for provider: %s", provider)
		return tmpl, nil
	}
	
	// 从底层加载器加载
	tmpl, err := c.loader.Load(ctx, provider)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "failed to load template from underlying loader", err)
	}
	
	// 存入缓存
	c.cache[cacheKey] = tmpl
	return tmpl, nil
}

// ListTemplates 列出所有可用模板
func (c *CachedLoader) ListTemplates(ctx context.Context, provider string) ([]*Template, error) {
	// 列表操作不使用缓存，总是获取最新
	return c.loader.ListTemplates(ctx, provider)
}

// ClearCache 清除缓存
func (c *CachedLoader) ClearCache() {
	c.cache = make(map[string]*Template)
}