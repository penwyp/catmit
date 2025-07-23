package ui

import (
	"context"
	"testing"

	"github.com/penwyp/catmit/internal/rebase"
	"github.com/penwyp/catmit/pkg/githistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func (m *MockHistoryManager) GetCommit(ctx context.Context, ref string) (*githistory.Commit, error) {
	args := m.Called(ctx, ref)
	if commit := args.Get(0); commit != nil {
		return commit.(*githistory.Commit), args.Error(1)
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

// MockSquashClient is a mock implementation of squash.ClientInterface
type MockSquashClient struct {
	mock.Mock
}

func (m *MockSquashClient) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	args := m.Called(ctx, prompt)
	return args.String(0), args.Error(1)
}

func TestNewRebaseModel(t *testing.T) {
	mockHistory := new(MockHistoryManager)
	mockClient := new(MockSquashClient)
	config := rebase.Config{BaseBranch: "main"}
	workflow := rebase.New(mockHistory, mockClient, config)
	
	model := NewRebaseModel(workflow)

	assert.NotNil(t, model)
	assert.Equal(t, workflow, model.workflow)
	assert.Equal(t, RebasePhaseAnalyzing, model.phase)
	assert.False(t, model.accepted)
	assert.Nil(t, model.analysis)
	assert.Empty(t, model.message)
	assert.Empty(t, model.backupBranch)
	assert.Nil(t, model.error)
}

func TestRebaseModel_Init(t *testing.T) {
	mockHistory := new(MockHistoryManager)
	mockClient := new(MockSquashClient)
	config := rebase.Config{BaseBranch: "main"}
	workflow := rebase.New(mockHistory, mockClient, config)
	model := NewRebaseModel(workflow)

	cmd := model.Init()
	assert.NotNil(t, cmd)
	// The Init command should start the analysis process
}

func TestRebaseModel_IsAccepted(t *testing.T) {
	mockHistory := new(MockHistoryManager)
	mockClient := new(MockSquashClient)
	config := rebase.Config{BaseBranch: "main"}
	workflow := rebase.New(mockHistory, mockClient, config)
	model := NewRebaseModel(workflow)

	// Initially not accepted
	assert.False(t, model.IsAccepted())

	// Set accepted
	model.accepted = true
	assert.True(t, model.IsAccepted())
}

func TestRebaseModel_GetBackupBranch(t *testing.T) {
	mockHistory := new(MockHistoryManager)
	mockClient := new(MockSquashClient)
	config := rebase.Config{BaseBranch: "main"}
	workflow := rebase.New(mockHistory, mockClient, config)
	model := NewRebaseModel(workflow)

	// Initially empty
	assert.Empty(t, model.GetBackupBranch())

	// Set backup branch
	expectedBranch := "feature-backup-12345"
	model.backupBranch = expectedBranch
	assert.Equal(t, expectedBranch, model.GetBackupBranch())
}

func TestRebaseModel_GetResult(t *testing.T) {
	mockHistory := new(MockHistoryManager)
	mockClient := new(MockSquashClient)
	config := rebase.Config{BaseBranch: "main"}
	workflow := rebase.New(mockHistory, mockClient, config)
	model := NewRebaseModel(workflow)

	// Initially empty
	assert.Empty(t, model.GetResult())

	// Set result
	expectedResult := "feat: consolidated commit message"
	model.result = expectedResult
	assert.Equal(t, expectedResult, model.GetResult())
}