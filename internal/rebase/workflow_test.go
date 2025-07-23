package rebase

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/penwyp/catmit/pkg/githistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockHistoryManager is a mock implementation of githistory.HistoryManager
type MockHistoryManager struct {
	mock.Mock
}

func (m *MockHistoryManager) GetCurrentBranch(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockHistoryManager) HasUncommittedChanges(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

func (m *MockHistoryManager) FindMergeBase(ctx context.Context, ref1, ref2 string) (string, error) {
	args := m.Called(ctx, ref1, ref2)
	return args.String(0), args.Error(1)
}

func (m *MockHistoryManager) GetUnpushedCommits(ctx context.Context, baseBranch, headRef string) ([]githistory.Commit, error) {
	args := m.Called(ctx, baseBranch, headRef)
	if commits := args.Get(0); commits != nil {
		return commits.([]githistory.Commit), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockHistoryManager) GetCommitsBetween(ctx context.Context, from, to string) ([]githistory.Commit, error) {
	args := m.Called(ctx, from, to)
	if commits := args.Get(0); commits != nil {
		return commits.([]githistory.Commit), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockHistoryManager) BackupBranch(ctx context.Context, branchName string) (string, error) {
	args := m.Called(ctx, branchName)
	return args.String(0), args.Error(1)
}

func (m *MockHistoryManager) RebaseInteractive(ctx context.Context, onto string, commits []githistory.Commit, newMessage string) error {
	args := m.Called(ctx, onto, commits, newMessage)
	return args.Error(0)
}

func (m *MockHistoryManager) ResetHard(ctx context.Context, ref string) error {
	args := m.Called(ctx, ref)
	return args.Error(0)
}

func (m *MockHistoryManager) AbortRebase(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockHistoryManager) CherryPick(ctx context.Context, commits []string) error {
	args := m.Called(ctx, commits)
	return args.Error(0)
}

func (m *MockHistoryManager) GetCommit(ctx context.Context, ref string) (*githistory.Commit, error) {
	args := m.Called(ctx, ref)
	if commit := args.Get(0); commit != nil {
		return commit.(*githistory.Commit), args.Error(1)
	}
	return nil, args.Error(1)
}

// MockSquashClient is a mock implementation of squash.ClientInterface
type MockSquashClient struct {
	mock.Mock
}

func (m *MockSquashClient) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	args := m.Called(ctx, prompt)
	return args.String(0), args.Error(1)
}

func TestNew(t *testing.T) {
	mockHistory := new(MockHistoryManager)
	mockClient := new(MockSquashClient)
	logger := zap.NewNop()

	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "with logger",
			config: Config{
				BaseBranch: "main",
				Language:   "en",
				Logger:     logger,
			},
		},
		{
			name: "without logger",
			config: Config{
				BaseBranch: "master",
				Language:   "zh",
				Logger:     nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflow := New(mockHistory, mockClient, tt.config)
			assert.NotNil(t, workflow)
			assert.Equal(t, mockHistory, workflow.history)
			assert.NotNil(t, workflow.squash)
			assert.Equal(t, tt.config.BaseBranch, workflow.config.BaseBranch)
			assert.Equal(t, tt.config.Language, workflow.config.Language)
			assert.NotNil(t, workflow.logger)
		})
	}
}

func TestWorkflow_Analyze(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	tests := []struct {
		name      string
		setupMock func(*MockHistoryManager)
		config    Config
		want      *AnalysisResult
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful analysis with multiple commits",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return([]githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: add feature"},
					{ShortSHA: "ghi789", Subject: "fix: fix bug"},
					{ShortSHA: "jkl012", Subject: "docs: update docs"},
				}, nil)
			},
			config: Config{BaseBranch: "main", Logger: logger},
			want: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: add feature"},
					{ShortSHA: "ghi789", Subject: "fix: fix bug"},
					{ShortSHA: "jkl012", Subject: "docs: update docs"},
				},
				HasChanges: false,
				CanRebase:  true,
				Message:    "Found 3 commits that can be squashed.",
			},
			wantErr: false,
		},
		{
			name: "uncommitted changes present",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(true, nil)
			},
			config: Config{BaseBranch: "main", Logger: logger},
			want: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				HasChanges:    true,
				CanRebase:     false,
				Message:       "Cannot rebase: You have uncommitted changes. Please commit or stash them first.",
			},
			wantErr: false,
		},
		{
			name: "no commits to rebase",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return([]githistory.Commit{}, nil)
			},
			config: Config{BaseBranch: "main", Logger: logger},
			want: &AnalysisResult{
				CurrentBranch:   "feature-branch",
				BaseBranch:      "main",
				MergeBase:       "abc123",
				UnpushedCommits: []githistory.Commit{},
				HasChanges:      false,
				CanRebase:       false,
				Message:         "No commits to rebase. Your branch is up to date with main.",
			},
			wantErr: false,
		},
		{
			name: "only one commit",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return([]githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: single commit"},
				}, nil)
			},
			config: Config{BaseBranch: "main", Logger: logger},
			want: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: single commit"},
				},
				HasChanges: false,
				CanRebase:  false,
				Message:    "Only one commit found. Nothing to squash.",
			},
			wantErr: false,
		},
		{
			name: "fallback to GetCommitsBetween when GetUnpushedCommits fails",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return(nil, errors.New("upstream not found"))
				m.On("GetCommitsBetween", ctx, "abc123", "HEAD").Return([]githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: add feature"},
					{ShortSHA: "ghi789", Subject: "fix: fix bug"},
				}, nil)
			},
			config: Config{BaseBranch: "main", Logger: logger},
			want: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: add feature"},
					{ShortSHA: "ghi789", Subject: "fix: fix bug"},
				},
				HasChanges: false,
				CanRebase:  true,
				Message:    "Found 2 commits that can be squashed.",
			},
			wantErr: false,
		},
		{
			name: "error getting current branch",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("", errors.New("not in a git repo"))
			},
			config:  Config{BaseBranch: "main", Logger: logger},
			wantErr: true,
			errMsg:  "failed to get current branch",
		},
		{
			name: "error checking uncommitted changes",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, errors.New("git error"))
			},
			config:  Config{BaseBranch: "main", Logger: logger},
			wantErr: true,
			errMsg:  "failed to check uncommitted changes",
		},
		{
			name: "error finding merge base",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("", errors.New("merge base not found"))
			},
			config:  Config{BaseBranch: "main", Logger: logger},
			wantErr: true,
			errMsg:  "failed to find merge base",
		},
		{
			name: "error getting commits",
			setupMock: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return(nil, errors.New("git error"))
				m.On("GetCommitsBetween", ctx, "abc123", "HEAD").Return(nil, errors.New("git error"))
			},
			config:  Config{BaseBranch: "main", Logger: logger},
			wantErr: true,
			errMsg:  "failed to get commits for rebase",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHistory := new(MockHistoryManager)
			mockClient := new(MockSquashClient)
			tt.setupMock(mockHistory)

			workflow := New(mockHistory, mockClient, tt.config)
			got, err := workflow.Analyze(ctx)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockHistory.AssertExpectations(t)
		})
	}
}

func TestWorkflow_GenerateCommitMessage(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	tests := []struct {
		name      string
		commits   []githistory.Commit
		lang      string
		mockResp  string
		mockError error
		want      string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful generation with multiple commits",
			commits: []githistory.Commit{
				{Subject: "feat: add user auth", Body: "Implemented JWT authentication"},
				{Subject: "fix: resolve login bug", Body: ""},
				{Subject: "docs: update README", Body: "Added auth section"},
			},
			lang:     "en",
			mockResp: "feat: implement complete authentication system",
			want:     "feat: implement complete authentication system",
			wantErr:  false,
		},
		{
			name: "successful generation in Chinese",
			commits: []githistory.Commit{
				{Subject: "feat: 添加用户认证", Body: "实现了JWT认证"},
				{Subject: "fix: 修复登录错误", Body: ""},
			},
			lang:     "zh",
			mockResp: "feat: 实现完整的认证系统",
			want:     "feat: 实现完整的认证系统",
			wantErr:  false,
		},
		{
			name: "error from squash client",
			commits: []githistory.Commit{
				{Subject: "feat: add feature"},
				{Subject: "fix: fix bug"},
			},
			lang:      "en",
			mockError: errors.New("API error"),
			wantErr:   true,
			errMsg:    "failed to generate commit message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHistory := new(MockHistoryManager)
			mockClient := new(MockSquashClient)

			if tt.mockError != nil {
				mockClient.On("GenerateCommitMessage", ctx, mock.Anything).Return("", tt.mockError)
			} else {
				mockClient.On("GenerateCommitMessage", ctx, mock.Anything).Return(tt.mockResp, nil)
			}

			config := Config{BaseBranch: "main", Language: tt.lang, Logger: logger}
			workflow := New(mockHistory, mockClient, config)

			got, err := workflow.GenerateCommitMessage(ctx, tt.commits)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestWorkflow_ExecuteRebase(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	tests := []struct {
		name         string
		analysis     *AnalysisResult
		newMessage   string
		setupMock    func(*MockHistoryManager)
		wantErr      bool
		errMsg       string
		wantRecovery bool
	}{
		{
			name: "successful rebase",
			analysis: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: add feature"},
					{ShortSHA: "ghi789", Subject: "fix: fix bug"},
				},
				CanRebase: true,
			},
			newMessage: "feat: consolidated feature implementation",
			setupMock: func(m *MockHistoryManager) {
				m.On("BackupBranch", ctx, "feature-branch").Return("feature-branch-backup-12345", nil)
				m.On("RebaseInteractive", ctx, "abc123", mock.Anything, "feat: consolidated feature implementation").Return(nil)
			},
			wantErr: false,
		},
		{
			name: "cannot rebase",
			analysis: &AnalysisResult{
				CanRebase: false,
				Message:   "No commits to rebase",
			},
			setupMock: func(m *MockHistoryManager) {},
			wantErr:   true,
			errMsg:    "cannot rebase: No commits to rebase",
		},
		{
			name: "backup branch creation fails",
			analysis: &AnalysisResult{
				CurrentBranch:   "feature-branch",
				UnpushedCommits: []githistory.Commit{{ShortSHA: "def456"}},
				CanRebase:       true,
			},
			setupMock: func(m *MockHistoryManager) {
				m.On("BackupBranch", ctx, "feature-branch").Return("", errors.New("cannot create branch"))
			},
			wantErr: true,
			errMsg:  "failed to create backup branch",
		},
		{
			name: "rebase fails",
			analysis: &AnalysisResult{
				CurrentBranch: "feature-branch",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: add feature"},
				},
				CanRebase: true,
			},
			newMessage: "feat: new message",
			setupMock: func(m *MockHistoryManager) {
				m.On("BackupBranch", ctx, "feature-branch").Return("feature-branch-backup-12345", nil)
				m.On("RebaseInteractive", ctx, "abc123", mock.Anything, "feat: new message").Return(errors.New("merge conflict"))
			},
			wantErr:      true,
			errMsg:       "rebase failed",
			wantRecovery: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHistory := new(MockHistoryManager)
			mockClient := new(MockSquashClient)
			tt.setupMock(mockHistory)

			config := Config{BaseBranch: "main", Logger: logger}
			workflow := New(mockHistory, mockClient, config)

			err := workflow.ExecuteRebase(ctx, tt.analysis, tt.newMessage)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				if tt.wantRecovery {
					assert.Contains(t, err.Error(), "To recover")
					assert.Contains(t, err.Error(), "git rebase --abort")
					assert.Contains(t, err.Error(), "git reset --hard")
				}
			} else {
				require.NoError(t, err)
			}

			mockHistory.AssertExpectations(t)
		})
	}
}

func TestFormatCommitList(t *testing.T) {
	tests := []struct {
		name    string
		commits []githistory.Commit
		want    string
	}{
		{
			name: "multiple commits",
			commits: []githistory.Commit{
				{ShortSHA: "abc123", Subject: "feat: add feature"},
				{ShortSHA: "def456", Subject: "fix: fix bug"},
				{ShortSHA: "ghi789", Subject: "docs: update docs"},
			},
			want: "  1. abc123 feat: add feature\n  2. def456 fix: fix bug\n  3. ghi789 docs: update docs",
		},
		{
			name:    "empty commits",
			commits: []githistory.Commit{},
			want:    "",
		},
		{
			name: "single commit",
			commits: []githistory.Commit{
				{ShortSHA: "abc123", Subject: "feat: single feature"},
			},
			want: "  1. abc123 feat: single feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCommitList(tt.commits)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetRecoveryInstructions(t *testing.T) {
	backupBranch := "feature-branch-backup-12345"
	got := GetRecoveryInstructions(backupBranch)

	assert.Contains(t, got, "If something went wrong")
	assert.Contains(t, got, "git rebase --abort")
	assert.Contains(t, got, fmt.Sprintf("git reset --hard %s", backupBranch))
	assert.Contains(t, got, fmt.Sprintf("git branch -D %s", backupBranch))
}