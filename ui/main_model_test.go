package ui

import (
	"context"
	"errors"
	"strings"
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
		{PhasePRPreview, false, "Pull Request Preview"},
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

func TestMainModel_StartCreatePR(t *testing.T) {
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
		true, // createPR enabled
	)

	// Test successful PR creation
	mockCommitter.On("CreatePullRequest", ctx).Return("https://github.com/user/repo/pull/123", nil)
	
	cmd := model.startCreatePR()
	assert.NotNil(t, cmd)
	
	// Execute the command
	msg := cmd()
	prMsg, ok := msg.(createPRDoneMsg)
	assert.True(t, ok)
	assert.NoError(t, prMsg.err)
	assert.Equal(t, "https://github.com/user/repo/pull/123", prMsg.prURL)
	
	mockCommitter.AssertExpectations(t)
}

func TestMainModel_StartCreatePR_Error(t *testing.T) {
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
		true,
	)

	// Test PR creation error
	testErr := errors.New("PR creation failed")
	mockCommitter.On("CreatePullRequest", ctx).Return("", testErr)
	
	cmd := model.startCreatePR()
	msg := cmd()
	prMsg, ok := msg.(createPRDoneMsg)
	assert.True(t, ok)
	assert.Equal(t, testErr, prMsg.err)
	assert.Empty(t, prMsg.prURL)
	
	mockCommitter.AssertExpectations(t)
}

func TestMainModel_PreparePRPreview(t *testing.T) {
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
		true,
	)

	// Set up test data
	model.message = "feat: add new feature\n\nThis is a detailed description\nof the new feature"
	
	// Mock collector methods
	mockCollector.On("BranchName", ctx).Return("feature-branch", nil)
	mockCollector.On("ChangedFiles", ctx).Return([]string{"file1.go", "file2.go"}, nil)
	
	cmd := model.preparePRPreview()
	assert.NotNil(t, cmd)
	
	// Execute the command
	msg := cmd()
	prMsg, ok := msg.(prPreviewReadyMsg)
	assert.True(t, ok)
	assert.Equal(t, "feat: add new feature", prMsg.data.Title)
	assert.Contains(t, prMsg.data.Body, "This is a detailed description")
	assert.Equal(t, "", prMsg.data.Base) // Default is empty
	assert.Equal(t, "feature-branch", prMsg.data.Head)
	assert.Equal(t, "origin", prMsg.data.Remote) // Default is "origin"
	assert.Equal(t, "", prMsg.data.Provider) // Default is empty
	assert.Len(t, prMsg.data.FileChanges, 2)
	
	mockCollector.AssertExpectations(t)
}

func TestMainModel_RenderPRPreviewContent(t *testing.T) {
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
		true,
	)

	// Test when PR preview is not ready
	content := model.renderPRPreviewContent()
	assert.Contains(t, content, "Preparing PR preview...")
	
	// Test when PR preview is ready
	prData := PRPreviewData{
		Title:    "feat: add feature",
		Body:     "Description",
		Base:     "main",
		Head:     "feature",
		Remote:   "origin",
		Provider: "github",
	}
	model.prPreview = NewPRPreviewModel(prData, model.styles, 80)
	
	content = model.renderPRPreviewContent()
	assert.Contains(t, content, "Pull Request Preview")
	assert.Contains(t, content, "feat: add feature")
}

func TestMainModel_UpdatePRPreview(t *testing.T) {
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
		true,
	)

	// Set up PR preview
	prData := PRPreviewData{
		Title: "feat: add feature",
		Body: strings.Join([]string{
			"Line 1", "Line 2", "Line 3", "Line 4", "Line 5",
			"Line 6", "Line 7", "Line 8",
		}, "\n"),
		Base:     "main",
		Head:     "feature",
		Remote:   "origin",
		Provider: "github",
	}
	model.prPreview = NewPRPreviewModel(prData, model.styles, 80)
	model.phase = PhasePRPreview

	// Test toggle details with 'd' key
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	newModel, cmd := model.updatePRPreview(msg)
	m := newModel.(*MainModel)
	assert.Nil(t, cmd)
	assert.True(t, m.prPreview.showDetails)

	// Test toggle back
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}}
	newModel, cmd = model.updatePRPreview(msg)
	m = newModel.(*MainModel)
	assert.Nil(t, cmd)
	assert.False(t, m.prPreview.showDetails)

	// Test continue with enter key
	msg = tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd = model.updatePRPreview(msg)
	assert.NotNil(t, cmd)

	// Test continue with space key
	msg = tea.KeyMsg{Type: tea.KeySpace}
	_, cmd = model.updatePRPreview(msg)
	assert.NotNil(t, cmd)

	// Test cancel with 'q' key (since "esc" string is checked, not tea.KeyEscape)
	msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	newModel, cmd = model.updatePRPreview(msg)
	m = newModel.(*MainModel)
	assert.NotNil(t, cmd)
	assert.True(t, m.done)
	assert.Equal(t, DecisionCancel, m.reviewDecision)
	assert.NoError(t, m.err) // No error is set, just done flag
}

func TestMainModel_PRPreviewPhaseTransition(t *testing.T) {
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
		true, // createPR enabled
	)

	// Simulate transition to PR preview phase
	prData := PRPreviewData{
		Title:    "feat: add feature",
		Body:     "Description",
		Base:     "main",
		Head:     "feature",
		Remote:   "origin",
		Provider: "github",
	}
	
	msg := prPreviewReadyMsg{data: prData}
	newModel, cmd := model.Update(msg)
	m := newModel.(*MainModel)
	
	assert.Nil(t, cmd)
	assert.Equal(t, PhasePRPreview, m.phase)
	assert.NotNil(t, m.prPreview)
	assert.Equal(t, "feat: add feature", m.prPreview.data.Title)
}

func TestMainModel_CreatePRDoneMsg(t *testing.T) {
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
		true,
	)

	// Test successful PR creation
	model.phase = PhaseCommit
	model.commitStage = CommitStageCreatingPR
	
	msg := createPRDoneMsg{
		prURL: "https://github.com/user/repo/pull/123",
		err:   nil,
	}
	newModel, cmd := model.Update(msg)
	m := newModel.(*MainModel)
	
	assert.NotNil(t, cmd)
	assert.Equal(t, CommitStagePRCreated, m.commitStage) // Should be PRCreated, not Done
	assert.Equal(t, "https://github.com/user/repo/pull/123", m.prURL)
	assert.NoError(t, m.err)

	// Test PR creation with error
	model.phase = PhaseCommit
	model.commitStage = CommitStageCreatingPR
	model.prURL = ""
	
	testErr := errors.New("PR creation failed")
	msg = createPRDoneMsg{
		prURL: "",
		err:   testErr,
	}
	newModel, cmd = model.Update(msg)
	m = newModel.(*MainModel)
	
	assert.NotNil(t, cmd)
	assert.Equal(t, CommitStagePRFailed, m.commitStage) // Should be PRFailed
	assert.Empty(t, m.prURL)
	assert.Equal(t, testErr, m.err)
}

func TestMainModel_PRPreviewWithTemplate(t *testing.T) {
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
		true,
	)

	// Set up test data with template
	model.message = "feat: add new feature\n\nThis uses a template"
	
	// Mock collector methods
	mockCollector.On("BranchName", ctx).Return("feature-branch", nil)
	mockCollector.On("ChangedFiles", ctx).Return([]string{"file1.go"}, nil)
	
	cmd := model.preparePRPreview()
	msg := cmd()
	prMsg, ok := msg.(prPreviewReadyMsg)
	assert.True(t, ok)
	
	// The template detection logic would be in the actual implementation
	// For now we just verify the data structure supports it
	assert.False(t, prMsg.data.UsingTemplate) // Default is false
	assert.Empty(t, prMsg.data.TemplateName)
	
	mockCollector.AssertExpectations(t)
}