package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/collector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCollector implements collectorInterface
type MockCollector struct {
	mock.Mock
}

func (m *MockCollector) RecentCommits(ctx context.Context, n int) ([]string, error) {
	args := m.Called(ctx, n)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockCollector) Diff(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockCollector) BranchName(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockCollector) ChangedFiles(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockCollector) FileStatusSummary(ctx context.Context) (*collector.FileStatusSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*collector.FileStatusSummary), args.Error(1)
}

func (m *MockCollector) ComprehensiveDiff(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockCollector) AnalyzeChanges(ctx context.Context) (*collector.ChangesSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*collector.ChangesSummary), args.Error(1)
}

// MockPromptBuilder implements promptInterface
type MockPromptBuilder struct {
	mock.Mock
}

func (m *MockPromptBuilder) Build(seed, diff string, commits []string, branch string, files []string) string {
	args := m.Called(seed, diff, commits, branch, files)
	return args.String(0)
}

func (m *MockPromptBuilder) BuildSystemPrompt() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockPromptBuilder) BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string {
	args := m.Called(seed, diff, commits, branch, files)
	return args.String(0)
}

func (m *MockPromptBuilder) BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error) {
	args := m.Called(ctx, collector, seed)
	return args.String(0), args.Error(1)
}

// MockClient implements clientInterface
type MockClient struct {
	mock.Mock
}

func (m *MockClient) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := m.Called(ctx, systemPrompt, userPrompt)
	return args.String(0), args.Error(1)
}


// MockCommitter implements commitInterface
type MockCommitter struct {
	mock.Mock
}

func (m *MockCommitter) Commit(ctx context.Context, message string) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

func (m *MockCommitter) Push(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockCommitter) StageAll(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockCommitter) HasStagedChanges(ctx context.Context) bool {
	args := m.Called(ctx)
	return args.Bool(0)
}

func (m *MockCommitter) CreatePullRequest(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockCommitter) NeedsPush(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

func TestMainModel_NewMainModel(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"test seed",
		"en",
		30*time.Second,
		true,
		true,
		false,
	)

	assert.NotNil(t, model)
	assert.Equal(t, PhaseLoading, model.phase)
	assert.Equal(t, StageCollect, model.loadingStage)
	assert.Equal(t, "test seed", model.seed)
	assert.Equal(t, "en", model.lang)
	assert.True(t, model.enablePush)
	assert.True(t, model.stageAll)
}

func TestMainModel_Init(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"",
		"en",
		30*time.Second,
		false,
		false,
		false,
	)

	cmd := model.Init()
	assert.NotNil(t, cmd)
}

func TestMainModel_Update_WindowSizeMsg(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"",
		"en",
		30*time.Second,
		false,
		false,
		false,
	)

	msg := tea.WindowSizeMsg{Width: 100, Height: 40}
	newModel, _ := model.Update(msg)
	m := newModel.(*MainModel)

	assert.Equal(t, 100, m.terminalWidth)
	assert.Equal(t, 40, m.terminalHeight)
}

func TestMainModel_Update_CtrlC(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"",
		"en",
		30*time.Second,
		false,
		false,
		false,
	)

	msg := tea.KeyMsg{Type: tea.KeyCtrlC}
	newModel, cmd := model.Update(msg)
	m := newModel.(*MainModel)

	assert.NotNil(t, cmd)
	assert.True(t, m.done)
	assert.Equal(t, context.Canceled, m.err)
}

func TestMainModel_View_LoadingPhase(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"",
		"en",
		30*time.Second,
		false,
		false,
		false,
	)

	view := model.View()
	assert.Contains(t, view, "Generating Message")
	assert.Contains(t, view, "Collecting diff")
}

func TestMainModel_View_ReviewPhase(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"",
		"en",
		30*time.Second,
		false,
		false,
		false,
	)

	model.phase = PhaseReview
	model.message = "feat: add new feature"

	view := model.View()
	assert.Contains(t, view, "Commit Preview")
	assert.Contains(t, view, "feat: add new feature")
	assert.Contains(t, view, "Accept")
	assert.Contains(t, view, "Edit")
	assert.Contains(t, view, "Cancel")
}

func TestMainModel_View_CommitPhase(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"",
		"en",
		30*time.Second,
		false,
		false,
		false,
	)

	model.phase = PhaseCommit
	model.message = "feat: add new feature"
	model.commitStage = CommitStageCommitting

	view := model.View()
	assert.Contains(t, view, "Commit Progress")
	assert.Contains(t, view, "Message:")
	assert.Contains(t, view, "Committing changes...")
}

func TestMainModel_GetPhaseTitle(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"",
		"en",
		30*time.Second,
		false,
		false,
		false,
	)

	tests := []struct {
		phase    Phase
		editing  bool
		expected string
	}{
		{PhaseLoading, false, "Generating Message"},
		{PhaseReview, false, "Commit Preview"},
		{PhaseReview, true, "Edit Message"},
		{PhaseCommit, false, "Commit Progress"},
		{PhaseDone, false, "Catmit"},
	}

	for _, tt := range tests {
		model.phase = tt.phase
		model.editing = tt.editing
		assert.Equal(t, tt.expected, model.getPhaseTitle())
	}
}

func TestMainModel_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	mockCollector := new(MockCollector)
	mockPrompt := new(MockPromptBuilder)
	mockClient := new(MockClient)
	mockCommitter := new(MockCommitter)

	model := NewMainModel(
		ctx,
		mockCollector,
		mockPrompt,
		mockClient,
		mockCommitter,
		"",
		"en",
		30*time.Second,
		false,
		false,
		false,
	)

	// Test error message handling
	testErr := errors.New("test error")
	msg := errorMsg{err: testErr}
	newModel, cmd := model.Update(msg)
	m := newModel.(*MainModel)

	assert.NotNil(t, cmd)
	assert.True(t, m.done)
	assert.Equal(t, testErr, m.err)
}