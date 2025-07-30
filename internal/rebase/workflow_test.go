package rebase

import (
	"context"
	"errors"
	"testing"

	"github.com/penwyp/catmit/pkg/githistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func (m *MockHistoryManager) GetUnpushedCommits(ctx context.Context, base, head string) ([]githistory.Commit, error) {
	args := m.Called(ctx, base, head)
	return args.Get(0).([]githistory.Commit), args.Error(1)
}

func (m *MockHistoryManager) GetCommitsBetween(ctx context.Context, base, head string) ([]githistory.Commit, error) {
	args := m.Called(ctx, base, head)
	return args.Get(0).([]githistory.Commit), args.Error(1)
}

func (m *MockHistoryManager) BackupBranch(ctx context.Context, branchName string) (string, error) {
	args := m.Called(ctx, branchName)
	return args.String(0), args.Error(1)
}

func (m *MockHistoryManager) RebaseInteractive(ctx context.Context, base string, commits []githistory.Commit, message string) error {
	args := m.Called(ctx, base, commits, message)
	return args.Error(0)
}

func (m *MockHistoryManager) AbortRebase(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockHistoryManager) GetCommit(ctx context.Context, ref string) (*githistory.Commit, error) {
	args := m.Called(ctx, ref)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*githistory.Commit), args.Error(1)
}

func (m *MockHistoryManager) ResetHard(ctx context.Context, ref string) error {
	args := m.Called(ctx, ref)
	return args.Error(0)
}

func (m *MockHistoryManager) CherryPick(ctx context.Context, commits []string) error {
	args := m.Called(ctx, commits)
	return args.Error(0)
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
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "with logger",
			config: Config{
				BaseBranch: "main",
				Language:   "en",
				Logger:     zap.NewNop(),
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
			mockHistory := new(MockHistoryManager)
			mockClient := new(MockSquashClient)

			w := New(mockHistory, mockClient, tt.config)

			assert.NotNil(t, w)
			assert.Equal(t, tt.config.BaseBranch, w.config.BaseBranch)
			assert.Equal(t, tt.config.Language, w.config.Language)
			assert.NotNil(t, w.logger)
			assert.NotNil(t, w.squash)
			assert.Equal(t, mockHistory, w.history)
		})
	}
}

func TestAnalyze(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		setupMocks     func(*MockHistoryManager)
		expectedResult *AnalysisResult
		expectedError  string
	}{
		{
			name: "successful analysis with multiple commits",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return([]githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: add feature"},
					{ShortSHA: "ghi789", Subject: "fix: bug fix"},
					{ShortSHA: "jkl012", Subject: "docs: update readme"},
				}, nil)
			},
			expectedResult: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: add feature"},
					{ShortSHA: "ghi789", Subject: "fix: bug fix"},
					{ShortSHA: "jkl012", Subject: "docs: update readme"},
				},
				HasChanges: false,
				CanRebase:  true,
				Message:    "Found 3 commits that can be squashed.",
			},
		},
		{
			name: "has uncommitted changes",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(true, nil)
			},
			expectedResult: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				HasChanges:    true,
				CanRebase:     false,
				Message:       "Cannot rebase: You have uncommitted changes. Please commit or stash them first.",
			},
		},
		{
			name: "no commits to rebase",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return([]githistory.Commit{}, nil)
			},
			expectedResult: &AnalysisResult{
				CurrentBranch:   "feature-branch",
				BaseBranch:      "main",
				MergeBase:       "abc123",
				UnpushedCommits: []githistory.Commit{},
				HasChanges:      false,
				CanRebase:       false,
				Message:         "No commits to rebase. Your branch is up to date with main.",
			},
		},
		{
			name: "only one commit",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return([]githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: single commit"},
				}, nil)
			},
			expectedResult: &AnalysisResult{
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
		},
		{
			name: "fallback to GetCommitsBetween",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return([]githistory.Commit{}, errors.New("unpushed failed"))
				m.On("GetCommitsBetween", ctx, "abc123", "HEAD").Return([]githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: commit 1"},
					{ShortSHA: "ghi789", Subject: "feat: commit 2"},
				}, nil)
			},
			expectedResult: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: commit 1"},
					{ShortSHA: "ghi789", Subject: "feat: commit 2"},
				},
				HasChanges: false,
				CanRebase:  true,
				Message:    "Found 2 commits that can be squashed.",
			},
		},
		{
			name: "error getting current branch",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("", errors.New("git error"))
			},
			expectedError: "failed to get current branch: git error",
		},
		{
			name: "error checking uncommitted changes",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, errors.New("check error"))
			},
			expectedError: "failed to check uncommitted changes: check error",
		},
		{
			name: "error finding merge base",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("", errors.New("merge base error"))
			},
			expectedError: "failed to find merge base with main: merge base error",
		},
		{
			name: "error getting commits",
			setupMocks: func(m *MockHistoryManager) {
				m.On("GetCurrentBranch", ctx).Return("feature-branch", nil)
				m.On("HasUncommittedChanges", ctx).Return(false, nil)
				m.On("FindMergeBase", ctx, "main", "HEAD").Return("abc123", nil)
				m.On("GetUnpushedCommits", ctx, "main", "HEAD").Return([]githistory.Commit{}, errors.New("unpushed failed"))
				m.On("GetCommitsBetween", ctx, "abc123", "HEAD").Return([]githistory.Commit{}, errors.New("between failed"))
			},
			expectedError: "failed to get commits for rebase: between failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHistory := new(MockHistoryManager)
			mockClient := new(MockSquashClient)
			tt.setupMocks(mockHistory)

			w := New(mockHistory, mockClient, Config{
				BaseBranch: "main",
				Language:   "en",
				Logger:     zap.NewNop(),
			})

			result, err := w.Analyze(ctx)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}

			mockHistory.AssertExpectations(t)
		})
	}
}

func TestGenerateCommitMessage(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		commits       []githistory.Commit
		mockResponse  string
		mockError     error
		expectedMsg   string
		expectedError string
	}{
		{
			name: "successful generation with multiple commits",
			commits: []githistory.Commit{
				{Subject: "feat: add feature", Body: "This adds a new feature"},
				{Subject: "fix: bug fix"},
				{Subject: "docs: update readme", Body: "Updated documentation\nwith examples"},
			},
			mockResponse: "feat: comprehensive feature update with bug fixes and documentation",
			expectedMsg:  "feat: comprehensive feature update with bug fixes and documentation",
		},
		{
			name: "generation error",
			commits: []githistory.Commit{
				{Subject: "feat: add feature"},
				{Subject: "fix: bug fix"},
			},
			mockError:     errors.New("API error"),
			expectedError: "failed to generate commit message: API error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHistory := new(MockHistoryManager)
			mockClient := new(MockSquashClient)


			// Mock the squash client's behavior
			if tt.mockError != nil {
				mockClient.On("GenerateCommitMessage", ctx, mock.Anything).Return("", tt.mockError)
			} else {
				mockClient.On("GenerateCommitMessage", ctx, mock.Anything).Return(tt.mockResponse, nil)
			}

			w := New(mockHistory, mockClient, Config{
				BaseBranch: "main",
				Language:   "en",
			})

			msg, err := w.GenerateCommitMessage(ctx, tt.commits)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedMsg, msg)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestExecuteRebase(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		analysis      *AnalysisResult
		newMessage    string
		setupMocks    func(*MockHistoryManager)
		expectedError string
	}{
		{
			name: "successful rebase",
			analysis: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: commit 1"},
					{ShortSHA: "ghi789", Subject: "feat: commit 2"},
				},
				CanRebase: true,
			},
			newMessage: "feat: consolidated feature",
			setupMocks: func(m *MockHistoryManager) {
				m.On("BackupBranch", ctx, "feature-branch").Return("backup-feature-branch", nil)
				m.On("RebaseInteractive", ctx, "abc123", mock.Anything, "feat: consolidated feature").Return(nil)
			},
		},
		{
			name: "cannot rebase",
			analysis: &AnalysisResult{
				CanRebase: false,
				Message:   "No commits to rebase",
			},
			expectedError: "cannot rebase: No commits to rebase",
		},
		{
			name: "backup branch error",
			analysis: &AnalysisResult{
				CurrentBranch: "feature-branch",
				CanRebase:     true,
			},
			setupMocks: func(m *MockHistoryManager) {
				m.On("BackupBranch", ctx, "feature-branch").Return("", errors.New("backup failed"))
			},
			expectedError: "failed to create backup branch: backup failed",
		},
		{
			name: "rebase interactive error",
			analysis: &AnalysisResult{
				CurrentBranch: "feature-branch",
				BaseBranch:    "main",
				MergeBase:     "abc123",
				UnpushedCommits: []githistory.Commit{
					{ShortSHA: "def456", Subject: "feat: commit 1"},
				},
				CanRebase: true,
			},
			newMessage: "feat: consolidated",
			setupMocks: func(m *MockHistoryManager) {
				m.On("BackupBranch", ctx, "feature-branch").Return("backup-feature-branch", nil)
				m.On("RebaseInteractive", ctx, "abc123", mock.Anything, "feat: consolidated").Return(errors.New("rebase failed"))
			},
			expectedError: "rebase failed: rebase failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHistory := new(MockHistoryManager)
			mockClient := new(MockSquashClient)

			if tt.setupMocks != nil {
				tt.setupMocks(mockHistory)
			}

			w := New(mockHistory, mockClient, Config{
				BaseBranch: "main",
				Logger:     zap.NewNop(),
			})

			err := w.ExecuteRebase(ctx, tt.analysis, tt.newMessage)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
			}

			mockHistory.AssertExpectations(t)
		})
	}
}

func TestFormatCommitList(t *testing.T) {
	tests := []struct {
		name     string
		commits  []githistory.Commit
		expected string
	}{
		{
			name: "multiple commits",
			commits: []githistory.Commit{
				{ShortSHA: "abc123", Subject: "feat: add feature"},
				{ShortSHA: "def456", Subject: "fix: bug fix"},
				{ShortSHA: "ghi789", Subject: "docs: update readme"},
			},
			expected: "  1. abc123 feat: add feature\n  2. def456 fix: bug fix\n  3. ghi789 docs: update readme",
		},
		{
			name:     "empty commits",
			commits:  []githistory.Commit{},
			expected: "",
		},
		{
			name: "single commit",
			commits: []githistory.Commit{
				{ShortSHA: "xyz789", Subject: "chore: cleanup"},
			},
			expected: "  1. xyz789 chore: cleanup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCommitList(tt.commits)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetRecoveryInstructions(t *testing.T) {
	backupBranch := "backup-feature-123"
	expected := `If something went wrong, you can recover using one of these commands:

1. Abort the rebase (if still in progress):
   git rebase --abort

2. Reset to the backup branch:
   git reset --hard backup-feature-123

3. Delete the backup branch when done:
   git branch -D backup-feature-123`

	result := GetRecoveryInstructions(backupBranch)
	assert.Equal(t, expected, result)
}