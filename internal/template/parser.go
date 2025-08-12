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

// Predefined errors
var (
	ErrTemplateParseError = errors.New(
		errors.ErrTypeValidation,
		"Template parsing failed",
	)

	ErrInvalidTemplate = errors.New(
		errors.ErrTypeValidation,
		"Invalid template format",
	).WithSuggestion("Ensure the template is valid Markdown format")
)

// Common variable patterns
var (
	// Go template style: {{.Variable}}
	goTemplatePattern = regexp.MustCompile(`\{\{\.(\w+)\}\}`)

	// Placeholder styles: [Variable], <Variable>, {Variable}
	bracketPattern = regexp.MustCompile(`\[(\w+)\]`)
	anglePattern   = regexp.MustCompile(`<(\w+)>`)
	bracePattern   = regexp.MustCompile(`\{(\w+)\}`)

	// Markdown comment style: <!-- Variable -->
	commentPattern = regexp.MustCompile(`<!--\s*(\w+)\s*-->`)

	// Variable with description: <!-- Variable: Description -->
	descriptionPattern = regexp.MustCompile(`<!--\s*(\w+)\s*:\s*(.+?)\s*-->`)
)

// MarkdownParser is a Markdown parser based on goldmark
type MarkdownParser struct {
	parser goldmark.Markdown
}

// NewMarkdownParser creates a Markdown parser
func NewMarkdownParser() *MarkdownParser {
	return &MarkdownParser{
		parser: goldmark.New(
			goldmark.WithExtensions(),
		),
	}
}

// Parse parses the template content
func (p *MarkdownParser) Parse(content string) (*Template, error) {
	if content == "" {
		return nil, ErrInvalidTemplate
	}

	tmpl := &Template{
		Content: content,
	}

	// Extract sections
	sections, err := p.ExtractSections(content)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeValidation, "Template parsing failed", err)
	}
	tmpl.Sections = sections

	// Extract variables
	variables, err := p.ExtractVariables(content)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeValidation, "Template parsing failed", err)
	}
	tmpl.Variables = variables

	return tmpl, nil
}

// ExtractSections extracts template sections
func (p *MarkdownParser) ExtractSections(content string) (map[string]*Section, error) {
	sections := make(map[string]*Section)

	// Parse Markdown AST
	reader := text.NewReader([]byte(content))
	doc := p.parser.Parser().Parse(reader)

	// Traverse AST to find headings and content
	var currentSection *Section
	var currentContent bytes.Buffer

	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			// Save previous section
			if currentSection != nil {
				currentSection.Content = strings.TrimSpace(currentContent.String())
				sections[currentSection.Name] = currentSection
				currentContent.Reset()
			}

			// Start new section
			headingText := extractText(node, content)
			currentSection = &Section{
				Name:     headingText,
				Level:    node.Level,
				Required: isRequiredSection(headingText),
			}

		default:
			// Collect non-heading content
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
		return nil, errors.Wrap(errors.ErrTypeUnknown, "failed to walk AST", err)
	}

	// Save the last section
	if currentSection != nil {
		currentSection.Content = strings.TrimSpace(currentContent.String())
		sections[currentSection.Name] = currentSection
	}

	// If no explicit sections, treat the whole content as a default section
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

// ExtractVariables extracts template variables
func (p *MarkdownParser) ExtractVariables(content string) ([]Variable, error) {
	variableMap := make(map[string]*Variable)

	// Extract variables with descriptions
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

	// Extract variables in various formats
	patterns := []struct {
		re  *regexp.Regexp
		fmt string
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

	// Convert to slice
	var variables []Variable
	for _, v := range variableMap {
		variables = append(variables, *v)
	}

	return variables, nil
}

// extractText extracts text from an AST node
func extractText(node ast.Node, source string) string {
	var text bytes.Buffer

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			text.Write(n.Segment.Value([]byte(source)))
		case *ast.CodeSpan:
			// For CodeSpan, we need to extract the value from the segment
			if n.FirstChild() != nil {
				text.WriteString(extractText(n, source))
			}
		default:
			// Recursively process other nodes
			text.WriteString(extractText(child, source))
		}
	}

	return strings.TrimSpace(text.String())
}

// extractNodeText extracts the full text of a node
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

// isRequiredSection determines if a section is required
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

// isRequiredVariable determines if a variable is required
func isRequiredVariable(name string, content string) bool {
	// Check if required marker is near the variable
	requiredMarkers := []string{
		"required",
		"必填",
		"必须",
		"*",
	}

	lowerContent := strings.ToLower(content)
	lowerName := strings.ToLower(name)

	// Search for required markers around the variable
	for _, marker := range requiredMarkers {
		// Check if required marker is before or after the variable
		patterns := []string{
			fmt.Sprintf("%s.*%s", marker, lowerName),
			fmt.Sprintf("%s.*%s", lowerName, marker),
			fmt.Sprintf("%s %s", marker, lowerName),
			fmt.Sprintf("%s %s", lowerName, marker),
			// Handle angle bracket case
			fmt.Sprintf("%s <%s>", marker, lowerName),
			fmt.Sprintf("<%s> %s", lowerName, marker),
			// Handle surrounded by asterisks
			fmt.Sprintf("%s **%s**", marker, lowerName),
			fmt.Sprintf("**%s** %s", lowerName, marker),
			// Handle trailing asterisk
			fmt.Sprintf("%s*", lowerName),
			// Handle Required: {{.Var}} format
			fmt.Sprintf("%s: {{.%s}}", marker, lowerName),
			fmt.Sprintf("%s：{{.%s}}", marker, lowerName),
			// Handle {{.Var}} *required* format
			fmt.Sprintf("{{.%s}} *%s*", lowerName, marker),
			// Handle Required: [Var] format
			fmt.Sprintf("%s: [%s]", marker, lowerName),
			fmt.Sprintf("%s：[%s]", marker, lowerName),
			// Handle * <Var> format
			fmt.Sprintf("* <%s>", lowerName),
		}

		for _, pattern := range patterns {
			if strings.Contains(lowerContent, pattern) {
				return true
			}
		}
	}

	// Some variable names are always required
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

