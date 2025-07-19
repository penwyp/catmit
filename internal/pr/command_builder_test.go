package pr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCommandBuilder_BuildGitHubPRCommand 测试GitHub PR命令构建
func TestCommandBuilder_BuildGitHubPRCommand(t *testing.T) {
	tests := []struct {
		name           string
		options        PROptions
		expectedCmd    string
		expectedArgs   []string
		expectedError  bool
	}{
		{
			name: "Basic PR with default branch",
			options: PROptions{
				Title:       "feat: add new feature",
				Body:        "This PR adds a new feature\n\n- Feature A\n- Feature B",
				BaseBranch:  "main",
				Draft:       false,
			},
			expectedCmd: "gh",
			expectedArgs: []string{
				"pr", "create",
				"--title", "feat: add new feature",
				"--body", "This PR adds a new feature\n\n- Feature A\n- Feature B",
				"--base", "main",
			},
			expectedError: false,
		},
		{
			name: "Draft PR with custom branch",
			options: PROptions{
				Title:       "WIP: experimental feature",
				Body:        "Work in progress",
				BaseBranch:  "develop",
				Draft:       true,
			},
			expectedCmd: "gh",
			expectedArgs: []string{
				"pr", "create",
				"--title", "WIP: experimental feature",
				"--body", "Work in progress",
				"--base", "develop",
				"--draft",
			},
			expectedError: false,
		},
		{
			name: "PR with assignees and labels",
			options: PROptions{
				Title:       "fix: resolve issue #123",
				Body:        "Fixes #123",
				BaseBranch:  "main",
				Assignees:   []string{"user1", "user2"},
				Labels:      []string{"bug", "priority:high"},
			},
			expectedCmd: "gh",
			expectedArgs: []string{
				"pr", "create",
				"--title", "fix: resolve issue #123",
				"--body", "Fixes #123",
				"--base", "main",
				"--assignee", "user1,user2",
				"--label", "bug,priority:high",
			},
			expectedError: false,
		},
		{
			name: "PR with reviewers",
			options: PROptions{
				Title:       "feat: new API endpoint",
				Body:        "Add new API endpoint",
				BaseBranch:  "main",
				Reviewers:   []string{"reviewer1", "reviewer2"},
			},
			expectedCmd: "gh",
			expectedArgs: []string{
				"pr", "create",
				"--title", "feat: new API endpoint",
				"--body", "Add new API endpoint",
				"--base", "main",
				"--reviewer", "reviewer1,reviewer2",
			},
			expectedError: false,
		},
		{
			name: "PR with fill option",
			options: PROptions{
				Fill:       true,
				BaseBranch: "main",
			},
			expectedCmd: "gh",
			expectedArgs: []string{
				"pr", "create",
				"--fill",
				"--base", "main",
				"--draft=false",
			},
			expectedError: false,
		},
		{
			name: "Missing required fields",
			options: PROptions{
				Title: "Test PR",
				// Missing BaseBranch
			},
			expectedCmd:   "",
			expectedArgs:  nil,
			expectedError: true,
		},
	}

	builder := NewCommandBuilder()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, err := builder.BuildGitHubPRCommand(tt.options)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCmd, cmd)
				assert.Equal(t, tt.expectedArgs, args)
			}
		})
	}
}

// TestCommandBuilder_BuildGiteaPRCommand 测试Gitea PR命令构建
func TestCommandBuilder_BuildGiteaPRCommand(t *testing.T) {
	tests := []struct {
		name           string
		options        PROptions
		expectedCmd    string
		expectedArgs   []string
		expectedError  bool
	}{
		{
			name: "Basic Gitea PR",
			options: PROptions{
				Title:       "feat: add new feature",
				Body:        "This PR adds a new feature",
				BaseBranch:  "main",
				HeadBranch:  "feature-branch",
			},
			expectedCmd: "tea",
			expectedArgs: []string{
				"pr", "create",
				"--title", "feat: add new feature",
				"--description", "This PR adds a new feature",
				"--base", "main",
				"--head", "feature-branch",
			},
			expectedError: false,
		},
		{
			name: "Gitea PR with assignees and labels",
			options: PROptions{
				Title:       "fix: bug fix",
				Body:        "Fix critical bug",
				BaseBranch:  "main",
				HeadBranch:  "bugfix",
				Assignees:   []string{"user1"},
				Labels:      []string{"bug", "urgent"},
			},
			expectedCmd: "tea",
			expectedArgs: []string{
				"pr", "create",
				"--title", "fix: bug fix",
				"--description", "Fix critical bug",
				"--base", "main",
				"--head", "bugfix",
				"--assignees", "user1",
				"--labels", "bug,urgent",
			},
			expectedError: false,
		},
		{
			name: "Gitea PR with milestone",
			options: PROptions{
				Title:       "feat: milestone feature",
				Body:        "Feature for v1.0",
				BaseBranch:  "main",
				HeadBranch:  "feature",
				Milestone:   "v1.0",
			},
			expectedCmd: "tea",
			expectedArgs: []string{
				"pr", "create",
				"--title", "feat: milestone feature",
				"--description", "Feature for v1.0",
				"--base", "main",
				"--head", "feature",
				"--milestone", "v1.0",
			},
			expectedError: false,
		},
		{
			name: "Missing head branch for Gitea",
			options: PROptions{
				Title:      "Test PR",
				BaseBranch: "main",
				// Missing HeadBranch (required for Gitea)
			},
			expectedCmd:   "",
			expectedArgs:  nil,
			expectedError: true,
		},
	}

	builder := NewCommandBuilder()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, args, err := builder.BuildGiteaPRCommand(tt.options)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCmd, cmd)
				assert.Equal(t, tt.expectedArgs, args)
			}
		})
	}
}

// TestCommandBuilder_BuildCommand 测试通用命令构建
func TestCommandBuilder_BuildCommand(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		options        PROptions
		expectedCmd    string
		expectedError  bool
	}{
		{
			name:     "GitHub provider",
			provider: "github",
			options: PROptions{
				Title:      "Test PR",
				Body:       "Test body",
				BaseBranch: "main",
			},
			expectedCmd:   "gh",
			expectedError: false,
		},
		{
			name:     "Gitea provider",
			provider: "gitea",
			options: PROptions{
				Title:      "Test PR",
				Body:       "Test body",
				BaseBranch: "main",
				HeadBranch: "feature",
			},
			expectedCmd:   "tea",
			expectedError: false,
		},
		{
			name:     "GitLab provider",
			provider: "gitlab",
			options: PROptions{
				Title:      "Test MR",
				Body:       "Test body",
				BaseBranch: "main",
			},
			expectedCmd:   "glab",
			expectedError: false,
		},
		{
			name:     "Unknown provider",
			provider: "bitbucket",
			options: PROptions{
				Title:      "Test PR",
				BaseBranch: "main",
			},
			expectedCmd:   "",
			expectedError: true,
		},
	}

	builder := NewCommandBuilder()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := builder.BuildCommand(tt.provider, tt.options)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCmd, cmd)
			}
		})
	}
}

// TestCommandBuilder_ParseGitHubPROutput 测试解析GitHub PR输出
func TestCommandBuilder_ParseGitHubPROutput(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		expectedURL   string
		expectedError bool
	}{
		{
			name:          "Standard PR creation output",
			output:        "Creating pull request for feature-branch into main in owner/repo\n\nhttps://github.com/owner/repo/pull/123\n",
			expectedURL:   "https://github.com/owner/repo/pull/123",
			expectedError: false,
		},
		{
			name:          "PR already exists",
			output:        "a pull request for branch \"feature-branch\" into branch \"main\" already exists:\nhttps://github.com/owner/repo/pull/456\n",
			expectedURL:   "https://github.com/owner/repo/pull/456",
			expectedError: false,
		},
		{
			name:          "URL in middle of output",
			output:        "Some text before\nPull request created: https://github.com/owner/repo/pull/789\nSome text after",
			expectedURL:   "https://github.com/owner/repo/pull/789",
			expectedError: false,
		},
		{
			name:          "No URL in output",
			output:        "Error: authentication required",
			expectedURL:   "",
			expectedError: true,
		},
	}

	builder := NewCommandBuilder()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := builder.ParseGitHubPROutput(tt.output)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
			}
		})
	}
}

// TestCommandBuilder_ParseGiteaPROutput 测试解析Gitea PR输出
func TestCommandBuilder_ParseGiteaPROutput(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		expectedURL   string
		expectedError bool
	}{
		{
			name:          "Standard tea PR creation",
			output:        "Created PR #42: https://gitea.example.com/owner/repo/pulls/42\n",
			expectedURL:   "https://gitea.example.com/owner/repo/pulls/42",
			expectedError: false,
		},
		{
			name:          "Alternative format",
			output:        "Pull request created successfully\nURL: https://gitea.io/org/project/pulls/123",
			expectedURL:   "https://gitea.io/org/project/pulls/123",
			expectedError: false,
		},
		{
			name:          "No URL in output",
			output:        "Error: permission denied",
			expectedURL:   "",
			expectedError: true,
		},
	}

	builder := NewCommandBuilder()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := builder.ParseGiteaPROutput(tt.output)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
			}
		})
	}
}