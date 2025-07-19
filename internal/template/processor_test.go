package template

import (
	"strings"
	"testing"
	
	"github.com/stretchr/testify/assert"
)

func TestTemplateProcessor_Process(t *testing.T) {
	processor := NewTemplateProcessor()
	
	tests := []struct {
		name     string
		template *Template
		data     *TemplateData
		wantErr  bool
		validate func(t *testing.T, result string)
	}{
		{
			name: "basic variable replacement",
			template: &Template{
				Content: `# PR: {{.CommitTitle}}

## Description
{{.CommitMessage}}

## Changes
Files changed: {{.FilesCount}}
- {{.ChangedFiles}}
`,
				Variables: []Variable{
					{Name: "CommitTitle", Placeholder: "{{.CommitTitle}}"},
					{Name: "CommitMessage", Placeholder: "{{.CommitMessage}}"},
					{Name: "FilesCount", Placeholder: "{{.FilesCount}}"},
					{Name: "ChangedFiles", Placeholder: "{{.ChangedFiles}}"},
				},
			},
			data: &TemplateData{
				CommitMessage: "feat: add new feature\n\nThis adds a new feature to the system",
				CommitTitle:   "feat: add new feature",
				ChangedFiles:  []string{"file1.go", "file2.go"},
				FilesCount:    2,
			},
			wantErr: false,
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "# PR: feat: add new feature")
				assert.Contains(t, result, "Files changed: 2")
				assert.Contains(t, result, "file1.go")
				assert.Contains(t, result, "file2.go")
			},
		},
		{
			name: "multiple placeholder formats",
			template: &Template{
				Content: `Title: [Title]
Branch: <Branch>
Issue: {IssueNumber}
Message: <!-- CommitMessage -->
`,
				Variables: []Variable{
					{Name: "Title", Placeholder: "[Title]"},
					{Name: "Branch", Placeholder: "<Branch>"},
					{Name: "IssueNumber", Placeholder: "{IssueNumber}"},
					{Name: "CommitMessage", Placeholder: "<!-- CommitMessage -->"},
				},
			},
			data: &TemplateData{
				CommitTitle:   "Fix bug",
				Branch:        "fix/issue-123",
				IssueNumber:   "123",
				CommitMessage: "Fixed the bug",
			},
			wantErr: false,
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "Title: Fix bug")
				assert.Contains(t, result, "Branch: fix/issue-123")
				assert.Contains(t, result, "Issue: 123")
				assert.Contains(t, result, "Message: Fixed the bug")
			},
		},
		{
			name: "template functions",
			template: &Template{
				Content: `# {{.CommitTitle | upper}}

Branch: {{.Branch | lower}}
Files ({{.ChangedFiles | len}}):
{{range .ChangedFiles}}  - {{.}}
{{end}}
`,
			},
			data: &TemplateData{
				CommitTitle:  "Fix Bug",
				Branch:       "FIX/ISSUE-123",
				ChangedFiles: []string{"main.go", "util.go"},
			},
			wantErr: false,
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "# FIX BUG")
				assert.Contains(t, result, "Branch: fix/issue-123")
				assert.Contains(t, result, "Files (2):")
				assert.Contains(t, result, "  - main.go")
				assert.Contains(t, result, "  - util.go")
			},
		},
		{
			name: "required field validation",
			template: &Template{
				Content: "{{.RequiredField}}",
				Variables: []Variable{
					{Name: "RequiredField", Required: true},
				},
			},
			data:    &TemplateData{},
			wantErr: true,
		},
		{
			name: "auto-fill checklist",
			template: &Template{
				Content: `## Checklist
- [ ] Tests added
- [ ] Documentation updated
- [ ] Lint passed
- [ ] No breaking changes
`,
			},
			data: &TemplateData{
				TestsAdded:     true,
				DocsUpdated:    false,
				BreakingChange: false,
			},
			wantErr: false,
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "- [x] Tests added")
				assert.Contains(t, result, "- [ ] Documentation updated")
				assert.Contains(t, result, "- [x] Lint passed")
				assert.Contains(t, result, "- [x] No breaking changes")
			},
		},
		{
			name: "issue number extraction",
			template: &Template{
				Content: "Related issue: #{{.IssueNumber}}",
			},
			data: &TemplateData{
				Branch: "fix/issue-456-memory-leak",
			},
			wantErr: false,
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "Related issue: #456")
			},
		},
		{
			name: "commit message parsing",
			template: &Template{
				Content: `Title: {{.CommitTitle}}
Body:
{{.CommitBody}}
`,
			},
			data: &TemplateData{
				CommitMessage: "feat: add feature\n\nDetailed description\nof the feature",
			},
			wantErr: false,
			validate: func(t *testing.T, result string) {
				assert.Contains(t, result, "Title: feat: add feature")
				assert.Contains(t, result, "Detailed description\nof the feature")
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.Process(tt.template, tt.data)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestTemplateProcessor_ValidateRequired(t *testing.T) {
	processor := NewTemplateProcessor()
	
	template := &Template{
		Variables: []Variable{
			{Name: "CommitMessage", Required: true},
			{Name: "Branch", Required: true},
			{Name: "OptionalField", Required: false},
		},
	}
	
	tests := []struct {
		name    string
		data    *TemplateData
		wantErr bool
	}{
		{
			name: "all required fields present",
			data: &TemplateData{
				CommitMessage: "feat: add feature",
				Branch:        "feature/new",
			},
			wantErr: false,
		},
		{
			name: "missing required field",
			data: &TemplateData{
				CommitMessage: "feat: add feature",
				// Branch is missing
			},
			wantErr: true,
		},
		{
			name: "empty required field",
			data: &TemplateData{
				CommitMessage: "",
				Branch:        "feature/new",
			},
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.ValidateRequired(template, tt.data)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPreprocessData(t *testing.T) {
	processor := NewTemplateProcessor()
	
	data := &TemplateData{
		CommitMessage: "fix: resolve memory leak in worker\n\nThis fixes issue #123 where workers were not releasing memory properly.",
		Branch:        "fix/PROJ-456-memory-leak",
		ChangedFiles:  []string{"worker.go", "worker_test.go", "docs/README.md"},
		FileStats: map[string]*FileStat{
			"worker.go":      {Added: 50, Deleted: 20},
			"worker_test.go": {Added: 100, Deleted: 0},
			"docs/README.md": {Added: 10, Deleted: 5},
		},
	}
	
	processor.preprocessData(data)
	
	// 验证提交消息分离
	assert.Equal(t, "fix: resolve memory leak in worker", data.CommitTitle)
	assert.Contains(t, data.CommitBody, "This fixes issue #123")
	
	// 验证文件统计
	assert.Equal(t, 3, data.FilesCount)
	assert.Equal(t, 160, data.AddedLines)
	assert.Equal(t, 25, data.DeletedLines)
	
	// 验证issue号提取
	assert.Equal(t, "456", data.IssueNumber)
	
	// 验证特殊标记检测
	assert.True(t, data.TestsAdded)
	assert.True(t, data.DocsUpdated)
	assert.False(t, data.BreakingChange)
}

func TestExtractIssueNumber(t *testing.T) {
	tests := []struct {
		text     string
		expected string
	}{
		{"fix: resolve issue #123", "123"},
		{"feature/issue-456-new-feature", "456"},
		{"PROJ-789: add feature", "PROJ-789"},
		{"fixes JIRA-1234", "JIRA-1234"},
		{"no issue number here", ""},
		{"multiple #111 and #222", "111"}, // 返回第一个
	}
	
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := extractIssueNumber(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateTestingInstructions(t *testing.T) {
	processor := NewTemplateProcessor()
	
	tests := []struct {
		name     string
		data     *TemplateData
		expected []string
	}{
		{
			name: "go files changed",
			data: &TemplateData{
				ChangedFiles: []string{"main.go", "util/helper.go"},
			},
			expected: []string{
				"go test ./...",
				"go build",
			},
		},
		{
			name: "js files changed",
			data: &TemplateData{
				ChangedFiles: []string{"app.js", "components/Button.tsx"},
			},
			expected: []string{
				"npm test",
				"npm run build",
			},
		},
		{
			name: "config files changed",
			data: &TemplateData{
				ChangedFiles: []string{"config.yaml", "settings.json"},
			},
			expected: []string{
				"configuration changes are backward compatible",
				"old and new configuration formats",
			},
		},
		{
			name: "mixed files",
			data: &TemplateData{
				ChangedFiles: []string{"main.go", "app.js", "config.yaml"},
			},
			expected: []string{
				"go test",
				"npm test",
				"configuration changes",
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.generateTestingInstructions(tt.data)
			
			for _, exp := range tt.expected {
				assert.Contains(t, result, exp)
			}
		})
	}
}

func TestFillChecklist(t *testing.T) {
	processor := NewTemplateProcessor()
	
	content := `## Checklist
- [ ] Tests added
- [ ] Tests pass
- [ ] Documentation updated
- [ ] Code follows style guidelines
- [ ] No breaking changes
- [ ] Lint checks pass
`
	
	data := &TemplateData{
		TestsAdded:     true,
		DocsUpdated:    true,
		BreakingChange: false,
	}
	
	result := processor.fillChecklist(content, data)
	
	// 验证自动勾选
	assert.Contains(t, result, "- [x] Tests added")
	assert.Contains(t, result, "- [ ] Tests pass") // 不自动勾选
	assert.Contains(t, result, "- [x] Documentation updated")
	assert.Contains(t, result, "- [ ] Code follows style guidelines")
	assert.Contains(t, result, "- [x] No breaking changes")
	assert.Contains(t, result, "- [x] Lint checks pass")
}

func TestPostprocess(t *testing.T) {
	processor := NewTemplateProcessor()
	
	input := `Line 1


Line 2



Line 3


Line 4`
	
	result := processor.postprocess(input)
	
	// 验证多余空行被移除（最多保留2个连续空行）
	lines := strings.Split(result, "\n")
	maxEmpty := 0
	currentEmpty := 0
	
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			currentEmpty++
			if currentEmpty > maxEmpty {
				maxEmpty = currentEmpty
			}
		} else {
			currentEmpty = 0
		}
	}
	
	assert.LessOrEqual(t, maxEmpty, 2)
}

func TestCustomFunctions(t *testing.T) {
	funcMap := createDefaultFuncMap()
	
	// 测试 default 函数
	defaultFn := funcMap["default"].(func(interface{}, interface{}) interface{})
	assert.Equal(t, "default", defaultFn("default", ""))
	assert.Equal(t, "value", defaultFn("default", "value"))
	
	// 测试 empty 函数
	emptyFn := funcMap["empty"].(func(interface{}) bool)
	assert.True(t, emptyFn(""))
	assert.True(t, emptyFn([]string{}))
	assert.False(t, emptyFn("text"))
	assert.False(t, emptyFn([]string{"item"}))
	
	// 测试 indent 函数
	indentFn := funcMap["indent"].(func(int, string) string)
	result := indentFn(2, "line1\nline2")
	assert.Equal(t, "  line1\n  line2", result)
	
	// 测试 list 函数
	listFn := funcMap["list"].(func([]string) string)
	result = listFn([]string{"item1", "item2"})
	assert.Equal(t, "- item1\n- item2", result)
}