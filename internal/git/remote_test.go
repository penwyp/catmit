package git

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockRunner 用于模拟git命令执行
type MockRunner struct {
	mock.Mock
}

func (m *MockRunner) Run(ctx context.Context, command string, args ...string) (string, error) {
	arguments := m.Called(ctx, command, args)
	return arguments.String(0), arguments.Error(1)
}

func TestGetRemotes(t *testing.T) {
	tests := []struct {
		name       string
		mockOutput string
		mockError  error
		expected   []Remote
		expectErr  bool
	}{
		{
			name: "Single origin remote",
			mockOutput: `origin	https://github.com/owner/repo.git (fetch)
origin	https://github.com/owner/repo.git (push)`,
			expected: []Remote{
				{
					Name:     "origin",
					FetchURL: "https://github.com/owner/repo.git",
					PushURL:  "https://github.com/owner/repo.git",
				},
			},
		},
		{
			name: "Multiple remotes",
			mockOutput: `origin	https://github.com/owner/repo.git (fetch)
origin	https://github.com/owner/repo.git (push)
upstream	https://github.com/upstream/repo.git (fetch)
upstream	https://github.com/upstream/repo.git (push)`,
			expected: []Remote{
				{
					Name:     "origin",
					FetchURL: "https://github.com/owner/repo.git",
					PushURL:  "https://github.com/owner/repo.git",
				},
				{
					Name:     "upstream",
					FetchURL: "https://github.com/upstream/repo.git",
					PushURL:  "https://github.com/upstream/repo.git",
				},
			},
		},
		{
			name: "Different fetch and push URLs",
			mockOutput: `origin	https://github.com/owner/repo.git (fetch)
origin	git@github.com:owner/repo.git (push)`,
			expected: []Remote{
				{
					Name:     "origin",
					FetchURL: "https://github.com/owner/repo.git",
					PushURL:  "git@github.com:owner/repo.git",
				},
			},
		},
		{
			name:       "No remotes",
			mockOutput: "",
			expected:   []Remote{},
		},
		{
			name:      "Git command error",
			mockError: assert.AnError,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := new(MockRunner)
			if tt.mockError != nil {
				mockRunner.On("Run", mock.Anything, "git", []string{"remote", "-v"}).
					Return("", tt.mockError)
			} else {
				mockRunner.On("Run", mock.Anything, "git", []string{"remote", "-v"}).
					Return(tt.mockOutput, nil)
			}

			manager := NewRemoteManager(mockRunner)
			ctx := context.Background()

			remotes, err := manager.GetRemotes(ctx)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, remotes)
			}

			mockRunner.AssertExpectations(t)
		})
	}
}

func TestSelectRemote(t *testing.T) {
	tests := []struct {
		name           string
		remotes        []Remote
		preferredName  string
		expectedRemote *Remote
		expectErr      bool
		errContains    string
	}{
		{
			name: "Select origin by default",
			remotes: []Remote{
				{Name: "origin", FetchURL: "https://github.com/owner/repo.git"},
				{Name: "upstream", FetchURL: "https://github.com/upstream/repo.git"},
			},
			preferredName: "",
			expectedRemote: &Remote{
				Name:     "origin",
				FetchURL: "https://github.com/owner/repo.git",
			},
		},
		{
			name: "Select specified remote",
			remotes: []Remote{
				{Name: "origin", FetchURL: "https://github.com/owner/repo.git"},
				{Name: "upstream", FetchURL: "https://github.com/upstream/repo.git"},
			},
			preferredName: "upstream",
			expectedRemote: &Remote{
				Name:     "upstream",
				FetchURL: "https://github.com/upstream/repo.git",
			},
		},
		{
			name: "No origin and no preference",
			remotes: []Remote{
				{Name: "upstream", FetchURL: "https://github.com/upstream/repo.git"},
			},
			preferredName: "",
			expectErr:     true,
			errContains:   "no 'origin' remote found",
		},
		{
			name: "Specified remote not found",
			remotes: []Remote{
				{Name: "origin", FetchURL: "https://github.com/owner/repo.git"},
			},
			preferredName: "nonexistent",
			expectErr:     true,
			errContains:   "remote 'nonexistent' not found",
		},
		{
			name:           "No remotes at all",
			remotes:        []Remote{},
			preferredName:  "",
			expectErr:      true,
			errContains:    "no remotes configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &remoteManager{}
			remote, err := manager.SelectRemote(tt.remotes, tt.preferredName)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRemote, remote)
			}
		})
	}
}

func TestGetCurrentBranch(t *testing.T) {
	tests := []struct {
		name       string
		mockOutput string
		mockError  error
		expected   string
		expectErr  bool
	}{
		{
			name:       "Normal branch",
			mockOutput: "feature-branch\n",
			expected:   "feature-branch",
		},
		{
			name:       "Branch with special characters",
			mockOutput: "feature/add-new-feature\n",
			expected:   "feature/add-new-feature",
		},
		{
			name:       "Main branch",
			mockOutput: "main\n",
			expected:   "main",
		},
		{
			name:      "Detached HEAD state",
			mockError: assert.AnError,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := new(MockRunner)
			mockRunner.On("Run", mock.Anything, "git", []string{"branch", "--show-current"}).
				Return(tt.mockOutput, tt.mockError)

			manager := NewRemoteManager(mockRunner)
			ctx := context.Background()

			branch, err := manager.GetCurrentBranch(ctx)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, branch)
			}

			mockRunner.AssertExpectations(t)
		})
	}
}

func TestHasUpstreamBranch(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		mockOutput string
		mockError  error
		expected   bool
	}{
		{
			name:       "Branch with upstream",
			branch:     "feature-branch",
			mockOutput: "origin/feature-branch",
			expected:   true,
		},
		{
			name:      "Branch without upstream",
			branch:    "new-feature",
			mockError: assert.AnError,
			expected:  false,
		},
		{
			name:       "Main branch with upstream",
			branch:     "main",
			mockOutput: "origin/main",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := new(MockRunner)
			mockRunner.On("Run", mock.Anything, "git", []string{"rev-parse", "--abbrev-ref", tt.branch + "@{upstream}"}).
				Return(tt.mockOutput, tt.mockError)

			manager := NewRemoteManager(mockRunner)
			ctx := context.Background()

			hasUpstream := manager.HasUpstreamBranch(ctx, tt.branch)
			assert.Equal(t, tt.expected, hasUpstream)

			mockRunner.AssertExpectations(t)
		})
	}
}