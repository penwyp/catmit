package pr

import (
	"context"
	"fmt"
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
		jsonOutput := `[{"iid":42,"web_url":"https://gitlab.com/upstream-owner/upstream-repo/-/merge_requests/42","state":"opened","source_branch":"feature-branch"}]`
		mockRunner.On("Run", ctx, "glab",
			"mr", "list", "--output", "json", "--source-branch", branch,
			"-R", "upstream-owner/upstream-repo",
		).Return([]byte(jsonOutput), nil).Once()

		exists, mrURL, err := creator.checkGitLabMR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "https://gitlab.com/upstream-owner/upstream-repo/-/merge_requests/42", mrURL)
	})

	// Test case: No MR exists
	t.Run("No MR exists", func(t *testing.T) {
		jsonOutput := `[]`
		mockRunner.On("Run", ctx, "glab",
			"mr", "list", "--output", "json", "--source-branch", branch,
			"-R", "upstream-owner/upstream-repo",
		).Return([]byte(jsonOutput), nil).Once()

		exists, mrURL, err := creator.checkGitLabMR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.False(t, exists)
		assert.Empty(t, mrURL)
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
		jsonOutput := `[{"index":15,"url":"https://gitea.com/upstream-owner/upstream-repo/pulls/15","head":{"name":"feature-branch","repo":{"owner":{"login":"upstream-owner"}}}},{"index":16,"url":"https://gitea.com/upstream-owner/upstream-repo/pulls/16","head":{"name":"other-branch","repo":{"owner":{"login":"upstream-owner"}}}}]`
		mockRunner.On("Run", ctx, "tea",
			"pulls", "list", "--output", "json", "--fields", "index,head,url", "--state", "open", "--repo", "upstream-owner/upstream-repo",
		).Return([]byte(jsonOutput), nil).Once()

		exists, prURL, err := creator.checkGiteaPR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "https://gitea.com/upstream-owner/upstream-repo/pulls/15", prURL)
	})

	// Test case: No PR exists
	t.Run("No PR exists", func(t *testing.T) {
		jsonOutput := `[{"index":16,"url":"https://gitea.com/upstream-owner/upstream-repo/pulls/16","head":{"name":"other-branch","repo":{"owner":{"login":"upstream-owner"}}}}]`
		mockRunner.On("Run", ctx, "tea",
			"pulls", "list", "--output", "json", "--fields", "index,head,url", "--state", "open", "--repo", "upstream-owner/upstream-repo",
		).Return([]byte(jsonOutput), nil).Once()

		exists, prURL, err := creator.checkGiteaPR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.False(t, exists)
		assert.Empty(t, prURL)
	})

	// Test case: Cross-fork PR exists (owner:branch format)
	t.Run("Cross-fork PR exists", func(t *testing.T) {
		jsonOutput := `[{"index":17,"url":"https://gitea.com/upstream-owner/upstream-repo/pulls/17","head":{"name":"yunpeng.wu:feature-branch","repo":{"owner":{"login":"yunpeng.wu"}}}},{"index":16,"url":"https://gitea.com/upstream-owner/upstream-repo/pulls/16","head":{"name":"other-branch","repo":{"owner":{"login":"upstream-owner"}}}}]`
		mockRunner.On("Run", ctx, "tea",
			"pulls", "list", "--output", "json", "--fields", "index,head,url", "--state", "open", "--repo", "upstream-owner/upstream-repo",
		).Return([]byte(jsonOutput), nil).Once()

		exists, prURL, err := creator.checkGiteaPR(ctx, branch, remoteInfo)
		assert.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, "https://gitea.com/upstream-owner/upstream-repo/pulls/17", prURL)
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

			// Mock the version check that PrepareContext now calls
			mockCLI.On("CheckMinVersion", "2.40.0", "2.0.0").Return(true, nil).Once()

			// Mock the base branch detection that PrepareContext now calls
			mockGit.On("GetParentBranch", ctx, tt.remote).Return("", fmt.Errorf("no parent")).Once()
			mockGit.On("GetDefaultBranch", ctx, tt.remote).Return("main", nil).Once()

			mockGit.On("GetCurrentBranch", ctx).Return("feature-branch", nil).Once()

			// The key assertion: verify it checks the correct repository with base branch
			mockRunner.On("Run", ctx, "gh",
				"pr", "list", "--head", "feature-branch", "--base", "main", "--json", "url,state,base",
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
