package template

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	
	"github.com/penwyp/catmit/internal/errors"
)

// 预定义错误
var (
	ErrProcessingFailed = errors.New(
		errors.ErrTypeUnknown,
		"模板处理失败",
	)
	
	ErrRequiredFieldMissing = errors.New(
		errors.ErrTypeValidation,
		"必填字段缺失",
	).WithSuggestion("请提供所有必填字段的值")
)

// TemplateProcessor 模板处理器
type TemplateProcessor struct {
	// 自定义函数映射
	funcMap template.FuncMap
}

// NewTemplateProcessor 创建模板处理器
func NewTemplateProcessor() *TemplateProcessor {
	return &TemplateProcessor{
		funcMap: createDefaultFuncMap(),
	}
}

// Process 处理模板，替换变量
func (p *TemplateProcessor) Process(tmpl *Template, data *TemplateData) (string, error) {
	// 验证必填项
	if err := p.ValidateRequired(tmpl, data); err != nil {
		return "", err
	}
	
	// 预处理数据
	p.preprocessData(data)
	
	// 替换变量
	processed := tmpl.Content
	
	// 使用不同的替换策略
	processed = p.replaceGoTemplateVars(processed, data)
	processed = p.replacePlaceholderVars(processed, data)
	processed = p.fillSections(processed, tmpl, data)
	
	// 后处理：清理空行等
	processed = p.postprocess(processed)
	
	return processed, nil
}

// ValidateRequired 验证必填项
func (p *TemplateProcessor) ValidateRequired(tmpl *Template, data *TemplateData) error {
	var missingFields []string
	
	// 检查必填变量
	for _, variable := range tmpl.Variables {
		if !variable.Required {
			continue
		}
		
		// 检查对应的数据字段是否存在
		if p.isFieldEmpty(variable.Name, data) {
			missingFields = append(missingFields, variable.Name)
		}
	}
	
	if len(missingFields) > 0 {
		return errors.Wrap(
			errors.ErrTypeValidation,
			fmt.Sprintf("缺失必填字段: %s", strings.Join(missingFields, ", ")),
			nil,
		)
	}
	
	return nil
}

// preprocessData 预处理模板数据
func (p *TemplateProcessor) preprocessData(data *TemplateData) {
	// 分离提交消息的标题和正文
	if data.CommitMessage != "" && data.CommitTitle == "" {
		lines := strings.Split(data.CommitMessage, "\n")
		data.CommitTitle = lines[0]
		if len(lines) > 1 {
			data.CommitBody = strings.Join(lines[1:], "\n")
			data.CommitBody = strings.TrimSpace(data.CommitBody)
		}
	}
	
	// 计算文件统计
	if data.FileStats != nil {
		data.FilesCount = len(data.FileStats)
		data.AddedLines = 0
		data.DeletedLines = 0
		
		for _, stat := range data.FileStats {
			data.AddedLines += stat.Added
			data.DeletedLines += stat.Deleted
		}
	} else if len(data.ChangedFiles) > 0 {
		data.FilesCount = len(data.ChangedFiles)
	}
	
	// 从分支名提取issue号
	if data.IssueNumber == "" {
		data.IssueNumber = extractIssueNumber(data.Branch)
		if data.IssueNumber == "" {
			data.IssueNumber = extractIssueNumber(data.CommitMessage)
		}
	}
	
	// 检测特殊标记
	lowerMsg := strings.ToLower(data.CommitMessage)
	data.BreakingChange = strings.Contains(lowerMsg, "breaking") || 
	                  strings.Contains(lowerMsg, "!:")
	data.TestsAdded = p.detectTestsAdded(data)
	data.DocsUpdated = p.detectDocsUpdated(data)
}

// replaceGoTemplateVars 替换Go模板风格的变量
func (p *TemplateProcessor) replaceGoTemplateVars(content string, data *TemplateData) string {
	// 创建模板
	tmpl, err := template.New("pr").Funcs(p.funcMap).Parse(content)
	if err != nil {
		// 如果解析失败，返回原内容
		return content
	}
	
	// 执行模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// 如果执行失败，返回原内容
		return content
	}
	
	return buf.String()
}

// replacePlaceholderVars 替换占位符风格的变量
func (p *TemplateProcessor) replacePlaceholderVars(content string, data *TemplateData) string {
	replacements := p.createReplacementMap(data)
	
	// 替换各种格式的占位符
	for key, value := range replacements {
		// [Variable] 格式
		content = strings.ReplaceAll(content, fmt.Sprintf("[%s]", key), value)
		content = strings.ReplaceAll(content, fmt.Sprintf("[%s]", strings.ToLower(key)), value)
		
		// <Variable> 格式
		content = strings.ReplaceAll(content, fmt.Sprintf("<%s>", key), value)
		content = strings.ReplaceAll(content, fmt.Sprintf("<%s>", strings.ToLower(key)), value)
		
		// {Variable} 格式
		content = strings.ReplaceAll(content, fmt.Sprintf("{%s}", key), value)
		content = strings.ReplaceAll(content, fmt.Sprintf("{%s}", strings.ToLower(key)), value)
		
		// <!-- Variable --> 格式
		content = strings.ReplaceAll(content, fmt.Sprintf("<!-- %s -->", key), value)
		content = strings.ReplaceAll(content, fmt.Sprintf("<!-- %s -->", strings.ToLower(key)), value)
	}
	
	return content
}

// fillSections 填充特定章节
func (p *TemplateProcessor) fillSections(content string, tmpl *Template, data *TemplateData) string {
	// 特殊处理某些章节
	
	// 填充测试章节
	if section, exists := tmpl.Sections["Testing"]; exists && section.Content == "" {
		testingInstructions := p.generateTestingInstructions(data)
		content = strings.Replace(content, "## Testing\n", 
			fmt.Sprintf("## Testing\n%s\n", testingInstructions), 1)
	}
	
	// 填充Checklist
	content = p.fillChecklist(content, data)
	
	return content
}

// postprocess 后处理
func (p *TemplateProcessor) postprocess(content string) string {
	// 移除多余的空行
	lines := strings.Split(content, "\n")
	var processed []string
	emptyCount := 0
	
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			emptyCount++
			if emptyCount <= 2 {
				processed = append(processed, line)
			}
		} else {
			emptyCount = 0
			processed = append(processed, line)
		}
	}
	
	return strings.Join(processed, "\n")
}

// isFieldEmpty 检查字段是否为空
func (p *TemplateProcessor) isFieldEmpty(fieldName string, data *TemplateData) bool {
	switch strings.ToLower(fieldName) {
	case "commitmessage", "commit_message":
		return data.CommitMessage == ""
	case "committitle", "commit_title", "title":
		return data.CommitTitle == ""
	case "commitbody", "commit_body", "description":
		return data.CommitBody == ""
	case "branch":
		return data.Branch == ""
	case "changedfiles", "changed_files", "files":
		return len(data.ChangedFiles) == 0
	case "summary", "changessummary", "changes_summary":
		return data.ChangesSummary == ""
	default:
		return false
	}
}

// createReplacementMap 创建替换映射
func (p *TemplateProcessor) createReplacementMap(data *TemplateData) map[string]string {
	m := make(map[string]string)
	
	// 基础信息
	m["CommitMessage"] = data.CommitMessage
	m["CommitTitle"] = data.CommitTitle
	m["CommitBody"] = data.CommitBody
	m["Title"] = data.CommitTitle
	m["Description"] = data.CommitBody
	m["Branch"] = data.Branch
	m["BaseBranch"] = data.BaseBranch
	m["Remote"] = data.Remote
	m["RepoOwner"] = data.RepoOwner
	m["RepoName"] = data.RepoName
	
	// 文件信息
	m["ChangedFiles"] = strings.Join(data.ChangedFiles, "\n")
	m["FilesCount"] = strconv.Itoa(data.FilesCount)
	m["AddedLines"] = strconv.Itoa(data.AddedLines)
	m["DeletedLines"] = strconv.Itoa(data.DeletedLines)
	
	// 其他信息
	m["ChangesSummary"] = data.ChangesSummary
	m["IssueNumber"] = data.IssueNumber
	m["RecentCommits"] = strings.Join(data.RecentCommits, "\n")
	
	// 布尔值
	m["BreakingChange"] = strconv.FormatBool(data.BreakingChange)
	m["TestsAdded"] = strconv.FormatBool(data.TestsAdded)
	m["DocsUpdated"] = strconv.FormatBool(data.DocsUpdated)
	
	return m
}

// createDefaultFuncMap 创建默认的模板函数映射
func createDefaultFuncMap() template.FuncMap {
	return template.FuncMap{
		// 字符串处理
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"title":      strings.Title,
		"trim":       strings.TrimSpace,
		"replace":    strings.ReplaceAll,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		
		// 列表处理
		"join": strings.Join,
		"split": strings.Split,
		
		// 条件判断
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"empty": func(val interface{}) bool {
			if val == nil {
				return true
			}
			switch v := val.(type) {
			case string:
				return v == ""
			case []string:
				return len(v) == 0
			default:
				return false
			}
		},
		
		// 格式化
		"indent": func(spaces int, s string) string {
			indent := strings.Repeat(" ", spaces)
			lines := strings.Split(s, "\n")
			for i, line := range lines {
				if line != "" {
					lines[i] = indent + line
				}
			}
			return strings.Join(lines, "\n")
		},
		"list": func(items []string) string {
			var result []string
			for _, item := range items {
				result = append(result, "- "+item)
			}
			return strings.Join(result, "\n")
		},
	}
}

// extractIssueNumber 从文本中提取issue编号
func extractIssueNumber(text string) string {
	// 匹配 #123, issue-123, JIRA-123 等格式
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`#(\d+)`),
		regexp.MustCompile(`(?i)issue[-_]?(\d+)`),
		regexp.MustCompile(`([A-Z]+-\d+)`), // JIRA格式
	}
	
	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
	}
	
	return ""
}

// detectTestsAdded 检测是否添加了测试
func (p *TemplateProcessor) detectTestsAdded(data *TemplateData) bool {
	for _, file := range data.ChangedFiles {
		if strings.Contains(file, "_test.go") ||
		   strings.Contains(file, ".test.") ||
		   strings.Contains(file, "/test/") ||
		   strings.Contains(file, "/tests/") {
			return true
		}
	}
	
	// 检查提交消息
	lowerMsg := strings.ToLower(data.CommitMessage)
	return strings.Contains(lowerMsg, "test") ||
	       strings.Contains(lowerMsg, "测试")
}

// detectDocsUpdated 检测是否更新了文档
func (p *TemplateProcessor) detectDocsUpdated(data *TemplateData) bool {
	for _, file := range data.ChangedFiles {
		if strings.HasSuffix(file, ".md") ||
		   strings.HasSuffix(file, ".rst") ||
		   strings.HasSuffix(file, ".txt") ||
		   strings.Contains(file, "/docs/") ||
		   strings.Contains(file, "/doc/") ||
		   strings.Contains(file, "README") {
			return true
		}
	}
	
	// 检查提交消息
	lowerMsg := strings.ToLower(data.CommitMessage)
	return strings.Contains(lowerMsg, "doc") ||
	       strings.Contains(lowerMsg, "文档")
}

// generateTestingInstructions 生成测试说明
func (p *TemplateProcessor) generateTestingInstructions(data *TemplateData) string {
	var instructions []string
	
	// 基于文件类型生成测试建议
	hasGoFiles := false
	hasJSFiles := false
	hasConfigFiles := false
	
	for _, file := range data.ChangedFiles {
		switch {
		case strings.HasSuffix(file, ".go"):
			hasGoFiles = true
		case strings.HasSuffix(file, ".js") || strings.HasSuffix(file, ".ts"):
			hasJSFiles = true
		case strings.Contains(file, "config") || strings.HasSuffix(file, ".yaml") || strings.HasSuffix(file, ".json"):
			hasConfigFiles = true
		}
	}
	
	if hasGoFiles {
		instructions = append(instructions, "1. Run `go test ./...` to execute all tests")
		instructions = append(instructions, "2. Run `go build` to ensure the code compiles")
	}
	
	if hasJSFiles {
		instructions = append(instructions, "1. Run `npm test` to execute all tests")
		instructions = append(instructions, "2. Run `npm run build` to ensure the code builds")
	}
	
	if hasConfigFiles {
		instructions = append(instructions, "- Verify configuration changes are backward compatible")
		instructions = append(instructions, "- Test with both old and new configuration formats")
	}
	
	if len(instructions) == 0 {
		instructions = append(instructions, "- Manual testing of the changes")
		instructions = append(instructions, "- Verify no regressions were introduced")
	}
	
	return strings.Join(instructions, "\n")
}

// fillChecklist 填充检查列表
func (p *TemplateProcessor) fillChecklist(content string, data *TemplateData) string {
	// 查找checkbox模式: - [ ] 或 - [x]
	checkboxPattern := regexp.MustCompile(`(?m)^(\s*)-\s*\[\s*\]\s*(.+)$`)
	
	return checkboxPattern.ReplaceAllStringFunc(content, func(match string) string {
		lower := strings.ToLower(match)
		
		// 根据条件自动勾选
		shouldCheck := false
		
		switch {
		case strings.Contains(lower, "test"):
			if data.TestsAdded && (strings.Contains(lower, "added") || strings.Contains(lower, "write") || strings.Contains(lower, "written")) {
				shouldCheck = true
			}
		case strings.Contains(lower, "doc") && data.DocsUpdated:
			shouldCheck = true
		case strings.Contains(lower, "breaking") && !data.BreakingChange:
			shouldCheck = true // 勾选"没有破坏性变更"
		case strings.Contains(lower, "lint") || strings.Contains(lower, "format"):
			shouldCheck = true // 假设已通过lint
		}
		
		if shouldCheck {
			return strings.Replace(match, "[ ]", "[x]", 1)
		}
		
		return match
	})
}