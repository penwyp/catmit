package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/penwyp/catmit/internal/provider"
	"github.com/penwyp/catmit/internal/template"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_PRCreation_WithTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	tests := []struct {
		name             string
		provider         string
		templateContent  string
		templatePath     string
		commitMessage    string
		expectedContains []string
	}{
		{
			name:     "github with template",
			provider: "github",
			templateContent: `# Pull Request

## Description
{{.CommitMessage}}

## Changes
- Files changed: {{.FilesCount}}
{{range .ChangedFiles}}- {{.}}
{{end}}

## Checklist
- [ ] Tests added
- [ ] Documentation updated
- [ ] Lint passed
`,
			templatePath:  ".github/PULL_REQUEST_TEMPLATE.md",
			commitMessage: "feat: add new feature\n\nThis adds an awesome new feature",
			expectedContains: []string{
				"## Description",
				"feat: add new feature",
				"This adds an awesome new feature",
				"## Changes",
				"Files changed: 2",
				"- [x] Tests added", // Should be checked if test files present
				"- [x] Lint passed", // Should be auto-checked
			},
		},
		{
			name:     "gitlab with template",
			provider: "gitlab",
			templateContent: `## What does this MR do?

{{.CommitBody}}

## Related issues

{{if .IssueNumber}}Closes #{{.IssueNumber}}{{else}}N/A{{end}}

## Author's checklist

- [ ] Tests are passing
- [ ] Documentation is updated
`,
			templatePath:  ".gitlab/merge_request_templates/Default.md",
			commitMessage: "fix: resolve issue #123\n\nThis fixes the bug reported in issue 123",
			expectedContains: []string{
				"## What does this MR do?",
				"This fixes the bug reported in issue 123",
				"Closes #123",
				"- [ ] Tests are passing",
			},
		},
		{
			name:     "template with all variables",
			provider: "github",
			templateContent: `# {{.CommitTitle}}

**Branch**: {{.Branch}}
**Base**: {{.BaseBranch}}
**Remote**: {{.Remote}}

## Description
{{.CommitBody}}

## Changes Summary
{{.ChangesSummary}}

## Files Changed ({{.FilesCount}})
{{range .ChangedFiles}}- {{.}}
{{end}}

## Statistics
- Added lines: {{.AddedLines}}
- Deleted lines: {{.DeletedLines}}

## Metadata
- Issue: {{if .IssueNumber}}#{{.IssueNumber}}{{else}}None{{end}}
- Breaking Change: {{if .BreakingChange}}Yes{{else}}No{{end}}
- Tests Added: {{if .TestsAdded}}Yes{{else}}No{{end}}
- Docs Updated: {{if .DocsUpdated}}Yes{{else}}No{{end}}
`,
			templatePath:  ".github/PULL_REQUEST_TEMPLATE.md",
			commitMessage: "feat!: breaking change\n\nThis is a breaking change",
			expectedContains: []string{
				"# feat!: breaking change",
				"**Branch**: feature/test",
				"**Base**: main",
				"## Description",
				"This is a breaking change",
				"Breaking Change: Yes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory
			tmpDir, err := os.MkdirTemp("", "pr-template-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tmpDir)

			// Initialize git repository
			ctx := context.Background()
			err = setupGitRepo(tmpDir)
			require.NoError(t, err)

			// Change to test directory
			oldWd, err := os.Getwd()
			require.NoError(t, err)
			defer func() { _ = os.Chdir(oldWd) }()
			require.NoError(t, os.Chdir(tmpDir))

			// Create template file
			templateDir := filepath.Dir(tt.templatePath)
			if templateDir != "." {
				require.NoError(t, os.MkdirAll(templateDir, 0755))
			}
			require.NoError(t, os.WriteFile(tt.templatePath, []byte(tt.templateContent), 0644))

			// Create test files
			require.NoError(t, os.WriteFile("feature.go", []byte("package main\n\nfunc Feature() {}\n"), 0644))
			require.NoError(t, os.WriteFile("feature_test.go", []byte("package main\n\nimport \"testing\"\n\nfunc TestFeature(t *testing.T) {}\n"), 0644))

			// Add and commit files
			runCommand(t, "git", "add", ".")
			runCommand(t, "git", "commit", "-m", "initial commit")

			// Create feature branch
			runCommand(t, "git", "checkout", "-b", "feature/test")

			// Modify files
			require.NoError(t, os.WriteFile("feature.go", []byte("package main\n\nfunc Feature() {\n\t// New feature\n}\n"), 0644))
			require.NoError(t, os.WriteFile("feature_test.go", []byte("package main\n\nimport \"testing\"\n\nfunc TestFeature(t *testing.T) {\n\t// Test updated\n}\n"), 0644))

			// Test template processing
			manager := template.NewDefaultManager(tmpDir)

			// Load template
			tmpl, err := manager.LoadTemplate(ctx, &provider.RemoteInfo{
				Provider: tt.provider,
			})
			require.NoError(t, err)
			require.NotNil(t, tmpl)

			// Prepare template data
			templateData := &template.TemplateData{
				CommitMessage:  tt.commitMessage,
				Branch:         "feature/test",
				BaseBranch:     "main",
				Remote:         "origin",
				ChangedFiles:   []string{"feature.go", "feature_test.go"},
				FilesCount:     2,
				AddedLines:     2,
				DeletedLines:   0,
				ChangesSummary: "Added new feature implementation",
				IssueNumber:    extractIssueFromMessage(tt.commitMessage),
				BreakingChange: containsBreakingChange(tt.commitMessage),
				TestsAdded:     true, // We have test files
				DocsUpdated:    false,
			}

			// Process template
			result, err := manager.ProcessTemplate(ctx, tmpl, templateData)
			require.NoError(t, err)

			// Verify that the result contains the expected content
			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected, "Result should contain: %s", expected)
			}

			// Verify that template variables are replaced correctly
			assert.NotContains(t, result, "{{.", "All template variables should be replaced")
			assert.NotContains(t, result, "[BracketVar]", "All bracket variables should be replaced")
		})
	}
}

// TestE2E_PRTemplate_Integration tests the full integration of template and PR creation
func TestE2E_PRTemplate_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if GitHub CLI is available
	if !isCommandAvailable("gh") {
		t.Skip("GitHub CLI (gh) not available")
	}

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "pr-template-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize git repository
	ctx := context.Background()
	err = setupGitRepo(tmpDir)
	require.NoError(t, err)

	// Change directory
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldWd) }()
	require.NoError(t, os.Chdir(tmpDir))

	// Create GitHub PR template
	githubDir := filepath.Join(tmpDir, ".github")
	require.NoError(t, os.MkdirAll(githubDir, 0755))

	templateContent := `# Pull Request: {{.CommitTitle}}

## Description
{{.CommitMessage}}

## Type of Change
- [x] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)

## How Has This Been Tested?
{{if .TestsAdded}}Tests have been added/updated.{{else}}Manual testing performed.{{end}}

## Checklist:
- [x] My code follows the style guidelines of this project
- [x] I have performed a self-review of my own code
- [ ] I have commented my code, particularly in hard-to-understand areas
- [x] I have made corresponding changes to the documentation
- [x] My changes generate no new warnings
`

	require.NoError(t, os.WriteFile(
		filepath.Join(githubDir, "PULL_REQUEST_TEMPLATE.md"),
		[]byte(templateContent),
		0644,
	))

	// Create test file
	require.NoError(t, os.WriteFile("main.go", []byte("package main\n\nfunc main() {}\n"), 0644))
	runCommand(t, "git", "add", ".")
	runCommand(t, "git", "commit", "-m", "initial commit")

	// Create feature branch
	runCommand(t, "git", "checkout", "-b", "fix/issue-42")

	// Modify file
	require.NoError(t, os.WriteFile("main.go", []byte("package main\n\nfunc main() {\n\t// Fixed issue #42\n}\n"), 0644))
	runCommand(t, "git", "add", ".")

	// Use catmit to commit (here we simulate the core logic)
	commitMessage := "fix: resolve memory leak in worker\n\nThis fixes issue #42 where workers were not releasing memory properly."

	// Create template manager
	manager := template.NewDefaultManager(tmpDir)

	// Load template
	tmpl, err := manager.LoadTemplate(ctx, &provider.RemoteInfo{Provider: "github"})
	require.NoError(t, err)

	// Prepare data
	templateData := &template.TemplateData{
		CommitMessage: commitMessage,
		CommitTitle:   "fix: resolve memory leak in worker",
		CommitBody:    "This fixes issue #42 where workers were not releasing memory properly.",
		Branch:        "fix/issue-42",
		BaseBranch:    "main",
		ChangedFiles:  []string{"main.go"},
		FilesCount:    1,
		IssueNumber:   "42",
		TestsAdded:    false,
		DocsUpdated:   true,
	}

	// Process template
	prBody, err := manager.ProcessTemplate(ctx, tmpl, templateData)
	require.NoError(t, err)

	// Verify PR body
	assert.Contains(t, prBody, "# Pull Request: fix: resolve memory leak in worker")
	assert.Contains(t, prBody, "This fixes issue #42")
	assert.Contains(t, prBody, "- [x] Bug fix")
	assert.Contains(t, prBody, "Manual testing performed")
	assert.Contains(t, prBody, "- [x] I have made corresponding changes to the documentation")
}

// Helper functions

func extractIssueFromMessage(message string) string {
	// Simple issue extraction logic
	if containsString(message, "#123") {
		return "123"
	}
	if containsString(message, "issue 123") {
		return "123"
	}
	if containsString(message, "#42") {
		return "42"
	}
	return ""
}

func containsBreakingChange(message string) bool {
	return containsString(message, "!:") || containsString(message, "breaking")
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// setupGitRepo initializes a git repository in the given directory
func setupGitRepo(dir string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return err
	}

	// Configure user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	return cmd.Run()
}

// runCommand runs a command in the current directory
func runCommand(t *testing.T, command string, args ...string) {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\nOutput: %s", command, args, err, out)
	}
}

// isCommandAvailable checks if a command is available in PATH
func isCommandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}
