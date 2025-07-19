package template

import (
	"testing"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownParser_Parse(t *testing.T) {
	parser := NewMarkdownParser()
	
	tests := []struct {
		name    string
		content string
		wantErr bool
		validate func(t *testing.T, tmpl *Template)
	}{
		{
			name: "basic template with sections",
			content: `# Pull Request

## Description
Please provide a description of your changes.

{{.CommitMessage}}

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change

## Testing
<!-- Testing instructions -->
[TestingInstructions]
`,
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				// 验证章节
				assert.Len(t, tmpl.Sections, 4)
				assert.Contains(t, tmpl.Sections, "Pull Request")
				assert.Contains(t, tmpl.Sections, "Description")
				assert.Contains(t, tmpl.Sections, "Type of Change")
				assert.Contains(t, tmpl.Sections, "Testing")
				
				// 验证变量
				assert.GreaterOrEqual(t, len(tmpl.Variables), 2)
				
				// 查找特定变量
				hasCommitMessage := false
				hasTestingInstructions := false
				for _, v := range tmpl.Variables {
					if v.Name == "CommitMessage" {
						hasCommitMessage = true
						assert.Equal(t, "{{.CommitMessage}}", v.Placeholder)
					}
					if v.Name == "TestingInstructions" {
						hasTestingInstructions = true
						assert.Equal(t, "[TestingInstructions]", v.Placeholder)
					}
				}
				assert.True(t, hasCommitMessage)
				assert.True(t, hasTestingInstructions)
			},
		},
		{
			name: "template with variable descriptions",
			content: `# PR Template

<!-- CommitMessage: The generated commit message -->
{{.CommitMessage}}

<!-- IssueNumber: Related issue number -->
Closes #{{.IssueNumber}}
`,
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				// 查找带描述的变量
				var commitMsgVar *Variable
				var issueNumVar *Variable
				
				for _, v := range tmpl.Variables {
					if v.Name == "CommitMessage" {
						commitMsgVar = &v
					}
					if v.Name == "IssueNumber" {
						issueNumVar = &v
					}
				}
				
				require.NotNil(t, commitMsgVar)
				assert.Equal(t, "The generated commit message", commitMsgVar.Description)
				
				require.NotNil(t, issueNumVar)
				assert.Equal(t, "Related issue number", issueNumVar.Description)
			},
		},
		{
			name: "template with multiple variable formats",
			content: `# Template

{{.GoTemplate}}
[BracketVar]
<AngleVar>
{BraceVar}
<!-- CommentVar -->
`,
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				varNames := make(map[string]bool)
				for _, v := range tmpl.Variables {
					varNames[v.Name] = true
				}
				
				assert.True(t, varNames["GoTemplate"])
				assert.True(t, varNames["BracketVar"])
				assert.True(t, varNames["AngleVar"])
				assert.True(t, varNames["BraceVar"])
				assert.True(t, varNames["CommentVar"])
			},
		},
		{
			name: "template without sections",
			content: `This is a simple template with {{.Variable}} but no markdown sections.`,
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				// 应该有一个默认章节
				assert.Len(t, tmpl.Sections, 1)
				assert.Contains(t, tmpl.Sections, "Content")
				
				// 应该找到变量
				assert.Len(t, tmpl.Variables, 1)
				assert.Equal(t, "Variable", tmpl.Variables[0].Name)
			},
		},
		{
			name:    "empty template",
			content: "",
			wantErr: true,
		},
		{
			name: "template with nested sections",
			content: `# Main Title

## Section 1
Content 1

### Subsection 1.1
Subcontent

## Section 2
Content 2
`,
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				assert.Len(t, tmpl.Sections, 4)
				
				// 验证章节级别
				if section, ok := tmpl.Sections["Section 1"]; ok {
					assert.Equal(t, 2, section.Level)
				}
				if section, ok := tmpl.Sections["Subsection 1.1"]; ok {
					assert.Equal(t, 3, section.Level)
				}
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := parser.Parse(tt.content)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, tmpl)
				
				if tt.validate != nil {
					tt.validate(t, tmpl)
				}
			}
		})
	}
}

func TestMarkdownParser_ExtractSections(t *testing.T) {
	parser := NewMarkdownParser()
	
	content := `# Title

Some intro text.

## Description
*Required* - Please describe your changes.

## Checklist
- [ ] Tests added
- [ ] Documentation updated

### Review Guidelines
Please follow our review guidelines.

## Related Issues
Closes #123
`
	
	sections, err := parser.ExtractSections(content)
	assert.NoError(t, err)
	assert.Len(t, sections, 5)
	
	// 验证Description章节
	desc := sections["Description"]
	require.NotNil(t, desc)
	assert.Equal(t, 2, desc.Level)
	assert.True(t, desc.Required)
	assert.Contains(t, desc.Content, "Required")
	assert.Contains(t, desc.Content, "Please describe your changes")
	
	// 验证Checklist章节
	checklist := sections["Checklist"]
	require.NotNil(t, checklist)
	assert.Contains(t, checklist.Content, "- [ ] Tests added")
	assert.Contains(t, checklist.Content, "- [ ] Documentation updated")
	
	// 验证嵌套章节
	review := sections["Review Guidelines"]
	require.NotNil(t, review)
	assert.Equal(t, 3, review.Level)
}

func TestMarkdownParser_ExtractVariables(t *testing.T) {
	parser := NewMarkdownParser()
	
	tests := []struct {
		name     string
		content  string
		expected map[string]struct {
			placeholder string
			required    bool
			description string
		}
	}{
		{
			name: "various formats",
			content: `
{{.CommitMessage}}
[Branch]
<Files>
{Count}
<!-- IssueNumber -->
`,
			expected: map[string]struct {
				placeholder string
				required    bool
				description string
			}{
				"CommitMessage": {placeholder: "{{.CommitMessage}}", required: false},
				"Branch":        {placeholder: "[Branch]", required: false},
				"Files":         {placeholder: "<Files>", required: false},
				"Count":         {placeholder: "{Count}", required: false},
				"IssueNumber":   {placeholder: "<!-- IssueNumber -->", required: false},
			},
		},
		{
			name: "with descriptions",
			content: `
<!-- Title: PR title -->
<!-- Description: Required - PR description -->
`,
			expected: map[string]struct {
				placeholder string
				required    bool
				description string
			}{
				"Title":       {placeholder: "<!-- Title: PR title -->", required: true, description: "PR title"},
				"Description": {placeholder: "<!-- Description: Required - PR description -->", required: true, description: "Required - PR description"},
			},
		},
		{
			name: "required markers",
			content: `
Required: {{.Summary}}
{{.Description}} *
* [Title]
`,
			expected: map[string]struct {
				placeholder string
				required    bool
				description string
			}{
				"Summary":     {placeholder: "{{.Summary}}", required: true},
				"Description": {placeholder: "{{.Description}}", required: true},
				"Title":       {placeholder: "[Title]", required: true},
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variables, err := parser.ExtractVariables(tt.content)
			assert.NoError(t, err)
			
			// 创建映射便于查找
			varMap := make(map[string]Variable)
			for _, v := range variables {
				varMap[v.Name] = v
			}
			
			// 验证每个期望的变量
			for name, expected := range tt.expected {
				v, ok := varMap[name]
				assert.True(t, ok, "Variable %s not found", name)
				
				if ok {
					assert.Equal(t, expected.placeholder, v.Placeholder)
					assert.Equal(t, expected.required, v.Required, "Variable %s required mismatch", name)
					if expected.description != "" {
						assert.Equal(t, expected.description, v.Description)
					}
				}
			}
		})
	}
}

func TestSimpleParser(t *testing.T) {
	parser := NewSimpleParser()
	
	content := `# PR Template

## What
{{.CommitTitle}}

## Why
{{.CommitBody}}

## Changes
- {{.ChangedFiles}}
`
	
	tmpl, err := parser.Parse(content)
	assert.NoError(t, err)
	require.NotNil(t, tmpl)
	
	// 验证章节
	assert.Len(t, tmpl.Sections, 4)
	assert.Contains(t, tmpl.Sections, "PR Template")
	assert.Contains(t, tmpl.Sections, "What")
	assert.Contains(t, tmpl.Sections, "Why")
	assert.Contains(t, tmpl.Sections, "Changes")
	
	// 验证变量
	assert.GreaterOrEqual(t, len(tmpl.Variables), 3)
	
	varNames := make(map[string]bool)
	for _, v := range tmpl.Variables {
		varNames[v.Name] = true
	}
	
	assert.True(t, varNames["CommitTitle"])
	assert.True(t, varNames["CommitBody"])
	assert.True(t, varNames["ChangedFiles"])
}

func TestIsRequiredSection(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"Description", true},
		{"DESCRIPTION", true},
		{"What does this PR do?", true},
		{"Summary", true},
		{"Changes Made", true},
		{"Type of Change", true},
		{"Testing", false},
		{"Checklist", false},
		{"Notes", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRequiredSection(tt.name)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRequiredVariable(t *testing.T) {
	content := `
Required: {{.Var1}}
{{.Var2}} *required*
必填: [Var3]
* <Var4>
{{.Description}}
{{.OptionalVar}}
`
	
	tests := []struct {
		name     string
		expected bool
	}{
		{"Var1", true},
		{"Var2", true},
		{"Var3", true},
		{"Var4", true},
		{"Description", true}, // 默认必填
		{"OptionalVar", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRequiredVariable(tt.name, content)
			assert.Equal(t, tt.expected, result)
		})
	}
}