package template

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/provider"
)

// DefaultManager 默认的模板管理器实现
type DefaultManager struct {
	loader    Loader
	parser    Parser
	processor Processor
	log       logger.Logger
}

// NewDefaultManager 创建默认管理器
func NewDefaultManager(basePath string) *DefaultManager {
	fileLoader := NewFileLoader(basePath)
	cachedLoader := NewCachedLoader(fileLoader)
	
	return &DefaultManager{
		loader:    cachedLoader,
		parser:    NewMarkdownParser(),
		processor: NewTemplateProcessor(),
		log:       logger.NewDefault(),
	}
}

// LoadTemplate 根据provider信息加载模板
func (m *DefaultManager) LoadTemplate(ctx context.Context, info *provider.RemoteInfo) (*Template, error) {
	m.log.Debugf("Loading template for provider: %s", info.Provider)
	
	// 加载原始模板
	tmpl, err := m.loader.Load(ctx, info.Provider)
	if err != nil {
		// 如果是模板未找到错误，尝试通用模板
		if errors.Is(err, ErrTemplateNotFound) && info.Provider != "github" {
			m.log.Debugf("Provider-specific template not found, trying generic template")
			tmpl, err = m.loader.Load(ctx, "github")
		}
		
		if err != nil {
			return nil, err
		}
	}
	
	// 解析模板结构
	parsed, err := m.parser.Parse(tmpl.Content)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeValidation, "模板解析失败", err)
	}
	
	// 合并解析结果
	tmpl.Sections = parsed.Sections
	tmpl.Variables = parsed.Variables
	tmpl.Provider = info.Provider
	
	return tmpl, nil
}

// ProcessTemplate 处理模板，填充变量
func (m *DefaultManager) ProcessTemplate(ctx context.Context, tmpl *Template, data *TemplateData) (string, error) {
	m.log.Debugf("Processing template with data")
	
	// 处理模板
	result, err := m.processor.Process(tmpl, data)
	if err != nil {
		return "", err
	}
	
	return result, nil
}

// ConfigurableManager 可配置的模板管理器
type ConfigurableManager struct {
	*DefaultManager
	config *ManagerConfig
}

// ManagerConfig 管理器配置
type ManagerConfig struct {
	// TemplateDirs 额外的模板搜索目录
	TemplateDirs []string
	
	// DefaultProvider 默认的provider类型
	DefaultProvider string
	
	// StrictMode 严格模式，必填字段缺失时报错
	StrictMode bool
	
	// CustomFunctions 自定义模板函数
	CustomFunctions map[string]interface{}
}

// NewConfigurableManager 创建可配置的管理器
func NewConfigurableManager(basePath string, config *ManagerConfig) *ConfigurableManager {
	if config == nil {
		config = &ManagerConfig{
			DefaultProvider: "github",
			StrictMode:      false,
		}
	}
	
	return &ConfigurableManager{
		DefaultManager: NewDefaultManager(basePath),
		config:         config,
	}
}

// LoadTemplate 加载模板（支持自定义目录）
func (m *ConfigurableManager) LoadTemplate(ctx context.Context, info *provider.RemoteInfo) (*Template, error) {
	// 首先尝试从自定义目录加载
	for _, dir := range m.config.TemplateDirs {
		loader := NewFileLoader(dir)
		tmpl, err := loader.Load(ctx, info.Provider)
		if err == nil {
			// 解析模板
			parsed, err := m.parser.Parse(tmpl.Content)
			if err != nil {
				continue
			}
			tmpl.Sections = parsed.Sections
			tmpl.Variables = parsed.Variables
			tmpl.Provider = info.Provider
			return tmpl, nil
		}
	}
	
	// 使用默认加载逻辑
	return m.DefaultManager.LoadTemplate(ctx, info)
}

// Helper functions

// FindRepositoryRoot 查找仓库根目录
func FindRepositoryRoot() (string, error) {
	// 从当前目录开始向上查找.git目录
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return dir, nil
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// 已到达文件系统根目录
			break
		}
		dir = parent
	}
	
	return "", fmt.Errorf("not in a git repository")
}

// CreateTemplateData 从各种源创建模板数据
func CreateTemplateData(commitMsg string, branch string, changedFiles []string) *TemplateData {
	data := &TemplateData{
		CommitMessage: commitMsg,
		Branch:        branch,
		ChangedFiles:  changedFiles,
		FilesCount:    len(changedFiles),
		FileStats:     make(map[string]*FileStat),
	}
	
	// 初始化文件统计
	for _, file := range changedFiles {
		data.FileStats[file] = &FileStat{
			Path: file,
		}
	}
	
	return data
}

// EnrichTemplateData 丰富模板数据
func EnrichTemplateData(data *TemplateData, info *provider.RemoteInfo) {
	if info != nil {
		data.RepoOwner = info.Owner
		data.RepoName = info.Repo
		data.Remote = "origin" // 默认值，可以从其他地方获取
	}
	
	// 设置默认的基础分支
	if data.BaseBranch == "" {
		data.BaseBranch = "main"
	}
}