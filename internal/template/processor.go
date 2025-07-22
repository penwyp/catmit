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

// Predefined errors
var (
	ErrProcessingFailed = errors.New(
		errors.ErrTypeUnknown,
		"Template processing failed",
	)

	ErrRequiredFieldMissing = errors.New(
		errors.ErrTypeValidation,
		"Required field missing",
	).WithSuggestion("Please provide values for all required fields")
)

// TemplateProcessor handles template processing
type TemplateProcessor struct {
	// Custom function map
	funcMap template.FuncMap
}

// NewTemplateProcessor creates a new template processor
func NewTemplateProcessor() *TemplateProcessor {
	return &TemplateProcessor{
		funcMap: createDefaultFuncMap(),
	}
}

// Process processes the template and replaces variables
func (p *TemplateProcessor) Process(tmpl *Template, data *TemplateData) (string, error) {
	// Validate required fields
	if err := p.ValidateRequired(tmpl, data); err != nil {
		return "", err
	}

	// Preprocess data
	p.preprocessData(data)

	// Replace variables
	processed := tmpl.Content

	// Use different replacement strategies
	processed = p.replaceGoTemplateVars(processed, data)
	processed = p.replacePlaceholderVars(processed, data)
	processed = p.fillSections(processed, tmpl, data)

	// Postprocess: clean up empty lines, etc.
	processed = p.postprocess(processed)

	return processed, nil
}

// ValidateRequired checks required fields
func (p *TemplateProcessor) ValidateRequired(tmpl *Template, data *TemplateData) error {
	var missingFields []string

	// Check required variables
	for _, variable := range tmpl.Variables {
		if !variable.Required {
			continue
		}

		// Check if the corresponding data field exists
		if p.isFieldEmpty(variable.Name, data) {
			missingFields = append(missingFields, variable.Name)
		}
	}

	if len(missingFields) > 0 {
		return errors.Wrap(
			errors.ErrTypeValidation,
			fmt.Sprintf("Missing required fields: %s", strings.Join(missingFields, ", ")),
			nil,
		)
	}

	return nil
}

// preprocessData preprocesses template data
func (p *TemplateProcessor) preprocessData(data *TemplateData) {
	// Split commit message into title and body
	if data.CommitMessage != "" && data.CommitTitle == "" {
		lines := strings.Split(data.CommitMessage, "\n")
		data.CommitTitle = lines[0]
		if len(lines) > 1 {
			data.CommitBody = strings.Join(lines[1:], "\n")
			data.CommitBody = strings.TrimSpace(data.CommitBody)
		}
	}

	// Calculate file statistics
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

	// Extract issue number from branch name
	if data.IssueNumber == "" {
		data.IssueNumber = extractIssueNumber(data.Branch)
		if data.IssueNumber == "" {
			data.IssueNumber = extractIssueNumber(data.CommitMessage)
		}
	}

	// Detect special markers
	lowerMsg := strings.ToLower(data.CommitMessage)
	data.BreakingChange = strings.Contains(lowerMsg, "breaking") ||
		strings.Contains(lowerMsg, "!:")

	// Only auto-detect if not explicitly set
	if !data.TestsAdded {
		data.TestsAdded = p.detectTestsAdded(data)
	}
	if !data.DocsUpdated {
		data.DocsUpdated = p.detectDocsUpdated(data)
	}
}

// replaceGoTemplateVars replaces Go template style variables
func (p *TemplateProcessor) replaceGoTemplateVars(content string, data *TemplateData) string {
	// Create template
	tmpl, err := template.New("pr").Funcs(p.funcMap).Parse(content)
	if err != nil {
		// If parsing fails, return original content
		return content
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// If execution fails, return original content
		return content
	}

	return buf.String()
}

// replacePlaceholderVars replaces placeholder style variables
func (p *TemplateProcessor) replacePlaceholderVars(content string, data *TemplateData) string {
	replacements := p.createReplacementMap(data)

	// Replace various placeholder formats
	for key, value := range replacements {
		// [Variable] format
		content = strings.ReplaceAll(content, fmt.Sprintf("[%s]", key), value)
		content = strings.ReplaceAll(content, fmt.Sprintf("[%s]", strings.ToLower(key)), value)

		// <Variable> format
		content = strings.ReplaceAll(content, fmt.Sprintf("<%s>", key), value)
		content = strings.ReplaceAll(content, fmt.Sprintf("<%s>", strings.ToLower(key)), value)

		// {Variable} format
		content = strings.ReplaceAll(content, fmt.Sprintf("{%s}", key), value)
		content = strings.ReplaceAll(content, fmt.Sprintf("{%s}", strings.ToLower(key)), value)

		// <!-- Variable --> format
		content = strings.ReplaceAll(content, fmt.Sprintf("<!-- %s -->", key), value)
		content = strings.ReplaceAll(content, fmt.Sprintf("<!-- %s -->", strings.ToLower(key)), value)
	}

	return content
}

// fillSections fills specific sections
func (p *TemplateProcessor) fillSections(content string, tmpl *Template, data *TemplateData) string {
	// Special handling for certain sections

	// Fill Testing section
	if section, exists := tmpl.Sections["Testing"]; exists && section.Content == "" {
		testingInstructions := p.generateTestingInstructions(data)
		content = strings.Replace(content, "## Testing\n",
			fmt.Sprintf("## Testing\n%s\n", testingInstructions), 1)
	}

	// Fill Checklist
	content = p.fillChecklist(content, data)

	return content
}

// postprocess post-processing
func (p *TemplateProcessor) postprocess(content string) string {
	// Remove excessive empty lines
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

// isFieldEmpty checks if a field is empty
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

// createReplacementMap creates the replacement map
func (p *TemplateProcessor) createReplacementMap(data *TemplateData) map[string]string {
	m := make(map[string]string)

	// Basic info
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

	// File info
	m["ChangedFiles"] = strings.Join(data.ChangedFiles, "\n")
	m["FilesCount"] = strconv.Itoa(data.FilesCount)
	m["AddedLines"] = strconv.Itoa(data.AddedLines)
	m["DeletedLines"] = strconv.Itoa(data.DeletedLines)

	// Other info
	m["ChangesSummary"] = data.ChangesSummary
	m["IssueNumber"] = data.IssueNumber
	m["RecentCommits"] = strings.Join(data.RecentCommits, "\n")

	// Boolean values
	m["BreakingChange"] = strconv.FormatBool(data.BreakingChange)
	m["TestsAdded"] = strconv.FormatBool(data.TestsAdded)
	m["DocsUpdated"] = strconv.FormatBool(data.DocsUpdated)

	return m
}

// titleCase converts a string to title case
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// createDefaultFuncMap creates the default template function map
func createDefaultFuncMap() template.FuncMap {
	return template.FuncMap{
		// String processing
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"title":     titleCase,
		"trim":      strings.TrimSpace,
		"replace":   strings.ReplaceAll,
		"contains":  strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,

		// List processing
		"join":  strings.Join,
		"split": strings.Split,

		// Conditional
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

		// Formatting
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

// extractIssueNumber extracts the issue number from text
func extractIssueNumber(text string) string {
	// Match #123, issue-123, JIRA-123, etc.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`#(\d+)`),
		regexp.MustCompile(`(?i)issue[-_]?(\d+)`),
		regexp.MustCompile(`([A-Z]+-\d+)`), // JIRA format
	}

	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(text); len(match) > 1 {
			return match[1]
		}
	}

	return ""
}

// detectTestsAdded checks whether any test files were added or if the commit message indicates tests were added.
func (p *TemplateProcessor) detectTestsAdded(data *TemplateData) bool {
	for _, file := range data.ChangedFiles {
		if strings.Contains(file, "_test.go") ||
			strings.Contains(file, ".test.") ||
			strings.Contains(file, "/test/") ||
			strings.Contains(file, "/tests/") {
			return true
		}
	}

	// Check if the commit message contains test-related keywords
	lowerMsg := strings.ToLower(data.CommitMessage)
	return strings.Contains(lowerMsg, "test") ||
		strings.Contains(lowerMsg, "测试")
}

// detectDocsUpdated detects if documentation was updated
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

	// Check commit message
	lowerMsg := strings.ToLower(data.CommitMessage)
	return strings.Contains(lowerMsg, "doc") ||
		strings.Contains(lowerMsg, "文档")
}

// generateTestingInstructions generates testing instructions
func (p *TemplateProcessor) generateTestingInstructions(data *TemplateData) string {
	var instructions []string

	// Generate test suggestions based on file types
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

// fillChecklist fills the checklist
func (p *TemplateProcessor) fillChecklist(content string, data *TemplateData) string {
	// Find checkbox pattern: - [ ] or - [x]
	checkboxPattern := regexp.MustCompile(`(?m)^(\s*)-\s*\[\s*\]\s*(.+)$`)

	return checkboxPattern.ReplaceAllStringFunc(content, func(match string) string {
		lower := strings.ToLower(match)

		// Auto-check based on conditions
		shouldCheck := false

		switch {
		case strings.Contains(lower, "test") && strings.Contains(lower, "added"):
			shouldCheck = data.TestsAdded
		case strings.Contains(lower, "test") && (strings.Contains(lower, "write") || strings.Contains(lower, "written")):
			shouldCheck = data.TestsAdded
		case strings.Contains(lower, "doc"):
			shouldCheck = data.DocsUpdated
		case strings.Contains(lower, "no breaking"):
			shouldCheck = !data.BreakingChange // Check if there is no breaking change
		case strings.Contains(lower, "lint") || strings.Contains(lower, "format"):
			shouldCheck = true // Assume lint passed
		}

		if shouldCheck {
			return strings.Replace(match, "[ ]", "[x]", 1)
		}

		return match
	})
}
