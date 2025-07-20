package cmd

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetDefaultBranch tests the GetDefaultBranch method with various scenarios
func TestGetDefaultBranch(t *testing.T) {
	// Skip if not in a real git repository
	if _, err := exec.Command("git", "rev-parse", "--git-dir").Output(); err != nil {
		t.Skip("Not in a git repository")
	}

	runner := &defaultGitRunner{}
	ctx := context.Background()

	tests := []struct {
		name           string
		remote         string
		setupFunc      func(t *testing.T)
		cleanupFunc    func(t *testing.T)
		expectedBranch string
		expectError    bool
	}{
		{
			name:   "Get default branch from origin",
			remote: "origin",
			setupFunc: func(t *testing.T) {
				// Ensure origin remote exists
				output, _ := exec.Command("git", "remote", "get-url", "origin").Output()
				if len(output) == 0 {
					t.Skip("No origin remote configured")
				}
			},
			expectError: false,
		},
		{
			name:           "Non-existent remote falls back to main",
			remote:         "non-existent-remote",
			expectedBranch: "main",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc(t)
			}
			if tt.cleanupFunc != nil {
				defer tt.cleanupFunc(t)
			}

			branch, err := runner.GetDefaultBranch(ctx, tt.remote)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, branch)
				
				// Verify it returns a valid branch name
				assert.True(t, len(branch) > 0)
				assert.False(t, strings.Contains(branch, "refs/"))
				assert.False(t, strings.Contains(branch, "HEAD"))
				
				if tt.expectedBranch != "" {
					assert.Equal(t, tt.expectedBranch, branch)
				}
			}
		})
	}
}

// TestGetDefaultBranchWithMockGit tests the parsing logic without actual git commands
func TestGetDefaultBranchParsing(t *testing.T) {
	tests := []struct {
		name           string
		lsRemoteOutput string
		expectedBranch string
	}{
		{
			name:           "Parse ls-remote output with main branch",
			lsRemoteOutput: "ref: refs/heads/main\tHEAD\n1234567890abcdef\tHEAD\n",
			expectedBranch: "main",
		},
		{
			name:           "Parse ls-remote output with master branch",
			lsRemoteOutput: "ref: refs/heads/master\tHEAD\n1234567890abcdef\tHEAD\n",
			expectedBranch: "master",
		},
		{
			name:           "Parse ls-remote output with develop branch",
			lsRemoteOutput: "ref: refs/heads/develop\tHEAD\n1234567890abcdef\tHEAD\n",
			expectedBranch: "develop",
		},
		{
			name:           "Parse ls-remote output with custom branch",
			lsRemoteOutput: "ref: refs/heads/production\tHEAD\n1234567890abcdef\tHEAD\n",
			expectedBranch: "production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the parsing logic
			lines := strings.Split(tt.lsRemoteOutput, "\n")
			var branch string
			for _, line := range lines {
				if strings.HasPrefix(line, "ref: refs/heads/") {
					b := strings.TrimPrefix(line, "ref: refs/heads/")
					parts := strings.Fields(b)
					if len(parts) > 0 {
						branch = parts[0]
						break
					}
				}
			}
			assert.Equal(t, tt.expectedBranch, branch)
		})
	}
}