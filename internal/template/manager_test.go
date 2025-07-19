package template

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	
	"github.com/penwyp/catmit/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultManager_LoadTemplate(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "manager-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	
	// 创建GitHub模板
	githubDir := filepath.Join(tmpDir, ".github")
	require.NoError(t, os.MkdirAll(githubDir, 0755))
	
	githubTemplate := `# Pull Request

## Description
{{.CommitMessage}}

## Checklist
- [ ] Tests added
- [ ] Documentation updated
`
	require.NoError(t, os.WriteFile(
		filepath.Join(githubDir, "PULL_REQUEST_TEMPLATE.md"),
		[]byte(githubTemplate),
		0644,
	))
	
	// 创建GitLab模板
	gitlabDir := filepath.Join(tmpDir, ".gitlab", "merge_request_templates")
	require.NoError(t, os.MkdirAll(gitlabDir, 0755))
	
	gitlabTemplate := `## What does this MR do?
{{.Description}}

## Related issues
Closes #{{.IssueNumber}}
`
	require.NoError(t, os.WriteFile(
		filepath.Join(gitlabDir, "Default.md"),
		[]byte(gitlabTemplate),
		0644,
	))
	
	manager := NewDefaultManager(tmpDir)
	
	tests := []struct {
		name     string
		info     *provider.RemoteInfo
		wantErr  bool
		validate func(t *testing.T, tmpl *Template)
	}{
		{
			name: "load github template",
			info: &provider.RemoteInfo{
				Provider: "github",
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				assert.Equal(t, "github", tmpl.Provider)
				assert.Contains(t, tmpl.Content, "## Description")
				assert.Len(t, tmpl.Sections, 3)
				assert.Contains(t, tmpl.Sections, "Pull Request")
				assert.Contains(t, tmpl.Sections, "Description")
				assert.Contains(t, tmpl.Sections, "Checklist")
			},
		},
		{
			name: "load gitlab template",
			info: &provider.RemoteInfo{
				Provider: "gitlab",
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				assert.Equal(t, "gitlab", tmpl.Provider)
				assert.Contains(t, tmpl.Content, "## What does this MR do?")
				assert.Contains(t, tmpl.Content, "{{.IssueNumber}}")
			},
		},
		{
			name: "fallback to github for unknown provider",
			info: &provider.RemoteInfo{
				Provider: "bitbucket",
			},
			wantErr: false,
			validate: func(t *testing.T, tmpl *Template) {
				// 应该回退到GitHub模板
				assert.Equal(t, "bitbucket", tmpl.Provider)
				assert.Contains(t, tmpl.Content, "## Description")
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := manager.LoadTemplate(context.Background(), tt.info)
			
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

func TestDefaultManager_ProcessTemplate(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "process-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	
	// 创建模板
	githubDir := filepath.Join(tmpDir, ".github")
	require.NoError(t, os.MkdirAll(githubDir, 0755))
	
	template := `# {{.CommitTitle}}

## Description
{{.CommitMessage}}

## Changes
- Files changed: {{.FilesCount}}
{{range .ChangedFiles}}- {{.}}
{{end}}

## Checklist
- [ ] Tests added
- [ ] Documentation updated
`
	require.NoError(t, os.WriteFile(
		filepath.Join(githubDir, "PULL_REQUEST_TEMPLATE.md"),
		[]byte(template),
		0644,
	))
	
	manager := NewDefaultManager(tmpDir)
	
	// 加载模板
	tmpl, err := manager.LoadTemplate(context.Background(), &provider.RemoteInfo{
		Provider: "github",
	})
	require.NoError(t, err)
	
	// 准备数据
	data := &TemplateData{
		CommitMessage: "feat: add new feature\n\nThis implements the new feature",
		CommitTitle:   "feat: add new feature",
		ChangedFiles:  []string{"feature.go", "feature_test.go"},
		FilesCount:    2,
		TestsAdded:    true,
		DocsUpdated:   false,
	}
	
	// 处理模板
	result, err := manager.ProcessTemplate(context.Background(), tmpl, data)
	assert.NoError(t, err)
	
	// 验证结果
	assert.Contains(t, result, "# feat: add new feature")
	assert.Contains(t, result, "Files changed: 2")
	assert.Contains(t, result, "- feature.go")
	assert.Contains(t, result, "- feature_test.go")
	assert.Contains(t, result, "- [x] Tests added")
	assert.Contains(t, result, "- [ ] Documentation updated")
}

func TestConfigurableManager(t *testing.T) {
	// 创建主目录和自定义目录
	mainDir, err := os.MkdirTemp("", "main-dir-*")
	require.NoError(t, err)
	defer os.RemoveAll(mainDir)
	
	customDir, err := os.MkdirTemp("", "custom-dir-*")
	require.NoError(t, err)
	defer os.RemoveAll(customDir)
	
	// 在主目录创建默认模板
	githubDir := filepath.Join(mainDir, ".github")
	require.NoError(t, os.MkdirAll(githubDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(githubDir, "PULL_REQUEST_TEMPLATE.md"),
		[]byte("# Default Template"),
		0644,
	))
	
	// 在自定义目录创建自定义模板
	customGithubDir := filepath.Join(customDir, ".github")
	require.NoError(t, os.MkdirAll(customGithubDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(customGithubDir, "PULL_REQUEST_TEMPLATE.md"),
		[]byte("# Custom Template"),
		0644,
	))
	
	// 创建配置
	config := &ManagerConfig{
		TemplateDirs:    []string{customDir},
		DefaultProvider: "github",
		StrictMode:      true,
	}
	
	manager := NewConfigurableManager(mainDir, config)
	
	// 加载模板（应该优先使用自定义目录）
	tmpl, err := manager.LoadTemplate(context.Background(), &provider.RemoteInfo{
		Provider: "github",
	})
	
	assert.NoError(t, err)
	require.NotNil(t, tmpl)
	assert.Contains(t, tmpl.Content, "# Custom Template")
}

func TestFindRepositoryRoot(t *testing.T) {
	// 创建临时目录结构
	tmpDir, err := os.MkdirTemp("", "repo-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	
	// 创建git目录
	gitDir := filepath.Join(tmpDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))
	
	// 创建子目录
	subDir := filepath.Join(tmpDir, "sub", "dir")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	
	// 保存当前目录
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldWd)
	
	// 测试从子目录查找
	require.NoError(t, os.Chdir(subDir))
	
	root, err := FindRepositoryRoot()
	assert.NoError(t, err)
	
	// Resolve symlinks for comparison
	expectedPath, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)
	actualPath, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	
	assert.Equal(t, expectedPath, actualPath)
	
	// 测试不在git仓库中
	noGitDir, err := os.MkdirTemp("", "no-git-*")
	require.NoError(t, err)
	defer os.RemoveAll(noGitDir)
	
	require.NoError(t, os.Chdir(noGitDir))
	
	_, err = FindRepositoryRoot()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in a git repository")
}

func TestCreateTemplateData(t *testing.T) {
	commitMsg := "feat: add feature\n\nDetailed description"
	branch := "feature/new-feature"
	files := []string{"file1.go", "file2.go", "file3_test.go"}
	
	data := CreateTemplateData(commitMsg, branch, files)
	
	assert.Equal(t, commitMsg, data.CommitMessage)
	assert.Equal(t, branch, data.Branch)
	assert.Equal(t, files, data.ChangedFiles)
	assert.Equal(t, 3, data.FilesCount)
	assert.Len(t, data.FileStats, 3)
	
	// 验证文件统计初始化
	for _, file := range files {
		stat, ok := data.FileStats[file]
		assert.True(t, ok)
		assert.Equal(t, file, stat.Path)
	}
}

func TestEnrichTemplateData(t *testing.T) {
	data := &TemplateData{}
	
	info := &provider.RemoteInfo{
		Owner: "penwyp",
		Repo:  "catmit",
	}
	
	EnrichTemplateData(data, info)
	
	assert.Equal(t, "penwyp", data.RepoOwner)
	assert.Equal(t, "catmit", data.RepoName)
	assert.Equal(t, "origin", data.Remote)
	assert.Equal(t, "main", data.BaseBranch)
	
	// 测试已有BaseBranch时不覆盖
	data2 := &TemplateData{
		BaseBranch: "develop",
	}
	EnrichTemplateData(data2, info)
	assert.Equal(t, "develop", data2.BaseBranch)
	
	// 测试nil info
	data3 := &TemplateData{}
	EnrichTemplateData(data3, nil)
	assert.Equal(t, "main", data3.BaseBranch)
	assert.Empty(t, data3.RepoOwner)
}