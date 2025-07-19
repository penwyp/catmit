package pr

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockProviderDetector 模拟Provider检测器
type MockProviderDetector struct {
	mock.Mock
}

func (m *MockProviderDetector) DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error) {
	args := m.Called(ctx, remoteURL)
	return args.Get(0).(provider.RemoteInfo), args.Error(1)
}

// MockCLIDetector 模拟CLI检测器
type MockCLIDetector struct {
	mock.Mock
}

func (m *MockCLIDetector) DetectCLI(ctx context.Context, provider string) (cli.CLIStatus, error) {
	args := m.Called(ctx, provider)
	return args.Get(0).(cli.CLIStatus), args.Error(1)
}

func (m *MockCLIDetector) CheckMinVersion(current, minimum string) (bool, error) {
	args := m.Called(current, minimum)
	return args.Bool(0), args.Error(1)
}

// MockCommandBuilder 模拟命令构建器
type MockCommandBuilder struct {
	mock.Mock
}

func (m *MockCommandBuilder) BuildCommand(provider string, options PROptions) (string, []string, error) {
	args := m.Called(provider, options)
	return args.String(0), args.Get(1).([]string), args.Error(2)
}

func (m *MockCommandBuilder) ParseGitHubPROutput(output string) (string, error) {
	args := m.Called(output)
	return args.String(0), args.Error(1)
}

func (m *MockCommandBuilder) ParseGiteaPROutput(output string) (string, error) {
	args := m.Called(output)
	return args.String(0), args.Error(1)
}

func (m *MockCommandBuilder) ParseGitLabMROutput(output string) (string, error) {
	args := m.Called(output)
	return args.String(0), args.Error(1)
}

// MockGitRunner 模拟Git执行器
type MockGitRunner struct {
	mock.Mock
}

func (m *MockGitRunner) GetRemoteURL(ctx context.Context, remote string) (string, error) {
	args := m.Called(ctx, remote)
	return args.String(0), args.Error(1)
}

func (m *MockGitRunner) GetCurrentBranch(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockGitRunner) GetCommitMessage(ctx context.Context, ref string) (string, error) {
	args := m.Called(ctx, ref)
	return args.String(0), args.Error(1)
}

func (m *MockGitRunner) GetDefaultBranch(ctx context.Context, remote string) (string, error) {
	args := m.Called(ctx, remote)
	return args.String(0), args.Error(1)
}

// MockCommandRunner 模拟命令执行器
type MockCommandRunner struct {
	mock.Mock
}

func (m *MockCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	argList := m.Called(ctx, name, args)
	if argList.Get(0) == nil {
		return nil, argList.Error(1)
	}
	return argList.Get(0).([]byte), argList.Error(1)
}

// TestPRCreator_Create 测试PR创建主流程
func TestPRCreator_Create(t *testing.T) {
	tests := []struct {
		name          string
		options       CreateOptions
		setupMocks    func(*MockGitRunner, *MockProviderDetector, *MockCLIDetector, *MockCommandBuilder, *MockCommandRunner)
		expectedURL   string
		expectedError string
	}{
		{
			name: "GitHub PR creation success",
			options: CreateOptions{
				Remote:     "origin",
				BaseBranch: "main",
				Fill:       true,
			},
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector, cmdBuilder *MockCommandBuilder, cmdRunner *MockCommandRunner) {
				// Git operations
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://github.com/owner/repo.git", nil)
				git.On("GetCurrentBranch", mock.Anything).Return("feature-branch", nil)
				
				// Provider detection
				remoteInfo := provider.RemoteInfo{Provider: "github", Host: "github.com", Owner: "owner", Repo: "repo"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://github.com/owner/repo.git").Return(remoteInfo, nil)
				
				// CLI detection
				cliStatus := cli.CLIStatus{Name: "gh", Installed: true, Version: "2.40.1", Authenticated: true}
				cliDetector.On("DetectCLI", mock.Anything, "github").Return(cliStatus, nil)
				cliDetector.On("CheckMinVersion", "2.40.1", "2.0.0").Return(true, nil)
				
				// Command building
				prOptions := PROptions{Fill: true, BaseBranch: "main"}
				cmdBuilder.On("BuildCommand", "github", prOptions).Return("gh", []string{"pr", "create", "--fill", "--base", "main"}, nil)
				
				// Command execution
				output := "Creating pull request for feature-branch into main in owner/repo\n\nhttps://github.com/owner/repo/pull/123\n"
				cmdRunner.On("Run", mock.Anything, "gh", []string{"pr", "create", "--fill", "--base", "main"}).Return([]byte(output), nil)
				
				// Output parsing
				cmdBuilder.On("ParseGitHubPROutput", output).Return("https://github.com/owner/repo/pull/123", nil)
			},
			expectedURL:   "https://github.com/owner/repo/pull/123",
			expectedError: "",
		},
		{
			name: "Gitea PR creation with explicit options",
			options: CreateOptions{
				Remote:     "origin",
				Title:      "feat: new feature",
				Body:       "This adds a new feature",
				BaseBranch: "main",
			},
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector, cmdBuilder *MockCommandBuilder, cmdRunner *MockCommandRunner) {
				// Git operations
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://gitea.io/owner/repo.git", nil)
				git.On("GetCurrentBranch", mock.Anything).Return("feature-branch", nil)
				
				// Provider detection
				remoteInfo := provider.RemoteInfo{Provider: "gitea", Host: "gitea.io", Owner: "owner", Repo: "repo"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://gitea.io/owner/repo.git").Return(remoteInfo, nil)
				
				// CLI detection
				cliStatus := cli.CLIStatus{Name: "tea", Installed: true, Version: "0.9.2", Authenticated: true}
				cliDetector.On("DetectCLI", mock.Anything, "gitea").Return(cliStatus, nil)
				cliDetector.On("CheckMinVersion", "0.9.2", "0.8.0").Return(true, nil)
				
				// Command building
				prOptions := PROptions{
					Title:      "feat: new feature",
					Body:       "This adds a new feature",
					BaseBranch: "main",
					HeadBranch: "feature-branch",
				}
				cmdBuilder.On("BuildCommand", "gitea", prOptions).Return("tea", []string{"pr", "create", "--title", "feat: new feature", "--description", "This adds a new feature", "--base", "main", "--head", "feature-branch"}, nil)
				
				// Command execution
				output := "Created PR #42: https://gitea.io/owner/repo/pulls/42\n"
				cmdRunner.On("Run", mock.Anything, "tea", mock.Anything).Return([]byte(output), nil)
				
				// Output parsing
				cmdBuilder.On("ParseGiteaPROutput", output).Return("https://gitea.io/owner/repo/pulls/42", nil)
			},
			expectedURL:   "https://gitea.io/owner/repo/pulls/42",
			expectedError: "",
		},
		{
			name: "CLI not installed error",
			options: CreateOptions{
				Remote:     "origin",
				BaseBranch: "main",
			},
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector, cmdBuilder *MockCommandBuilder, cmdRunner *MockCommandRunner) {
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://github.com/owner/repo.git", nil)
				
				remoteInfo := provider.RemoteInfo{Provider: "github", Host: "github.com"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://github.com/owner/repo.git").Return(remoteInfo, nil)
				
				cliStatus := cli.CLIStatus{Name: "gh", Installed: false}
				cliDetector.On("DetectCLI", mock.Anything, "github").Return(cliStatus, nil)
			},
			expectedURL:   "",
			expectedError: "gh is not installed",
		},
		{
			name: "CLI not authenticated error",
			options: CreateOptions{
				Remote:     "origin",
				BaseBranch: "main",
			},
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector, cmdBuilder *MockCommandBuilder, cmdRunner *MockCommandRunner) {
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://github.com/owner/repo.git", nil)
				
				remoteInfo := provider.RemoteInfo{Provider: "github", Host: "github.com"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://github.com/owner/repo.git").Return(remoteInfo, nil)
				
				cliStatus := cli.CLIStatus{Name: "gh", Installed: true, Version: "2.40.1", Authenticated: false}
				cliDetector.On("DetectCLI", mock.Anything, "github").Return(cliStatus, nil)
			},
			expectedURL:   "",
			expectedError: "gh is not authenticated",
		},
		{
			name: "PR already exists",
			options: CreateOptions{
				Remote:     "origin",
				BaseBranch: "main",
				Fill:       true,
			},
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector, cmdBuilder *MockCommandBuilder, cmdRunner *MockCommandRunner) {
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://github.com/owner/repo.git", nil)
				git.On("GetCurrentBranch", mock.Anything).Return("feature-branch", nil)
				
				remoteInfo := provider.RemoteInfo{Provider: "github", Host: "github.com"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://github.com/owner/repo.git").Return(remoteInfo, nil)
				
				cliStatus := cli.CLIStatus{Name: "gh", Installed: true, Version: "2.40.1", Authenticated: true}
				cliDetector.On("DetectCLI", mock.Anything, "github").Return(cliStatus, nil)
				cliDetector.On("CheckMinVersion", "2.40.1", "2.0.0").Return(true, nil)
				
				prOptions := PROptions{Fill: true, BaseBranch: "main"}
				cmdBuilder.On("BuildCommand", "github", prOptions).Return("gh", []string{"pr", "create", "--fill", "--base", "main"}, nil)
				
				// PR already exists error
				output := "a pull request for branch \"feature-branch\" into branch \"main\" already exists:\nhttps://github.com/owner/repo/pull/456\n"
				cmdRunner.On("Run", mock.Anything, "gh", mock.Anything).Return([]byte(output), fmt.Errorf("exit status 1"))
				
				cmdBuilder.On("ParseGitHubPROutput", output).Return("https://github.com/owner/repo/pull/456", nil)
			},
			expectedURL:   "",
			expectedError: "pull request already exists",
		},
		{
			name: "Unknown provider error",
			options: CreateOptions{
				Remote:     "origin",
				BaseBranch: "main",
			},
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector, cmdBuilder *MockCommandBuilder, cmdRunner *MockCommandRunner) {
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://unknown.com/owner/repo.git", nil)
				
				remoteInfo := provider.RemoteInfo{Provider: "unknown", Host: "unknown.com"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://unknown.com/owner/repo.git").Return(remoteInfo, nil)
			},
			expectedURL:   "",
			expectedError: "unsupported provider: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockGit := new(MockGitRunner)
			mockProvider := new(MockProviderDetector)
			mockCLI := new(MockCLIDetector)
			mockCmdBuilder := new(MockCommandBuilder)
			mockCmdRunner := new(MockCommandRunner)
			
			tt.setupMocks(mockGit, mockProvider, mockCLI, mockCmdBuilder, mockCmdRunner)
			
			// Create PR creator
			creator := NewCreator(
				mockGit,
				mockProvider,
				mockCLI,
				mockCmdBuilder,
				mockCmdRunner,
			)
			
			// Execute
			url, err := creator.Create(context.Background(), tt.options)
			
			// Assert
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				
				// Special check for PR already exists error
				if tt.name == "PR already exists" {
					var prExists *ErrPRAlreadyExists
					assert.True(t, errors.As(err, &prExists))
					assert.Equal(t, "https://github.com/owner/repo/pull/456", prExists.URL)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURL, url)
			}
			
			// Verify mocks
			mockGit.AssertExpectations(t)
			mockProvider.AssertExpectations(t)
			mockCLI.AssertExpectations(t)
			mockCmdBuilder.AssertExpectations(t)
			mockCmdRunner.AssertExpectations(t)
		})
	}
}