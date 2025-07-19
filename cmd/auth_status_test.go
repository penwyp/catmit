package cmd

import (
	"bytes"
	"context"
	"strings"
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

func (m *MockCLIDetector) SuggestInstallCommand(cliName string) []string {
	args := m.Called(cliName)
	if args.Get(0) == nil {
		return []string{}
	}
	return args.Get(0).([]string)
}

// MockGitRunner 模拟Git命令执行器
type MockGitRunner struct {
	mock.Mock
}

func (m *MockGitRunner) GetRemotes(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockGitRunner) GetRemoteURL(ctx context.Context, remote string) (string, error) {
	args := m.Called(ctx, remote)
	return args.String(0), args.Error(1)
}

// TestAuthStatusCommand_Execute 测试auth status命令执行
func TestAuthStatusCommand_Execute(t *testing.T) {
	tests := []struct {
		name            string
		setupMocks      func(*MockGitRunner, *MockProviderDetector, *MockCLIDetector)
		expectedOutput  []string
		expectedError   bool
	}{
		{
			name: "Single remote with GitHub authenticated",
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector) {
				git.On("GetRemotes", mock.Anything).Return([]string{"origin"}, nil)
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://github.com/owner/repo.git", nil)
				remoteInfo := provider.RemoteInfo{Provider: "github", Host: "github.com"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://github.com/owner/repo.git").
					Return(remoteInfo, nil)
				cliStatus := cli.CLIStatus{
					Name:          "gh",
					Installed:     true,
					Version:       "2.40.1",
					Authenticated: true,
					Username:      "testuser",
				}
				cliDetector.On("DetectCLI", mock.Anything, "github").
					Return(cliStatus, nil)
			},
			expectedOutput: []string{
				"Remote", "Provider", "CLI", "Status", "Version", "User",
				"origin", "github", "gh", "✓ Authenticated", "2.40.1", "testuser",
			},
			expectedError: false,
		},
		{
			name: "Multiple remotes with mixed status",
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector) {
				git.On("GetRemotes", mock.Anything).Return([]string{"origin", "upstream"}, nil)
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://github.com/owner/repo.git", nil)
				git.On("GetRemoteURL", mock.Anything, "upstream").Return("https://gitea.io/org/project.git", nil)
				
				remoteInfo1 := provider.RemoteInfo{Provider: "github", Host: "github.com"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://github.com/owner/repo.git").
					Return(remoteInfo1, nil)
				remoteInfo2 := provider.RemoteInfo{Provider: "gitea", Host: "gitea.io"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://gitea.io/org/project.git").
					Return(remoteInfo2, nil)
				
				cliStatus1 := cli.CLIStatus{
					Name:          "gh",
					Installed:     true,
					Version:       "2.40.1",
					Authenticated: true,
					Username:      "testuser",
				}
				cliDetector.On("DetectCLI", mock.Anything, "github").
					Return(cliStatus1, nil)
				cliStatus2 := cli.CLIStatus{
					Name:          "tea",
					Installed:     true,
					Version:       "0.9.2",
					Authenticated: false,
				}
				cliDetector.On("DetectCLI", mock.Anything, "gitea").
					Return(cliStatus2, nil)
			},
			expectedOutput: []string{
				"origin", "github", "gh", "✓ Authenticated", "2.40.1", "testuser",
				"upstream", "gitea", "tea", "✗ Not authenticated", "0.9.2", "-",
			},
			expectedError: false,
		},
		{
			name: "CLI not installed",
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector) {
				git.On("GetRemotes", mock.Anything).Return([]string{"origin"}, nil)
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://github.com/owner/repo.git", nil)
				remoteInfo := provider.RemoteInfo{Provider: "github", Host: "github.com"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://github.com/owner/repo.git").
					Return(remoteInfo, nil)
				cliStatus := cli.CLIStatus{
					Name:      "gh",
					Installed: false,
				}
				cliDetector.On("DetectCLI", mock.Anything, "github").
					Return(cliStatus, nil)
				cliDetector.On("SuggestInstallCommand", "gh").
					Return([]string{"brew install gh", "https://github.com/cli/cli#installation"})
			},
			expectedOutput: []string{
				"origin", "github", "gh", "✗ Not installed", "-", "-",
				"Install with:",
				"brew install gh",
			},
			expectedError: false,
		},
		{
			name: "Unknown provider",
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector) {
				git.On("GetRemotes", mock.Anything).Return([]string{"origin"}, nil)
				git.On("GetRemoteURL", mock.Anything, "origin").Return("https://unknown.com/owner/repo.git", nil)
				remoteInfo := provider.RemoteInfo{Provider: "unknown", Host: "unknown.com"}
				providerDetector.On("DetectFromRemote", mock.Anything, "https://unknown.com/owner/repo.git").
					Return(remoteInfo, nil)
			},
			expectedOutput: []string{
				"origin", "unknown", "-", "Provider not supported", "-", "-",
			},
			expectedError: false,
		},
		{
			name: "No remotes",
			setupMocks: func(git *MockGitRunner, providerDetector *MockProviderDetector, cliDetector *MockCLIDetector) {
				git.On("GetRemotes", mock.Anything).Return([]string{}, nil)
			},
			expectedOutput: []string{
				"No git remotes found",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置mock
			mockGit := new(MockGitRunner)
			mockProvider := new(MockProviderDetector)
			mockCLI := new(MockCLIDetector)
			tt.setupMocks(mockGit, mockProvider, mockCLI)

			// 创建命令并捕获输出
			var buf bytes.Buffer
			cmd := NewAuthStatusCommand(mockGit, mockProvider, mockCLI)
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// 执行命令
			err := cmd.Execute()

			// 验证结果
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				output := buf.String()
				for _, expected := range tt.expectedOutput {
					assert.Contains(t, output, expected)
				}
			}

			// 验证mock调用
			mockGit.AssertExpectations(t)
			mockProvider.AssertExpectations(t)
			mockCLI.AssertExpectations(t)
		})
	}
}

// TestAuthStatusCommand_FormatTable 测试表格格式化
func TestAuthStatusCommand_FormatTable(t *testing.T) {
	tests := []struct {
		name           string
		statuses       []RemoteAuthStatus
		expectedLines  []string
	}{
		{
			name: "Standard table format",
			statuses: []RemoteAuthStatus{
				{
					Remote:   "origin",
					Provider: "github",
					CLI:      "gh",
					Status:   "✓ Authenticated",
					Version:  "2.40.1",
					Username: "testuser",
				},
			},
			expectedLines: []string{
				"Remote", "Provider", "CLI", "Status", "Version", "User",
				"------", "--------", "---", "------", "-------", "----",
				"origin", "github", "gh", "✓ Authenticated", "2.40.1", "testuser",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := formatAuthStatusTable(tt.statuses)
			for _, line := range tt.expectedLines {
				assert.Contains(t, output, line)
			}
		})
	}
}

// TestAuthStatusCommand_ColorOutput 测试彩色输出
func TestAuthStatusCommand_ColorOutput(t *testing.T) {
	tests := []struct {
		name          string
		authenticated bool
		expectedColor string
	}{
		{
			name:          "Authenticated shows green",
			authenticated: true,
			expectedColor: "\x1b[32m", // ANSI green
		},
		{
			name:          "Not authenticated shows red",
			authenticated: false,
			expectedColor: "\x1b[31m", // ANSI red
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := RemoteAuthStatus{
				Remote:        "origin",
				Provider:      "github",
				CLI:           "gh",
				Authenticated: tt.authenticated,
			}
			
			output := formatAuthStatusWithColor(status)
			if strings.Contains(output, "✓") || strings.Contains(output, "✗") {
				assert.Contains(t, output, tt.expectedColor)
			}
		})
	}
}