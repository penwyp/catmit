package template

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	
	"github.com/penwyp/catmit/internal/errors"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// 预定义错误
var (
	ErrTemplateParseError = errors.New(
		errors.ErrTypeValidation,
		"模板解析失败",
	)
	
	ErrInvalidTemplate = errors.New(
		errors.ErrTypeValidation,
		"无效的模板格式",
	).WithSuggestion("确保模板是有效的Markdown格式")
)

// 常用的变量模式
var (
	// Go模板风格: {{.Variable}}
	goTemplatePattern = regexp.MustCompile(`\{\{\.(\w+)\}\}`)
	
	// 占位符风格: [Variable], <Variable>, {Variable}
	bracketPattern = regexp.MustCompile(`\[(\w+)\]`)
	anglePattern   = regexp.MustCompile(`<(\w+)>`)
	bracePattern   = regexp.MustCompile(`\{(\w+)\}`)
	
	// Markdown注释风格: <!-- Variable -->
	commentPattern = regexp.MustCompile(`<!--\s*(\w+)\s*-->`)
	
	// 变量描述模式: <!-- Variable: Description -->
	descriptionPattern = regexp.MustCompile(`<!--\s*(\w+)\s*:\s*(.+?)\s*-->`)
)

// MarkdownParser 基于goldmark的Markdown解析器
type MarkdownParser struct {
	parser goldmark.Markdown
}

// NewMarkdownParser 创建Markdown解析器
func NewMarkdownParser() *MarkdownParser {
	return &MarkdownParser{
		parser: goldmark.New(
			goldmark.WithExtensions(),
		),
	}
}

// Parse 解析模板内容
func (p *MarkdownParser) Parse(content string) (*Template, error) {
	if content == "" {
		return nil, ErrInvalidTemplate
	}
	
	tmpl := &Template{
		Content: content,
	}
	
	// 提取章节
	sections, err := p.ExtractSections(content)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeValidation, "模板解析失败", err)
	}
	tmpl.Sections = sections
	
	// 提取变量
	variables, err := p.ExtractVariables(content)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeValidation, "模板解析失败", err)
	}
	tmpl.Variables = variables
	
	return tmpl, nil
}

// ExtractSections 提取模板章节
func (p *MarkdownParser) ExtractSections(content string) (map[string]*Section, error) {
	sections := make(map[string]*Section)
	
	// 解析Markdown AST
	reader := text.NewReader([]byte(content))
	doc := p.parser.Parser().Parse(reader)
	
	// 遍历AST查找标题和内容
	var currentSection *Section
	var currentContent bytes.Buffer
	
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		
		switch node := n.(type) {
		case *ast.Heading:
			// 保存前一个章节
			if currentSection != nil {
				currentSection.Content = strings.TrimSpace(currentContent.String())
				sections[currentSection.Name] = currentSection
				currentContent.Reset()
			}
			
			// 开始新章节
			headingText := extractText(node, content)
			currentSection = &Section{
				Name:     headingText,
				Level:    node.Level,
				Required: isRequiredSection(headingText),
			}
			
		default:
			// 收集非标题内容
			if currentSection != nil {
				nodeText := extractNodeText(n, content)
				if nodeText != "" {
					currentContent.WriteString(nodeText)
					currentContent.WriteString("\n")
				}
			}
		}
		
		return ast.WalkContinue, nil
	})
	
	if err != nil {
		return nil, err
	}
	
	// 保存最后一个章节
	if currentSection != nil {
		currentSection.Content = strings.TrimSpace(currentContent.String())
		sections[currentSection.Name] = currentSection
	}
	
	// 如果没有明确的章节，将整个内容作为默认章节
	if len(sections) == 0 {
		sections["Content"] = &Section{
			Name:     "Content",
			Content:  content,
			Required: false,
			Level:    1,
		}
	}
	
	return sections, nil
}

// ExtractVariables 提取模板变量
func (p *MarkdownParser) ExtractVariables(content string) ([]Variable, error) {
	variableMap := make(map[string]*Variable)
	
	// 提取带描述的变量
	matches := descriptionPattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			name := match[1]
			desc := match[2]
			variableMap[name] = &Variable{
				Name:        name,
				Placeholder: match[0],
				Description: desc,
				Required:    isRequiredVariable(name, content),
			}
		}
	}
	
	// 提取各种格式的变量
	patterns := []struct {
		re   *regexp.Regexp
		fmt  string
	}{
		{goTemplatePattern, "{{.%s}}"},
		{bracketPattern, "[%s]"},
		{anglePattern, "<%s>"},
		{bracePattern, "{%s}"},
		{commentPattern, "<!-- %s -->"},
	}
	
	for _, pattern := range patterns {
		matches := pattern.re.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				name := match[1]
				if _, exists := variableMap[name]; !exists {
					variableMap[name] = &Variable{
						Name:        name,
						Placeholder: match[0],
						Required:    isRequiredVariable(name, content),
					}
				}
			}
		}
	}
	
	// 转换为切片
	var variables []Variable
	for _, v := range variableMap {
		variables = append(variables, *v)
	}
	
	return variables, nil
}

// extractText 从AST节点提取文本
func extractText(node ast.Node, source string) string {
	var text bytes.Buffer
	
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			text.Write(n.Segment.Value([]byte(source)))
		case *ast.CodeSpan:
			text.Write(n.Text([]byte(source)))
		default:
			// 递归处理其他节点
			text.WriteString(extractText(child, source))
		}
	}
	
	return strings.TrimSpace(text.String())
}

// extractNodeText 提取节点的完整文本
func extractNodeText(node ast.Node, source string) string {
	switch n := node.(type) {
	case *ast.Text:
		return string(n.Segment.Value([]byte(source)))
	case *ast.Paragraph:
		return extractText(n, source)
	case *ast.ListItem:
		return "- " + extractText(n, source)
	case *ast.CodeBlock:
		var text bytes.Buffer
		for i := 0; i < n.Lines().Len(); i++ {
			line := n.Lines().At(i)
			text.Write(line.Value([]byte(source)))
		}
		return text.String()
	case *ast.FencedCodeBlock:
		var text bytes.Buffer
		text.WriteString("```")
		if n.Info != nil {
			text.Write(n.Info.Segment.Value([]byte(source)))
		}
		text.WriteString("\n")
		for i := 0; i < n.Lines().Len(); i++ {
			line := n.Lines().At(i)
			text.Write(line.Value([]byte(source)))
		}
		text.WriteString("```")
		return text.String()
	default:
		return ""
	}
}

// isRequiredSection 判断章节是否必填
func isRequiredSection(name string) bool {
	requiredNames := []string{
		"description",
		"what",
		"why",
		"summary",
		"changes",
		"type of change",
	}
	
	lowerName := strings.ToLower(name)
	for _, required := range requiredNames {
		if strings.Contains(lowerName, required) {
			return true
		}
	}
	
	return false
}

// isRequiredVariable 判断变量是否必填
func isRequiredVariable(name string, content string) bool {
	// 检查是否在必填标记附近
	requiredMarkers := []string{
		"required",
		"必填",
		"必须",
		"*",
	}
	
	lowerContent := strings.ToLower(content)
	lowerName := strings.ToLower(name)
	
	// 查找变量周围的文本
	for _, marker := range requiredMarkers {
		// 检查变量前后是否有必填标记
		patterns := []string{
			fmt.Sprintf("%s.*%s", marker, lowerName),
			fmt.Sprintf("%s.*%s", lowerName, marker),
			fmt.Sprintf("%s %s", marker, lowerName),
			fmt.Sprintf("%s %s", lowerName, marker),
			// 处理带尖括号的情况
			fmt.Sprintf("%s.*<%s>", marker, lowerName),
			fmt.Sprintf("<%s>.*%s", lowerName, marker),
			// 处理带星号包围的情况
			fmt.Sprintf("%s.*\\*\\*%s\\*\\*", marker, lowerName),
			fmt.Sprintf("\\*\\*%s\\*\\*.*%s", lowerName, marker),
			// 处理末尾带星号的情况
			fmt.Sprintf("%s\\*", lowerName),
		}
		
		for _, pattern := range patterns {
			if strings.Contains(lowerContent, pattern) {
				return true
			}
		}
	}
	
	// 某些变量名默认必填
	requiredVars := []string{
		"description",
		"summary",
		"title",
		"what",
		"why",
	}
	
	for _, required := range requiredVars {
		if strings.EqualFold(name, required) {
			return true
		}
	}
	
	return false
}

// SimpleParser 简单的模板解析器（用于快速解析）
type SimpleParser struct{}

// NewSimpleParser 创建简单解析器
func NewSimpleParser() *SimpleParser {
	return &SimpleParser{}
}

// Parse 解析模板内容
func (s *SimpleParser) Parse(content string) (*Template, error) {
	tmpl := &Template{
		Content: content,
	}
	
	// 简单的章节提取（基于Markdown标题）
	sections, err := s.ExtractSections(content)
	if err != nil {
		return nil, err
	}
	tmpl.Sections = sections
	
	// 提取变量
	variables, err := s.ExtractVariables(content)
	if err != nil {
		return nil, err
	}
	tmpl.Variables = variables
	
	return tmpl, nil
}

// ExtractSections 简单提取章节
func (s *SimpleParser) ExtractSections(content string) (map[string]*Section, error) {
	sections := make(map[string]*Section)
	lines := strings.Split(content, "\n")
	
	var currentSection *Section
	var currentContent []string
	
	for _, line := range lines {
		// 检查是否是标题行
		if strings.HasPrefix(line, "#") {
			// 保存前一个章节
			if currentSection != nil {
				currentSection.Content = strings.TrimSpace(strings.Join(currentContent, "\n"))
				sections[currentSection.Name] = currentSection
				currentContent = nil
			}
			
			// 解析标题级别和文本
			level := 0
			for _, ch := range line {
				if ch == '#' {
					level++
				} else {
					break
				}
			}
			
			name := strings.TrimSpace(strings.TrimLeft(line, "#"))
			currentSection = &Section{
				Name:     name,
				Level:    level,
				Required: isRequiredSection(name),
			}
		} else if currentSection != nil {
			// 收集内容
			currentContent = append(currentContent, line)
		}
	}
	
	// 保存最后一个章节
	if currentSection != nil {
		currentSection.Content = strings.TrimSpace(strings.Join(currentContent, "\n"))
		sections[currentSection.Name] = currentSection
	}
	
	return sections, nil
}

// ExtractVariables 简单提取变量
func (s *SimpleParser) ExtractVariables(content string) ([]Variable, error) {
	return (&MarkdownParser{}).ExtractVariables(content)
}