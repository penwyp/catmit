package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/penwyp/catmit/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockConfigManager is a mock implementation of config.Manager
type MockConfigManager struct {
	mock.Mock
}

func (m *MockConfigManager) Load() (*config.Config, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*config.Config), args.Error(1)
}

func (m *MockConfigManager) Save(cfg *config.Config) error {
	args := m.Called(cfg)
	return args.Error(0)
}

func (m *MockConfigManager) CreateDefaultConfig() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockConfigManager) UpdateRemote(host string, cfg config.RemoteConfig) error {
	args := m.Called(host, cfg)
	return args.Error(0)
}

// MockHTTPProber is a mock implementation of HTTPProber
type MockHTTPProber struct {
	mock.Mock
}

func (m *MockHTTPProber) ProbeGitea(ctx context.Context, url string) ProbeResult {
	args := m.Called(ctx, url)
	return args.Get(0).(ProbeResult)
}

func TestConfigDetector_DetectFromRemote(t *testing.T) {
	tests := []struct {
		name           string
		remoteURL      string
		configSetup    func(*MockConfigManager)
		probeSetup     func(*MockHTTPProber)
		expectedInfo   RemoteInfo
		expectedErr    bool
	}{
		{
			name:      "Config mapping takes priority over pattern matching",
			remoteURL: "https://git.mycompany.com/team/project.git",
			configSetup: func(m *MockConfigManager) {
				cfg := &config.Config{
					Version: "1.0",
					Remotes: map[string]config.RemoteConfig{
						"git.mycompany.com": {
							Provider: "gitlab",
							CLITool:  "glab",
						},
					},
				}
				m.On("Load").Return(cfg, nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called since config mapping exists
			},
			expectedInfo: RemoteInfo{
				Provider: "gitlab",
				Host:     "git.mycompany.com",
				Owner:    "team",
				Repo:     "project",
				Protocol: "https",
			},
		},
		{
			name:      "Pattern matching for GitHub",
			remoteURL: "git@github.com:owner/repo.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(&config.Config{}, nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called for known provider
			},
			expectedInfo: RemoteInfo{
				Provider: "github",
				Host:     "github.com",
				Owner:    "owner",
				Repo:     "repo",
				Protocol: "ssh",
			},
		},
		{
			name:      "Pattern matching for GitLab",
			remoteURL: "https://gitlab.com/group/project.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(&config.Config{}, nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called for known provider
			},
			expectedInfo: RemoteInfo{
				Provider: "gitlab",
				Host:     "gitlab.com",
				Owner:    "group",
				Repo:     "project",
				Protocol: "https",
			},
		},
		{
			name:      "Pattern matching for self-hosted GitLab",
			remoteURL: "https://gitlab.internal.com/team/app.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(&config.Config{}, nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called when pattern matches
			},
			expectedInfo: RemoteInfo{
				Provider: "gitlab",
				Host:     "gitlab.internal.com",
				Owner:    "team",
				Repo:     "app",
				Protocol: "https",
			},
		},
		{
			name:      "Pattern matching for Bitbucket",
			remoteURL: "https://bitbucket.org/workspace/repo.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(&config.Config{}, nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called for known provider
			},
			expectedInfo: RemoteInfo{
				Provider: "bitbucket",
				Host:     "bitbucket.org",
				Owner:    "workspace",
				Repo:     "repo",
				Protocol: "https",
			},
		},
		{
			name:      "Pattern matching for Gitea subdomain",
			remoteURL: "https://gitea.company.com/org/project.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(&config.Config{}, nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called when pattern matches
			},
			expectedInfo: RemoteInfo{
				Provider: "gitea",
				Host:     "gitea.company.com",
				Owner:    "org",
				Repo:     "project",
				Protocol: "https",
			},
		},
		{
			name:      "Pattern matching for Gogs",
			remoteURL: "https://gogs.example.com/user/repo.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(&config.Config{}, nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called when pattern matches
			},
			expectedInfo: RemoteInfo{
				Provider: "gogs",
				Host:     "gogs.example.com",
				Owner:    "user",
				Repo:     "repo",
				Protocol: "https",
			},
		},
		{
			name:      "HTTP probe detects Gitea for unknown host",
			remoteURL: "https://git.unknown.com/org/project.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(&config.Config{}, nil)
				m.On("UpdateRemote", "git.unknown.com", config.RemoteConfig{
					Provider: "gitea",
					CLITool:  "tea",
				}).Return(nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				m.On("ProbeGitea", mock.Anything, "https://git.unknown.com").Return(ProbeResult{
					IsGitea: true,
					Version: "1.19.0",
				})
			},
			expectedInfo: RemoteInfo{
				Provider: "gitea",
				Host:     "git.unknown.com",
				Owner:    "org",
				Repo:     "project",
				Protocol: "https",
			},
		},
		{
			name:      "Unknown provider when all detection fails",
			remoteURL: "https://git.internal.com/team/app.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(&config.Config{}, nil)
			},
			probeSetup: func(m *MockHTTPProber) {
				m.On("ProbeGitea", mock.Anything, "https://git.internal.com").Return(ProbeResult{
					IsGitea: false,
				})
			},
			expectedInfo: RemoteInfo{
				Provider: "unknown",
				Host:     "git.internal.com",
				Owner:    "team",
				Repo:     "app",
				Protocol: "https",
			},
		},
		{
			name:      "Config load error is ignored",
			remoteURL: "https://github.com/user/repo.git",
			configSetup: func(m *MockConfigManager) {
				m.On("Load").Return(nil, errors.New("config file not found"))
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called for known provider
			},
			expectedInfo: RemoteInfo{
				Provider: "github",
				Host:     "github.com",
				Owner:    "user",
				Repo:     "repo",
				Protocol: "https",
			},
		},
		{
			name:      "Invalid URL returns error",
			remoteURL: "not-a-valid-url",
			configSetup: func(m *MockConfigManager) {
				// Should not be called
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called
			},
			expectedErr: true,
		},
		{
			name:      "Nil config manager doesn't crash",
			remoteURL: "https://github.com/user/repo.git",
			configSetup: func(m *MockConfigManager) {
				// Not used - detector created with nil
			},
			probeSetup: func(m *MockHTTPProber) {
				// Should not be called for known provider
			},
			expectedInfo: RemoteInfo{
				Provider: "github",
				Host:     "github.com",
				Owner:    "user",
				Repo:     "repo",
				Protocol: "https",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := new(MockConfigManager)
			mockProber := new(MockHTTPProber)

			if tt.configSetup != nil {
				tt.configSetup(mockConfig)
			}
			if tt.probeSetup != nil {
				tt.probeSetup(mockProber)
			}

			// Test with nil config manager for specific test case
			var detector *ConfigDetector
			if tt.name == "Nil config manager doesn't crash" {
				detector = NewConfigDetector(nil)
				detector.httpProber = mockProber
			} else {
				detector = NewConfigDetector(mockConfig)
				detector.httpProber = mockProber
			}

			ctx := context.Background()
			info, err := detector.DetectFromRemote(ctx, tt.remoteURL)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedInfo, info)
			}

			mockConfig.AssertExpectations(t)
			mockProber.AssertExpectations(t)
		})
	}
}

func TestConfigDetector_saveDiscoveredProvider(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		provider string
		setupMock func(*MockConfigManager)
	}{
		{
			name:     "Save discovered Gitea provider",
			host:     "git.example.com",
			provider: "gitea",
			setupMock: func(m *MockConfigManager) {
				m.On("UpdateRemote", "git.example.com", config.RemoteConfig{
					Provider: "gitea",
					CLITool:  "tea",
				}).Return(nil)
			},
		},
		{
			name:     "Save discovered GitLab provider",
			host:     "gitlab.company.com",
			provider: "gitlab",
			setupMock: func(m *MockConfigManager) {
				m.On("UpdateRemote", "gitlab.company.com", config.RemoteConfig{
					Provider: "gitlab",
					CLITool:  "glab",
				}).Return(nil)
			},
		},
		{
			name:     "Save error is ignored",
			host:     "git.example.com",
			provider: "github",
			setupMock: func(m *MockConfigManager) {
				m.On("UpdateRemote", "git.example.com", config.RemoteConfig{
					Provider: "github",
					CLITool:  "gh",
				}).Return(errors.New("permission denied"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := new(MockConfigManager)
			tt.setupMock(mockConfig)

			detector := NewConfigDetector(mockConfig)
			detector.saveDiscoveredProvider(tt.host, tt.provider)

			mockConfig.AssertExpectations(t)
		})
	}
}

func TestGetDefaultCLITool(t *testing.T) {
	// Test all cases to ensure 100% coverage
	assert.Equal(t, "gh", getDefaultCLITool("github"))
	assert.Equal(t, "glab", getDefaultCLITool("gitlab"))
	assert.Equal(t, "tea", getDefaultCLITool("gitea"))
	assert.Equal(t, "bb", getDefaultCLITool("bitbucket"))
	assert.Equal(t, "", getDefaultCLITool("gogs"))
	assert.Equal(t, "", getDefaultCLITool("unknown"))
	assert.Equal(t, "", getDefaultCLITool("custom"))
	assert.Equal(t, "", getDefaultCLITool(""))
}

func TestConfigDetector_NilHTTPProber(t *testing.T) {
	mockConfig := new(MockConfigManager)
	mockConfig.On("Load").Return(&config.Config{}, nil)

	detector := NewConfigDetector(mockConfig)
	detector.httpProber = nil // Explicitly set to nil

	ctx := context.Background()
	info, err := detector.DetectFromRemote(ctx, "https://unknown.example.com/user/repo.git")

	assert.NoError(t, err)
	assert.Equal(t, "unknown", info.Provider)
	mockConfig.AssertExpectations(t)
}