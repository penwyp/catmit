package pr

import (
	"context"
	"strings"
	"testing"

	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Test checkGitHubPR with cross-repository scenario
func TestCreator_checkGitHubPR_CrossRepo(t *testing.T) {
	mockRunner := new(MockCommandRunner)
	creator := &Creator{
		commandRunner: mockRunner,
	}

	ctx := context.Background()
	branch := "feature-branch"
	remoteInfo := provider.RemoteInfo{
		Provider: "github",
		Host:     "github.com",
		Owner:    "upstream-owner",
		Repo:     "upstream-repo",
	}

	// Test case 1: PR exists
	t.Run("PR exists", func(t *testing.T) {
		mockRunner.On("Run", ctx, "gh",
			"pr", "list", "--head", branch, "--json", "url,state",
			"-R", "upstream-owner/upstream-repo",
		).Return([]byte(`[{"url":"https://github.com/upstream-owner/upstream-repo/pull/123","state":"OPEN"}]`), nil).Once()

		exists, prURL, err := creator.checkGitHubPR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "https://github.com/upstream-owner/upstream-repo/pull/123", prURL)
	})

	// Test case 2: No PR exists
	t.Run("No PR exists", func(t *testing.T) {
		mockRunner.On("Run", ctx, "gh",
			"pr", "list", "--head", branch, "--json", "url,state",
			"-R", "upstream-owner/upstream-repo",
		).Return([]byte(`[]`), nil).Once()

		exists, prURL, err := creator.checkGitHubPR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.False(t, exists)
		assert.Empty(t, prURL)
	})

	mockRunner.AssertExpectations(t)
}

// Test checkGitLabMR with cross-repository scenario
func TestCreator_checkGitLabMR_CrossRepo(t *testing.T) {
	mockRunner := new(MockCommandRunner)
	creator := &Creator{
		commandRunner: mockRunner,
	}

	ctx := context.Background()
	branch := "feature-branch"
	remoteInfo := provider.RemoteInfo{
		Provider: "gitlab",
		Host:     "gitlab.com",
		Owner:    "upstream-owner",
		Repo:     "upstream-repo",
	}

	// Test case: MR exists
	t.Run("MR exists", func(t *testing.T) {
		// First call to list MRs
		mockRunner.On("Run", ctx, "glab",
			"mr", "list", "--source-branch", branch,
			"-R", "upstream-owner/upstream-repo",
		).Return([]byte("!42  Feature MR  (feature-branch -> main)"), nil).Once()

		// Second call to get MR details
		mockRunner.On("Run", ctx, "glab",
			"mr", "view", "42", "--output", "json",
			"-R", "upstream-owner/upstream-repo",
		).Return([]byte(`{"web_url":"https://gitlab.com/upstream-owner/upstream-repo/-/merge_requests/42"}`), nil).Once()

		exists, mrURL, err := creator.checkGitLabMR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "https://gitlab.com/upstream-owner/upstream-repo/-/merge_requests/42", mrURL)
	})

	mockRunner.AssertExpectations(t)
}

// Test checkGiteaPR with cross-repository scenario
func TestCreator_checkGiteaPR_CrossRepo(t *testing.T) {
	mockRunner := new(MockCommandRunner)
	creator := &Creator{
		commandRunner: mockRunner,
	}

	ctx := context.Background()
	branch := "feature-branch"
	remoteInfo := provider.RemoteInfo{
		Provider: "gitea",
		Host:     "gitea.com",
		Owner:    "upstream-owner",
		Repo:     "upstream-repo",
	}

	// Test case: PR exists
	t.Run("PR exists", func(t *testing.T) {
		mockRunner.On("Run", ctx, "tea",
			"pulls", "--state", "open", "--repo", "upstream-owner/upstream-repo",
		).Return([]byte("#15 Add new feature (feature-branch -> main)\n#16 Other PR (other-branch -> main)"), nil).Once()

		exists, prURL, err := creator.checkGiteaPR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "https://gitea.com/upstream-owner/upstream-repo/pulls/15", prURL)
	})

	// Test case: No PR exists
	t.Run("No PR exists", func(t *testing.T) {
		mockRunner.On("Run", ctx, "tea",
			"pulls", "--state", "open", "--repo", "upstream-owner/upstream-repo",
		).Return([]byte("#16 Other PR (other-branch -> main)"), nil).Once()

		exists, prURL, err := creator.checkGiteaPR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.False(t, exists)
		assert.Empty(t, prURL)
	})

	mockRunner.AssertExpectations(t)
}

// Test CheckExists with different remotes
func TestCreator_CheckExists_DifferentRemotes(t *testing.T) {
	tests := []struct {
		name         string
		remote       string
		expectedRepo string
	}{
		{
			name:         "origin remote",
			remote:       "origin",
			expectedRepo: "my-fork/repo",
		},
		{
			name:         "upstream remote",
			remote:       "upstream",
			expectedRepo: "original-owner/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockGit := new(MockGitRunner)
			mockProvider := new(MockProviderDetector)
			mockCLI := new(MockCLIDetector)
			mockRunner := new(MockCommandRunner)

			creator := &Creator{
				git:              mockGit,
				providerDetector: mockProvider,
				cliDetector:      mockCLI,
				commandRunner:    mockRunner,
			}

			ctx := context.Background()
			options := CreateOptions{
				Remote: tt.remote,
			}

			// Mock the flow
			mockGit.On("GetRemoteURL", ctx, tt.remote).Return("https://github.com/"+tt.expectedRepo+".git", nil).Once()

			parts := strings.Split(tt.expectedRepo, "/")
			remoteInfo := provider.RemoteInfo{
				Provider: "github",
				Host:     "github.com",
				Owner:    parts[0],
				Repo:     parts[1],
			}
			mockProvider.On("DetectFromRemote", ctx, mock.Anything).Return(remoteInfo, nil).Once()

			mockCLI.On("DetectCLI", ctx, "github").Return(cli.CLIStatus{
				Installed:     true,
				Authenticated: true,
				Version:       "2.40.0",
			}, nil).Once()

			mockGit.On("GetCurrentBranch", ctx).Return("feature-branch", nil).Once()

			// The key assertion: verify it checks the correct repository
			mockRunner.On("Run", ctx, "gh",
				"pr", "list", "--head", "feature-branch", "--json", "url,state",
				"-R", tt.expectedRepo,
			).Return([]byte(`[]`), nil).Once()

			exists, _, err := creator.CheckExists(ctx, options)
			assert.NoError(t, err)
			assert.False(t, exists)

			// Verify all expectations were met
			mockGit.AssertExpectations(t)
			mockProvider.AssertExpectations(t)
			mockCLI.AssertExpectations(t)
			mockRunner.AssertExpectations(t)
		})
	}
}
