package pr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCommandBuilder_BuildGitLabMRCommand 测试GitLab MR命令构建
func TestCommandBuilder_BuildGitLabMRCommand(t *testing.T) {
	tests := []struct {
		name           string
		options        PROptions
		expectedCmd    string
		expectedArgs   []string
		expectedError  bool
	}{
		{
			name: "Basic GitLab MR",
			options: PROptions{
				Title:       "feat: add new feature",
				Body:        "This MR adds a new feature",
				BaseBranch:  "main",
			},
			expectedCmd: "glab",
			expectedArgs: []string{
				"mr", "create",
				"--title", "feat: add new feature",
				"--description", "This MR adds a new feature",
				"--target-branch", "main",
				"--remove-source-branch=false",
			},
			expectedError: false,
		},
		{
			name: "GitLab MR with fill option",
			options: PROptions{
				Fill:       true,
				BaseBranch: "main",
				Draft:      false,
			},
			expectedCmd: "glab",
			expectedArgs: []string{
				"mr", "create",
				"--fill",
				"--target-branch", "main",
				"--remove-source-branch=false",
			},
			expectedError: false,
		},
		{
			name: "Draft GitLab MR",
			options: PROptions{
				Title:       "WIP: experimental feature",
				Body:        "Work in progress",
				BaseBranch:  "develop",
				Draft:       true,
			},
			expectedCmd: "glab",
			expectedArgs: []string{
				"mr", "create",
				"--title", "WIP: experimental feature",
				"--description", "Work in progress",
				"--target-branch", "develop",
				"--draft",
				"--remove-source-branch=false",
			},
			expectedError: false,
		},
		{
			name: "GitLab MR with assignees, labels, and reviewers",
			options: PROptions{
				Title:       "fix: resolve issue #123",
				Body:        "Fixes #123",
				BaseBranch:  "main",
				Assignees:   []string{"user1", "user2"},
				Labels:      []string{"bug", "priority::high"},
				Reviewers:   []string{"reviewer1", "reviewer2"},
			},
			expectedCmd: "glab",
			expectedArgs: []string{
				"mr", "create",
				"--title", "fix: resolve issue #123",
				"--description", "Fixes #123",
				"--target-branch", "main",
				"--assignee", "user1,user2",
				"--label", "bug,priority::high",
				"--reviewer", "reviewer1,reviewer2",
				"--remove-source-branch=false",
			},
			expectedError: false,
		},
		{
			name: "GitLab MR with milestone",
			options: PROptions{
				Title:       "feat: milestone feature",
				Body:        "Feature for milestone",
				BaseBranch:  "main",
				Milestone:   "v1.0.0",
			},
			expectedCmd: "glab",
			expectedArgs: []string{
				"mr", "create",
				"--title", "feat: milestone feature",
				"--description", "Feature for milestone",
				"--target-branch", "main",
				"--milestone", "v1.0.0",
				"--remove-source-branch=false",
			},
			expectedError: false,
		},
		{
			name: "Missing required fields",
			options: PROptions{
				Title: "Test MR",
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
			cmd, args, err := builder.BuildGitLabMRCommand(tt.options)
			
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

// TestCommandBuilder_ParseGitLabMROutput 测试解析GitLab MR输出
func TestCommandBuilder_ParseGitLabMROutput(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		expectedURL   string
		expectedError bool
	}{
		{
			name:          "Standard glab MR creation",
			output:        "Created merge request !42: https://gitlab.com/owner/repo/-/merge_requests/42\n",
			expectedURL:   "https://gitlab.com/owner/repo/-/merge_requests/42",
			expectedError: false,
		},
		{
			name:          "Alternative format",
			output:        "Merge request created successfully\nURL: https://gitlab.example.com/org/project/-/merge_requests/123",
			expectedURL:   "https://gitlab.example.com/org/project/-/merge_requests/123",
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
			url, err := builder.ParseGitLabMROutput(tt.output)
			
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
			}
		})
	}
}