package cli

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

// TestDetector_CheckInstalled 测试CLI工具安装状态检测
func TestDetector_CheckInstalled(t *testing.T) {
	tests := []struct {
		name            string
		cliName         string
		checkCommand    string
		mockOutput      []byte
		mockError       error
		expectedInstalled bool
		expectedError   error
	}{
		{
			name:         "GitHub CLI installed",
			cliName:      "gh",
			checkCommand: "version",
			mockOutput:   []byte("gh version 2.40.1 (2024-01-15)"),
			mockError:    nil,
			expectedInstalled: true,
			expectedError: nil,
		},
		{
			name:         "tea CLI installed",
			cliName:      "tea",
			checkCommand: "version",
			mockOutput:   []byte("tea version 0.9.2"),
			mockError:    nil,
			expectedInstalled: true,
			expectedError: nil,
		},
		{
			name:         "CLI not installed",
			cliName:      "gh",
			checkCommand: "version",
			mockOutput:   nil,
			mockError:    fmt.Errorf("command not found: gh"),
			expectedInstalled: false,
			expectedError: nil,
		},
		{
			name:         "CLI exists but version check fails",
			cliName:      "gh",
			checkCommand: "version",
			mockOutput:   []byte("error: not authenticated"),
			mockError:    fmt.Errorf("exit status 1"),
			expectedInstalled: true, // CLI exists but may not be configured
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := new(MockCommandRunner)
			mockRunner.On("Run", mock.Anything, tt.cliName, []string{tt.checkCommand}).
				Return(tt.mockOutput, tt.mockError)

			detector := NewDetector(mockRunner)
			installed, err := detector.CheckInstalled(context.Background(), tt.cliName, tt.checkCommand)

			assert.Equal(t, tt.expectedInstalled, installed)
			assert.Equal(t, tt.expectedError, err)
			mockRunner.AssertExpectations(t)
		})
	}
}

// TestDetector_GetVersion 测试获取CLI工具版本
func TestDetector_GetVersion(t *testing.T) {
	tests := []struct {
		name            string
		cliName         string
		versionCommand  string
		versionArgs     []string
		mockOutput      []byte
		mockError       error
		expectedVersion string
		expectedError   bool
	}{
		{
			name:           "GitHub CLI version",
			cliName:        "gh",
			versionCommand: "version",
			versionArgs:    []string{},
			mockOutput:     []byte("gh version 2.40.1 (2024-01-15)\nhttps://github.com/cli/cli/releases/tag/v2.40.1\n"),
			mockError:      nil,
			expectedVersion: "2.40.1",
			expectedError:  false,
		},
		{
			name:           "tea version with v prefix",
			cliName:        "tea",
			versionCommand: "version",
			versionArgs:    []string{},
			mockOutput:     []byte("tea version v0.9.2\n"),
			mockError:      nil,
			expectedVersion: "0.9.2",
			expectedError:  false,
		},
		{
			name:           "Complex version output",
			cliName:        "custom-cli",
			versionCommand: "--version",
			versionArgs:    []string{},
			mockOutput:     []byte("Custom CLI Tool\nVersion: 1.2.3-beta.1+build123\nBuilt on: 2024-01-15\n"),
			mockError:      nil,
			expectedVersion: "1.2.3-beta.1+build123",
			expectedError:  false,
		},
		{
			name:           "Version command fails",
			cliName:        "gh",
			versionCommand: "version",
			versionArgs:    []string{},
			mockOutput:     nil,
			mockError:      fmt.Errorf("exit status 1"),
			expectedVersion: "",
			expectedError:  true,
		},
		{
			name:           "No version in output",
			cliName:        "bad-cli",
			versionCommand: "version",
			versionArgs:    []string{},
			mockOutput:     []byte("This CLI has no version info"),
			mockError:      nil,
			expectedVersion: "",
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := new(MockCommandRunner)
			args := append([]string{tt.versionCommand}, tt.versionArgs...)
			mockRunner.On("Run", mock.Anything, tt.cliName, args).
				Return(tt.mockOutput, tt.mockError)

			detector := NewDetector(mockRunner)
			version, err := detector.GetVersion(context.Background(), tt.cliName, tt.versionCommand, tt.versionArgs...)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVersion, version)
			}
			mockRunner.AssertExpectations(t)
		})
	}
}

// TestDetector_CheckAuthStatus 测试认证状态检测
func TestDetector_CheckAuthStatus(t *testing.T) {
	tests := []struct {
		name          string
		cliName       string
		authCommand   string
		authArgs      []string
		mockOutput    []byte
		mockError     error
		expectedAuth  bool
		expectedUser  string
		expectedError bool
	}{
		{
			name:        "GitHub CLI authenticated",
			cliName:     "gh",
			authCommand: "auth",
			authArgs:    []string{"status", "--hostname", "github.com"},
			mockOutput: []byte(`github.com
  ✓ Logged in to github.com as testuser (oauth_token)
  ✓ Git operations for github.com configured to use https protocol.
  ✓ Token: gho_************************************
  ✓ Token scopes: admin:public_key, gist, read:org, repo`),
			mockError:     nil,
			expectedAuth:  true,
			expectedUser:  "testuser",
			expectedError: false,
		},
		{
			name:        "GitHub CLI not authenticated",
			cliName:     "gh",
			authCommand: "auth",
			authArgs:    []string{"status"},
			mockOutput:  []byte("You are not logged into any GitHub hosts. Run gh auth login to authenticate."),
			mockError:   fmt.Errorf("exit status 1"),
			expectedAuth:  false,
			expectedUser:  "",
			expectedError: false,
		},
		{
			name:        "tea authenticated",
			cliName:     "tea",
			authCommand: "login",
			authArgs:    []string{"list"},
			mockOutput: []byte(`+---+------------------+-------------+--------+
| # | URL              | USER        | ACTIVE |
+---+------------------+-------------+--------+
| 1 | https://gitea.io | tea-user    | true   |
+---+------------------+-------------+--------+`),
			mockError:     nil,
			expectedAuth:  true,
			expectedUser:  "tea-user",
			expectedError: false,
		},
		{
			name:          "Command execution error",
			cliName:       "gh",
			authCommand:   "auth",
			authArgs:      []string{"status"},
			mockOutput:    nil,
			mockError:     fmt.Errorf("command not found"),
			expectedAuth:  false,
			expectedUser:  "",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := new(MockCommandRunner)
			args := append([]string{tt.authCommand}, tt.authArgs...)
			mockRunner.On("Run", mock.Anything, tt.cliName, args).
				Return(tt.mockOutput, tt.mockError)

			detector := NewDetector(mockRunner)
			authenticated, username, err := detector.CheckAuthStatus(context.Background(), tt.cliName, tt.authCommand, tt.authArgs...)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAuth, authenticated)
				assert.Equal(t, tt.expectedUser, username)
			}
			mockRunner.AssertExpectations(t)
		})
	}
}

// TestDetector_DetectCLI 测试综合CLI检测功能
func TestDetector_DetectCLI(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		setupMocks     func(*MockCommandRunner)
		expectedResult CLIStatus
		expectedError  bool
	}{
		{
			name:     "GitHub CLI fully configured",
			provider: "github",
			setupMocks: func(m *MockCommandRunner) {
				// Check installed
				m.On("Run", mock.Anything, "gh", []string{"version"}).
					Return([]byte("gh version 2.40.1 (2024-01-15)"), nil)
				// Get version
				m.On("Run", mock.Anything, "gh", []string{"version"}).
					Return([]byte("gh version 2.40.1 (2024-01-15)"), nil)
				// Check auth
				m.On("Run", mock.Anything, "gh", []string{"auth", "status"}).
					Return([]byte("✓ Logged in to github.com as testuser"), nil)
			},
			expectedResult: CLIStatus{
				Name:          "gh",
				Installed:     true,
				Version:       "2.40.1",
				Authenticated: true,
				Username:      "testuser",
			},
			expectedError: false,
		},
		{
			name:     "GitHub CLI not installed",
			provider: "github",
			setupMocks: func(m *MockCommandRunner) {
				// Check installed
				m.On("Run", mock.Anything, "gh", []string{"version"}).
					Return(nil, fmt.Errorf("command not found"))
			},
			expectedResult: CLIStatus{
				Name:      "gh",
				Installed: false,
			},
			expectedError: false,
		},
		{
			name:     "Gitea CLI installed but not authenticated",
			provider: "gitea",
			setupMocks: func(m *MockCommandRunner) {
				// Check installed
				m.On("Run", mock.Anything, "tea", []string{"version"}).
					Return([]byte("tea version 0.9.2"), nil)
				// Get version
				m.On("Run", mock.Anything, "tea", []string{"version"}).
					Return([]byte("tea version 0.9.2"), nil)
				// Check auth
				m.On("Run", mock.Anything, "tea", []string{"login", "list"}).
					Return([]byte("No logins found"), fmt.Errorf("exit status 1"))
			},
			expectedResult: CLIStatus{
				Name:          "tea",
				Installed:     true,
				Version:       "0.9.2",
				Authenticated: false,
			},
			expectedError: false,
		},
		{
			name:     "Unknown provider",
			provider: "unknown",
			setupMocks: func(m *MockCommandRunner) {
				// No mocks needed
			},
			expectedResult: CLIStatus{},
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := new(MockCommandRunner)
			tt.setupMocks(mockRunner)

			detector := NewDetector(mockRunner)
			status, err := detector.DetectCLI(context.Background(), tt.provider)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult.Name, status.Name)
				assert.Equal(t, tt.expectedResult.Installed, status.Installed)
				assert.Equal(t, tt.expectedResult.Version, status.Version)
				assert.Equal(t, tt.expectedResult.Authenticated, status.Authenticated)
				assert.Equal(t, tt.expectedResult.Username, status.Username)
			}
			mockRunner.AssertExpectations(t)
		})
	}
}

// TestDetector_SuggestInstallCommand 测试安装命令建议
func TestDetector_SuggestInstallCommand(t *testing.T) {
	tests := []struct {
		name             string
		cliName          string
		expectedCommands []string
	}{
		{
			name:    "GitHub CLI install commands",
			cliName: "gh",
			expectedCommands: []string{
				"brew install gh",
				"https://github.com/cli/cli#installation",
			},
		},
		{
			name:    "tea install commands",
			cliName: "tea",
			expectedCommands: []string{
				"go install gitea.com/gitea/tea@latest",
				"https://gitea.com/gitea/tea",
			},
		},
		{
			name:             "Unknown CLI",
			cliName:          "unknown-cli",
			expectedCommands: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewDetector(nil)
			commands := detector.SuggestInstallCommand(tt.cliName)
			assert.ElementsMatch(t, tt.expectedCommands, commands)
		})
	}
}