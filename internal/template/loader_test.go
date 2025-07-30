package template

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileLoader_Load(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		setup    func(dir string) error
		wantErr  bool
		validate func(t *testing.T, tmpl *Template)
	}{
		{
			name:     "github template found",
			provider: "github",
			setup: func(dir string) error {
				githubDir := filepath.Join(dir, ".github")
				if err := os.MkdirAll(githubDir, 0755); err != nil {
					return err
				}
				content := `# Pull Request

## Description
{{.CommitMessage}}

## Changes
- {{.ChangedFiles}}
`
				return os.WriteFile(
					filepath.Join(githubDir, "PULL_REQUEST_TEMPLATE.md"),
					[]byte(content),
					0644,
				)
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				assert.Contains(t, tmpl.Content, "## Description")
				assert.Contains(t, tmpl.Content, "{{.CommitMessage}}")
				assert.Equal(t, "Default", tmpl.Name)
				assert.Equal(t, "github", tmpl.Provider)
			},
		},
		{
			name:     "gitlab template found",
			provider: "gitlab",
			setup: func(dir string) error {
				gitlabDir := filepath.Join(dir, ".gitlab", "merge_request_templates")
				if err := os.MkdirAll(gitlabDir, 0755); err != nil {
					return err
				}
				content := `## What does this MR do?

{{.Description}}

## Related issues

Closes #{{.IssueNumber}}
`
				return os.WriteFile(
					filepath.Join(gitlabDir, "Default.md"),
					[]byte(content),
					0644,
				)
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				assert.Contains(t, tmpl.Content, "## What does this MR do?")
				assert.Contains(t, tmpl.Content, "{{.Description}}")
				assert.Equal(t, "Default", tmpl.Name)
				assert.Equal(t, "gitlab", tmpl.Provider)
			},
		},
		{
			name:     "gitea template found",
			provider: "gitea",
			setup: func(dir string) error {
				giteaDir := filepath.Join(dir, ".gitea")
				if err := os.MkdirAll(giteaDir, 0755); err != nil {
					return err
				}
				content := `# PR Title

[Description]

## Checklist
- [ ] Tests added
- [ ] Documentation updated
`
				return os.WriteFile(
					filepath.Join(giteaDir, "PULL_REQUEST_TEMPLATE.md"),
					[]byte(content),
					0644,
				)
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				assert.Contains(t, tmpl.Content, "## Checklist")
				assert.Contains(t, tmpl.Content, "[Description]")
			},
		},
		{
			name:     "template not found",
			provider: "github",
			setup:    func(dir string) error { return nil },
			wantErr:  true,
		},
		{
			name:     "multiple templates - use first",
			provider: "github",
			setup: func(dir string) error {
				githubDir := filepath.Join(dir, ".github", "PULL_REQUEST_TEMPLATE")
				if err := os.MkdirAll(githubDir, 0755); err != nil {
					return err
				}

				// Create multiple templates
				templates := map[string]string{
					"bug_fix.md": "# Bug Fix\n\n{{.Description}}",
					"feature.md": "# Feature\n\n{{.Description}}",
					"default.md": "# Default\n\n{{.Description}}",
				}

				for name, content := range templates {
					if err := os.WriteFile(
						filepath.Join(githubDir, name),
						[]byte(content),
						0644,
					); err != nil {
						return err
					}
				}
				return nil
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				// Should load the first found template
				assert.NotEmpty(t, tmpl.Content)
				assert.Contains(t, tmpl.Content, "{{.Description}}")
			},
		},
		{
			name:     "case insensitive file names",
			provider: "github",
			setup: func(dir string) error {
				return os.WriteFile(
					filepath.Join(dir, "pull_request_template.md"),
					[]byte("# Template\n\n{{.CommitMessage}}"),
					0644,
				)
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				assert.Contains(t, tmpl.Content, "# Template")
			},
		},
		{
			name:     "unknown provider falls back to github paths",
			provider: "unknown",
			setup: func(dir string) error {
				return os.WriteFile(
					filepath.Join(dir, "PULL_REQUEST_TEMPLATE.md"),
					[]byte("# Generic Template"),
					0644,
				)
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				assert.Contains(t, tmpl.Content, "# Generic Template")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir, err := os.MkdirTemp("", "template-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Execute setup
			if tt.setup != nil {
				require.NoError(t, tt.setup(tmpDir))
			}

			// Create loader and load template
			loader := NewFileLoader(tmpDir)
			tmpl, err := loader.Load(context.Background(), tt.provider)

			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrTemplateNotFound)
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

func TestFileLoader_ListTemplates(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "template-list-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Setup multiple templates
	githubDir := filepath.Join(tmpDir, ".github", "PULL_REQUEST_TEMPLATE")
	require.NoError(t, os.MkdirAll(githubDir, 0755))

	templates := map[string]string{
		"bug_fix.md": "# Bug Fix",
		"feature.md": "# Feature",
		"hotfix.md":  "# Hotfix",
	}

	for name, content := range templates {
		err := os.WriteFile(filepath.Join(githubDir, name), []byte(content), 0644)
		require.NoError(t, err)
	}

	// List templates
	loader := NewFileLoader(tmpDir)
	list, err := loader.ListTemplates(context.Background(), "github")

	assert.NoError(t, err)
	assert.Len(t, list, 3)

	// Verify all templates are found
	names := make(map[string]bool)
	for _, tmpl := range list {
		names[tmpl.Name] = true
	}

	assert.True(t, names["Bug Fix"])
	assert.True(t, names["Feature"])
	assert.True(t, names["Hotfix"])
}

func TestCachedLoader(t *testing.T) {
	// Create temporary directory and template
	tmpDir, err := os.MkdirTemp("", "cached-loader-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	githubDir := filepath.Join(tmpDir, ".github")
	require.NoError(t, os.MkdirAll(githubDir, 0755))

	templatePath := filepath.Join(githubDir, "PULL_REQUEST_TEMPLATE.md")
	originalContent := "# Original Template"
	require.NoError(t, os.WriteFile(templatePath, []byte(originalContent), 0644))

	// Create cached loader
	fileLoader := NewFileLoader(tmpDir)
	cachedLoader := NewCachedLoader(fileLoader)

	// First load
	tmpl1, err := cachedLoader.Load(context.Background(), "github")
	assert.NoError(t, err)
	assert.Equal(t, originalContent, tmpl1.Content)

	// Modify file content
	newContent := "# Modified Template"
	require.NoError(t, os.WriteFile(templatePath, []byte(newContent), 0644))

	// Second load (should get from cache)
	tmpl2, err := cachedLoader.Load(context.Background(), "github")
	assert.NoError(t, err)
	assert.Equal(t, originalContent, tmpl2.Content)

	// Clear cache
	cachedLoader.ClearCache()

	// Third load (should get new content)
	tmpl3, err := cachedLoader.Load(context.Background(), "github")
	assert.NoError(t, err)
	assert.Equal(t, newContent, tmpl3.Content)
}

func TestExtractTemplateName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"PULL_REQUEST_TEMPLATE.md", "Default"},
		{"pull_request_template.md", "Default"},
		{"merge_request_template.md", "Default"},
		{".github/PULL_REQUEST_TEMPLATE/bug_fix.md", "Bug Fix"},
		{".gitlab/merge_request_templates/feature-request.md", "Feature Request"},
		{"some/path/my_custom_template.md", "My Custom Template"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			name := extractTemplateName(tt.path)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestInferProviderFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{".github/PULL_REQUEST_TEMPLATE.md", "github"},
		{".gitlab/merge_request_templates/default.md", "gitlab"},
		{".gitea/PULL_REQUEST_TEMPLATE.md", "gitea"},
		{".bitbucket/PULLREQUEST_TEMPLATE.md", "bitbucket"},
		{"merge_request_template.md", "gitlab"},
		{"pullrequest_template.md", "bitbucket"},
		{"some_random_template.md", "github"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			provider := inferProviderFromPath(tt.path)
			assert.Equal(t, tt.expected, provider)
		})
	}
}
