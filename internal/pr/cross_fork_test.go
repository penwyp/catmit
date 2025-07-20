package pr

import (
	"context"
	"testing"

	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// TestCrossForkPR tests cross-fork PR creation
func TestCrossForkPR(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	gitRunner := new(MockGitRunner)
	providerDetector := new(MockProviderDetector)
	cliDetector := new(MockCLIDetector)
	cmdBuilder := new(MockCommandBuilder)
	cmdRunner := new(MockCommandRunner)

	// Create PR creator
	creator := NewCreator(gitRunner, providerDetector, cliDetector, cmdBuilder, cmdRunner)

	// Test options
	options := CreateOptions{
		Remote:     "upstream",
		Title:      "feat: cross-fork PR",
		Body:       "This is a cross-fork PR",
		BaseBranch: "main",
	}

	// Setup expectations for cross-fork scenario
	// 1. Get upstream URL
	gitRunner.On("GetRemoteURL", ctx, "upstream").Return("git@git.example.com:alice/app-frontend.git", nil)
	
	// 2. Detect provider from upstream
	upstreamInfo := provider.RemoteInfo{
		Provider: "gitea",
		Host:     "git.example.com",
		Owner:    "alice",
		Repo:     "app-frontend",
		Protocol: "ssh",
	}
	providerDetector.On("DetectFromRemote", ctx, "git@git.example.com:alice/app-frontend.git").Return(upstreamInfo, nil)

	// 3. Get origin URL for source info
	gitRunner.On("GetRemoteURL", ctx, "origin").Return("git@git.example.com:john.doe/app-frontend.git", nil)
	
	// 4. Detect provider from origin
	originInfo := provider.RemoteInfo{
		Provider: "gitea",
		Host:     "git.example.com",
		Owner:    "john.doe",
		Repo:     "app-frontend",
		Protocol: "ssh",
	}
	providerDetector.On("DetectFromRemote", ctx, "git@git.example.com:john.doe/app-frontend.git").Return(originInfo, nil)

	// 5. CLI detection
	cliStatus := cli.CLIStatus{
		Name:          "tea",
		Installed:     true,
		Version:       "0.9.2",
		Authenticated: true,
	}
	cliDetector.On("DetectCLI", ctx, "gitea").Return(cliStatus, nil)
	cliDetector.On("CheckMinVersion", "0.9.2", "0.8.0").Return(true, nil)

	// 6. Get current branch
	gitRunner.On("GetCurrentBranch", ctx).Return("feat-op", nil)

	// 7. Build command with correct parameters
	expectedPROptions := PROptions{
		Title:       "feat: cross-fork PR",
		Body:        "This is a cross-fork PR",
		BaseBranch:  "main",
		HeadBranch:  "john.doe:feat-op", // Expected format for cross-fork
		TargetOwner: "alice",
		TargetRepo:  "app-frontend",
		SourceOwner: "john.doe",
	}
	
	expectedArgs := []string{
		"pr", "create",
		"--repo", "alice/app-frontend",
		"--title", "feat: cross-fork PR",
		"--description", "This is a cross-fork PR",
		"--base", "main",
		"--head", "john.doe:feat-op",
	}
	
	cmdBuilder.On("BuildCommand", "gitea", expectedPROptions).Return("tea", expectedArgs, nil)

	// 8. Execute command
	output := "Created PR #123: https://git.example.com/alice/app-frontend/pulls/123"
	cmdRunner.On("Run", ctx, "tea", expectedArgs).Return([]byte(output), nil)

	// 9. Parse output
	cmdBuilder.On("ParseGiteaPROutput", output).Return("https://git.example.com/alice/app-frontend/pulls/123", nil)

	// Execute
	prURL, err := creator.Create(ctx, options)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, "https://git.example.com/alice/app-frontend/pulls/123", prURL)

	// Verify all expectations were met
	gitRunner.AssertExpectations(t)
	providerDetector.AssertExpectations(t)
	cliDetector.AssertExpectations(t)
	cmdBuilder.AssertExpectations(t)
	cmdRunner.AssertExpectations(t)
}

// TestPRAlreadyExistsWithoutURL tests the friendly error handling when PR exists but URL cannot be parsed
func TestPRAlreadyExistsWithoutURL(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	gitRunner := new(MockGitRunner)
	providerDetector := new(MockProviderDetector)
	cliDetector := new(MockCLIDetector)
	cmdBuilder := new(MockCommandBuilder)
	cmdRunner := new(MockCommandRunner)

	// Create PR creator
	creator := NewCreator(gitRunner, providerDetector, cliDetector, cmdBuilder, cmdRunner)

	// Test options
	options := CreateOptions{
		Remote:     "origin",
		BaseBranch: "main",
	}

	// Setup basic expectations
	gitRunner.On("GetRemoteURL", ctx, "origin").Return("git@git.example.com:john.doe/app-frontend.git", nil)
	
	remoteInfo := provider.RemoteInfo{
		Provider: "gitea",
		Host:     "git.example.com",
		Owner:    "john.doe",
		Repo:     "app-frontend",
		Protocol: "ssh",
	}
	providerDetector.On("DetectFromRemote", ctx, mock.Anything).Return(remoteInfo, nil)

	cliStatus := cli.CLIStatus{
		Name:          "tea",
		Installed:     true,
		Version:       "0.9.2",
		Authenticated: true,
	}
	cliDetector.On("DetectCLI", ctx, "gitea").Return(cliStatus, nil)
	cliDetector.On("CheckMinVersion", "0.9.2", "0.8.0").Return(true, nil)

	gitRunner.On("GetCurrentBranch", ctx).Return("feat-op", nil)

	// Build command
	cmdBuilder.On("BuildCommand", "gitea", mock.Anything).Return("tea", []string{"pr", "create"}, nil)

	// Execute command with "already exists" error but no parseable URL
	errorOutput := "Error: could not create PR from feat-op to john.doe:master: pull request already exists for these targets [id: 842, issue_id: 2, head_repo_id: 193, base_repo_id: 193, head_branch: feat-op, base_branch: master]"
	cmdRunner.On("Run", ctx, "tea", mock.Anything).Return([]byte(errorOutput), assert.AnError)

	// Parse output fails
	cmdBuilder.On("ParseGiteaPROutput", errorOutput).Return("", assert.AnError)
	
	// ParseGiteaErrorForPRInfo also fails
	cmdBuilder.On("ParseGiteaErrorForPRInfo", errorOutput, "git.example.com", "john.doe", "app-frontend").Return("", assert.AnError)

	// Execute
	_, err := creator.Create(ctx, options)

	// Verify we get ErrPRAlreadyExists
	assert.Error(t, err)
	var prExists *ErrPRAlreadyExists
	assert.ErrorAs(t, err, &prExists)
	assert.Empty(t, prExists.URL) // URL should be empty since we couldn't parse it

	// Verify all expectations were met
	gitRunner.AssertExpectations(t)
	providerDetector.AssertExpectations(t)
	cliDetector.AssertExpectations(t)
	cmdBuilder.AssertExpectations(t)
	cmdRunner.AssertExpectations(t)
}

// TestPRAlreadyExistsWithExtractedURL tests successful extraction of PR URL from error
func TestPRAlreadyExistsWithExtractedURL(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	gitRunner := new(MockGitRunner)
	providerDetector := new(MockProviderDetector)
	cliDetector := new(MockCLIDetector)
	cmdBuilder := new(MockCommandBuilder)
	cmdRunner := new(MockCommandRunner)

	// Create PR creator
	creator := NewCreator(gitRunner, providerDetector, cliDetector, cmdBuilder, cmdRunner)

	// Test options
	options := CreateOptions{
		Remote:     "origin",
		BaseBranch: "main",
	}

	// Setup basic expectations
	gitRunner.On("GetRemoteURL", ctx, "origin").Return("git@git.example.com:john.doe/app-frontend.git", nil)
	
	remoteInfo := provider.RemoteInfo{
		Provider: "gitea",
		Host:     "git.example.com",
		Owner:    "john.doe",
		Repo:     "app-frontend",
		Protocol: "ssh",
	}
	providerDetector.On("DetectFromRemote", ctx, mock.Anything).Return(remoteInfo, nil)

	cliStatus := cli.CLIStatus{
		Name:          "tea",
		Installed:     true,
		Version:       "0.9.2",
		Authenticated: true,
	}
	cliDetector.On("DetectCLI", ctx, "gitea").Return(cliStatus, nil)
	cliDetector.On("CheckMinVersion", "0.9.2", "0.8.0").Return(true, nil)

	gitRunner.On("GetCurrentBranch", ctx).Return("feat-op", nil)

	// Build command
	cmdBuilder.On("BuildCommand", "gitea", mock.Anything).Return("tea", []string{"pr", "create"}, nil)

	// Execute command with "already exists" error
	errorOutput := "Error: could not create PR from feat-op to john.doe:master: pull request already exists for these targets [id: 842, issue_id: 80, head_repo_id: 193, base_repo_id: 193, head_branch: feat-op, base_branch: master]"
	cmdRunner.On("Run", ctx, "tea", mock.Anything).Return([]byte(errorOutput), assert.AnError)

	// Parse output fails
	cmdBuilder.On("ParseGiteaPROutput", errorOutput).Return("", assert.AnError)
	
	// ParseGiteaErrorForPRInfo succeeds
	expectedURL := "https://git.example.com/john.doe/app-frontend/pulls/80"
	cmdBuilder.On("ParseGiteaErrorForPRInfo", errorOutput, "git.example.com", "john.doe", "app-frontend").Return(expectedURL, nil)

	// Execute
	_, err := creator.Create(ctx, options)

	// Verify we get ErrPRAlreadyExists with URL
	assert.Error(t, err)
	var prExists *ErrPRAlreadyExists
	assert.ErrorAs(t, err, &prExists)
	assert.Equal(t, expectedURL, prExists.URL)

	// Verify all expectations were met
	gitRunner.AssertExpectations(t)
	providerDetector.AssertExpectations(t)
	cliDetector.AssertExpectations(t)
	cmdBuilder.AssertExpectations(t)
	cmdRunner.AssertExpectations(t)
}