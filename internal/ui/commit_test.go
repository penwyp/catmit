package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/pkg/gitinfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for all required interfaces

// mockCommitCollector implements collectorInterface
type mockCommitCollector struct {
	mock.Mock
}

func (m *mockCommitCollector) RecentCommits(ctx context.Context, n int) ([]string, error) {
	args := m.Called(ctx, n)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockCommitCollector) BranchName(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockCommitCollector) ChangedFiles(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	return args.Get(0).([]string), args.Error(1)
}

func (m *mockCommitCollector) FileStatusSummary(ctx context.Context) (*gitinfo.FileStatusSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gitinfo.FileStatusSummary), args.Error(1)
}

func (m *mockCommitCollector) ComprehensiveDiff(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockCommitCollector) AnalyzeChanges(ctx context.Context) (*gitinfo.ChangesSummary, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*gitinfo.ChangesSummary), args.Error(1)
}

// mockCommitPromptBuilder implements promptInterface
type mockCommitPromptBuilder struct {
	mock.Mock
}

func (m *mockCommitPromptBuilder) Build(seed, diff string, commits []string, branch string, files []string) string {
	args := m.Called(seed, diff, commits, branch, files)
	return args.String(0)
}

func (m *mockCommitPromptBuilder) BuildSystemPrompt() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockCommitPromptBuilder) BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string {
	args := m.Called(seed, diff, commits, branch, files)
	return args.String(0)
}

func (m *mockCommitPromptBuilder) BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error) {
	args := m.Called(ctx, collector, seed)
	return args.String(0), args.Error(1)
}

func (m *mockCommitPromptBuilder) BuildPRSystemPrompt() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockCommitPromptBuilder) BuildPRUserPrompt(commits []string) string {
	args := m.Called(commits)
	return args.String(0)
}

// mockCommitClient implements clientInterface
type mockCommitClient struct {
	mock.Mock
}

func (m *mockCommitClient) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	args := m.Called(ctx, systemPrompt, userPrompt)
	return args.String(0), args.Error(1)
}

func (m *mockCommitClient) GetCommitMessageStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
	args := m.Called(ctx, systemPrompt, userPrompt)
	
	contentChan := make(chan string, 1)
	errChan := make(chan error, 1)
	
	go func() {
		defer close(contentChan)
		defer close(errChan)
		
		if args.Error(1) != nil {
			errChan <- args.Error(1)
			return
		}
		contentChan <- args.String(0)
	}()
	
	return contentChan, errChan
}

// mockCommitCommitter implements commitInterface
type mockCommitCommitter struct {
	mock.Mock
}

func (m *mockCommitCommitter) Commit(ctx context.Context, message string) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

func (m *mockCommitCommitter) Push(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockCommitCommitter) StageAll(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockCommitCommitter) HasStagedChanges(ctx context.Context) bool {
	args := m.Called(ctx)
	return args.Bool(0)
}

func (m *mockCommitCommitter) CreatePullRequest(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *mockCommitCommitter) NeedsPush(ctx context.Context) (bool, error) {
	args := m.Called(ctx)
	return args.Bool(0), args.Error(1)
}

// Helper function to create a CommitWorkflowModel with mocks
func createTestCommitWorkflowModel() (*CommitWorkflowModel, *mockCommitCollector, *mockCommitPromptBuilder, *mockCommitClient, *mockCommitCommitter) {
	ctx := context.Background()
	collector := &mockCommitCollector{}
	promptBuilder := &mockCommitPromptBuilder{}
	client := &mockCommitClient{}
	committer := &mockCommitCommitter{}
	
	prConfig := PRConfig{
		CreatePR:    false,
		Remote:      "origin",
		Base:        "main",
		Draft:       false,
		Provider:    "github",
		UseTemplate: false,
	}
	
	model := NewCommitWorkflowModel(
		ctx,
		collector,
		promptBuilder,
		client,
		committer,
		"test seed",
		"en",
		5*time.Second,
		true,  // enablePush
		true,  // stageAll
		prConfig,
	)
	
	return model, collector, promptBuilder, client, committer
}

func TestNewCommitWorkflowModel(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()

	assert.NotNil(t, model)
	assert.Equal(t, WorkflowPhaseLoading, model.phase)
	assert.Equal(t, StageCollect, model.loadingStage)
	assert.Equal(t, "test seed", model.seed)
	assert.Equal(t, "en", model.lang)
	assert.True(t, model.enablePush)
	assert.True(t, model.stageAll)
	assert.Equal(t, 80, model.width)
	assert.Equal(t, 24, model.height)
}

func TestCommitWorkflowModel_Update_WindowSize(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()

	// Test window size update
	newModel, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	updatedModel := newModel.(*CommitWorkflowModel)

	assert.Equal(t, 120, updatedModel.width)
	assert.Equal(t, 30, updatedModel.height)
}

func TestCommitWorkflowModel_Update_CtrlC(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()

	// Test Ctrl+C cancellation
	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updatedModel := newModel.(*CommitWorkflowModel)

	assert.True(t, updatedModel.done)
	assert.Equal(t, context.Canceled, updatedModel.err)
	assert.NotNil(t, cmd) // Just check that quit command is returned
}

func TestCommitWorkflowModel_CommitSuccess_NoPush(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.enablePush = false
	model.createPR = false
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCommitting

	// Test successful commit without push
	newModel, _ := model.Update(commitDoneMsg{err: nil})
	updatedModel := newModel.(*CommitWorkflowModel)

	// Without push and PR, it should go directly to done
	assert.Equal(t, CommitStageDone, updatedModel.commitStage)
}

func TestCommitWorkflowModel_CommitSuccess_WithPush(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCommitting
	model.createPR = false // Disable PR for this test

	// Test successful commit, should proceed to push
	newModel, _ := model.Update(commitDoneMsg{err: nil})
	updatedModel := newModel.(*CommitWorkflowModel)

	assert.Equal(t, CommitStageCommitted, updatedModel.commitStage)

	// Simulate delayed push message
	newModel2, _ := updatedModel.Update(delayedPushMsg{})
	updatedModel2 := newModel2.(*CommitWorkflowModel)
	assert.Equal(t, CommitStagePushing, updatedModel2.commitStage)

	// Test successful push
	newModel3, _ := updatedModel2.Update(pushDoneMsg{err: nil})
	finalModel := newModel3.(*CommitWorkflowModel)

	// Without PR, it should go to done after push
	assert.Equal(t, CommitStageDone, finalModel.commitStage)
}

func TestCommitWorkflowModel_CommitError(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	commitErr := errors.New("commit failed")

	// Test commit error
	newModel, cmd := model.Update(commitDoneMsg{err: commitErr})
	updatedModel := newModel.(*CommitWorkflowModel)

	assert.True(t, updatedModel.done)
	assert.Equal(t, commitErr, updatedModel.err)
	assert.NotNil(t, cmd) // Just check that quit command is returned
}

func TestCommitWorkflowModel_PushError(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStagePushing
	pushErr := errors.New("push failed")

	// Test push error
	newModel, _ := model.Update(pushDoneMsg{err: pushErr})
	updatedModel := newModel.(*CommitWorkflowModel)

	assert.Equal(t, CommitStagePushFailed, updatedModel.commitStage)
	assert.Equal(t, pushErr, updatedModel.err)
	// Note: Push failure doesn't cause immediate quit, it shows error for a duration
}

func TestCommitWorkflowModel_View(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.message = "test: sample commit message"
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCommitting

	// Test view rendering
	view := model.View()

	// Check that the view contains expected elements
	// The view includes the bordered UI with title
	assert.Contains(t, view, "Progress (en)") // Title includes language
	assert.Contains(t, view, "Committing changes...")
	assert.Contains(t, view, "Message:")
}

func TestCommitWorkflowModel_CalculateContentWidth(t *testing.T) {
	// Test minimum width
	assert.Equal(t, 60, CalculateContentWidth(50)) // Should use minimum

	// Test normal width
	assert.Equal(t, 96, CalculateContentWidth(100)) // 100 - 4 margin

	// Test maximum width
	assert.Equal(t, 120, CalculateContentWidth(150)) // Should use maximum
}

func TestCommitWorkflowModel_FinalTimeout(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageDone

	// Test final timeout
	newModel, cmd := model.Update(finalTimeoutMsg{})
	updatedModel := newModel.(*CommitWorkflowModel)

	assert.True(t, updatedModel.done)
	assert.NotNil(t, cmd) // Just check that quit command is returned
}

// Test the complete workflow: Loading → Review → Commit → Done

func TestCommitWorkflowModel_LoadingPhase(t *testing.T) {
	model, collector, promptBuilder, client, _ := createTestCommitWorkflowModel()
	
	// Setup mocks for loading phase
	collector.On("ComprehensiveDiff", mock.Anything).Return("diff content", nil)
	collector.On("AnalyzeChanges", mock.Anything).Return(&gitinfo.ChangesSummary{
		HasStagedChanges:  true,
		TotalFiles:        2,
		ChangeTypes:       map[string]int{"modified": 2},
		PrimaryChangeType: "feat",
	}, nil)
	promptBuilder.On("BuildSystemPrompt").Return("system prompt")
	promptBuilder.On("BuildUserPromptWithBudget", mock.Anything, mock.Anything, "test seed").Return("user prompt", nil)
	client.On("GetCommitMessage", mock.Anything, "system prompt", "user prompt").Return("test: generated commit message", nil)

	// Test Init
	cmd := model.Init()
	assert.NotNil(t, cmd)

	// Test diff collection
	newModel, _ := model.Update(diffCollectedMsg{diff: "diff content"})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, StagePreprocess, updatedModel.loadingStage)

	// Test preprocessing
	newModel2, _ := updatedModel.Update(preprocessDoneMsg{})
	updatedModel2 := newModel2.(*CommitWorkflowModel)
	assert.Equal(t, StagePrompt, updatedModel2.loadingStage)

	// Test prompt building
	newModel3, _ := updatedModel2.Update(smartPromptBuiltMsg{
		systemPrompt: "system prompt",
		userPrompt:   "user prompt",
	})
	updatedModel3 := newModel3.(*CommitWorkflowModel)
	assert.Equal(t, StageQuery, updatedModel3.loadingStage)

	// Test query done
	newModel4, _ := updatedModel3.Update(queryDoneMsg{message: "test: generated commit message"})
	finalModel := newModel4.(*CommitWorkflowModel)
	assert.Equal(t, WorkflowPhaseReview, finalModel.phase)
	assert.Equal(t, "test: generated commit message", finalModel.message)
}

func TestCommitWorkflowModel_ReviewPhase_Accept(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "test: commit message"
	model.createPR = false // Disable PR for this test

	// Simulate accept action
	cmd := model.handleAccept()
	assert.NotNil(t, cmd)

	// Should transition to commit phase
	newModel, _ := model.Update(startCommitPhaseMsg{})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, WorkflowPhaseCommit, updatedModel.phase)
	assert.Equal(t, CommitStageCommitting, updatedModel.commitStage)
}

func TestCommitWorkflowModel_ReviewPhase_Edit(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "test: original message"

	// Simulate edit action
	cmd := model.handleEdit()
	assert.NotNil(t, cmd)
	assert.True(t, model.editing)
	assert.True(t, model.textArea.Focused())

	// Simulate saving edit
	model.textArea.SetValue("test: edited message")
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.False(t, updatedModel.editing)
	assert.Equal(t, "test: edited message", updatedModel.message)

	// Test cancel edit
	model.editing = true
	newModel2, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updatedModel2 := newModel2.(*CommitWorkflowModel)
	assert.False(t, updatedModel2.editing)
}

func TestCommitWorkflowModel_ReviewPhase_Regenerate(t *testing.T) {
	model, _, promptBuilder, client, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "test: original message"

	// Setup mocks for regeneration
	promptBuilder.On("BuildSystemPrompt").Return("system prompt")
	promptBuilder.On("BuildUserPromptWithBudget", mock.Anything, mock.Anything, "test seed").Return("user prompt", nil)
	client.On("GetCommitMessage", mock.Anything, "system prompt", "user prompt").Return("test: regenerated message", nil)

	// Simulate regenerate action
	cmd := model.handleRegenerate()
	assert.NotNil(t, cmd)
	assert.Equal(t, WorkflowPhaseLoading, model.phase)
	assert.Equal(t, StagePrompt, model.loadingStage)
}

func TestCommitWorkflowModel_ReviewPhase_Cancel(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseReview

	// Simulate cancel action
	cmd := model.handleCancel()
	assert.NotNil(t, cmd)
	assert.Equal(t, DecisionCancel, model.reviewDecision)
	assert.True(t, model.done)
}

func TestCommitWorkflowModel_ErrorHandling(t *testing.T) {
	model, collector, _, _, _ := createTestCommitWorkflowModel()

	// Test error during diff collection
	collectErr := errors.New("failed to collect diff")
	collector.On("ComprehensiveDiff", mock.Anything).Return("", collectErr)

	// Simulate error message
	newModel, cmd := model.Update(errorMsg{err: collectErr})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, collectErr, updatedModel.err)
	assert.True(t, updatedModel.done)
	assert.NotNil(t, cmd)
}

func TestCommitWorkflowModel_StageAll(t *testing.T) {
	model, _, _, _, committer := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.stageAll = true
	model.message = "test: commit with stage all"

	// Setup mock expectations
	committer.On("StageAll", mock.Anything).Return(nil)
	committer.On("Commit", mock.Anything, "test: commit with stage all").Return(nil)

	// Execute commit command
	cmd := model.startCommit()
	msg := cmd() // Execute the command

	// Verify the result
	commitMsg, ok := msg.(commitDoneMsg)
	assert.True(t, ok)
	assert.Nil(t, commitMsg.err)

	// Verify mocks were called
	committer.AssertCalled(t, "StageAll", mock.Anything)
	committer.AssertCalled(t, "Commit", mock.Anything, "test: commit with stage all")
}

// Test PR preview functionality

func TestCommitWorkflowModel_PRPreview(t *testing.T) {
	model, collector, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "test: PR commit\n\nThis is the PR body"
	model.createPR = true
	model.prRemote = "origin"
	model.prBase = "main"
	model.prDraft = true
	model.prProvider = "github"

	// Setup mocks
	collector.On("BranchName", mock.Anything).Return("feature-branch", nil)
	collector.On("ChangedFiles", mock.Anything).Return([]string{"file1.go", "file2.go"}, nil)

	// Trigger PR preview preparation
	cmd := model.preparePRPreview()
	msg := cmd() // Execute the command

	// Verify PR preview data
	prMsg, ok := msg.(prPreviewReadyMsg)
	assert.True(t, ok)
	assert.Equal(t, "test: PR commit", prMsg.data.Title)
	assert.Equal(t, "This is the PR body", prMsg.data.Body)
	assert.Equal(t, "main", prMsg.data.Base)
	assert.Equal(t, "feature-branch", prMsg.data.Head)
	assert.Equal(t, "origin", prMsg.data.Remote)
	assert.Equal(t, "github", prMsg.data.Provider)
	assert.True(t, prMsg.data.IsDraft)
	assert.True(t, prMsg.data.HasChanges)
	assert.Equal(t, 2, len(prMsg.data.FileChanges))

	// Update model with PR preview data
	newModel, _ := model.Update(prMsg)
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, WorkflowPhasePRPreview, updatedModel.phase)
	assert.NotNil(t, updatedModel.prPreview)
}

func TestCommitWorkflowModel_PRPreview_Navigation(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhasePRPreview
	model.prPreviewData = PRPreviewData{
		Title:      "test: PR title",
		Body:       "PR body",
		Base:       "main",
		Head:       "feature",
		Provider:   "github",
		HasChanges: true,
	}
	model.prPreview = NewEnhancedPRPreviewModel(model.prPreviewData, DefaultStyles(), CalculateContentWidth(model.width), 24)

	// Test toggle details
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	assert.Equal(t, model, newModel) // Model reference shouldn't change

	// Test continue to commit
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.NotNil(t, cmd)

	// Simulate commit phase transition
	newModel3, _ := model.Update(startCommitPhaseMsg{})
	updatedModel := newModel3.(*CommitWorkflowModel)
	assert.Equal(t, WorkflowPhaseCommit, updatedModel.phase)
	assert.Equal(t, CommitStageCommitting, updatedModel.commitStage)

	// Test cancel
	model.phase = WorkflowPhasePRPreview
	model.updateActionsForPhase() // Ensure actions are set for the phase
	newModel4, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updatedModel2 := newModel4.(*CommitWorkflowModel)
	assert.Equal(t, DecisionCancel, updatedModel2.reviewDecision)
	assert.True(t, updatedModel2.done)
}

func TestCommitWorkflowModel_CreatePR_Success(t *testing.T) {
	model, _, _, _, committer := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCreatingPR
	model.createPR = true

	// Setup mock
	committer.On("CreatePullRequest", mock.Anything).Return("https://github.com/user/repo/pull/123", nil)

	// Execute PR creation
	cmd := model.startCreatePR()
	msg := cmd() // Execute the command

	// Verify result
	prMsg, ok := msg.(createPRDoneMsg)
	assert.True(t, ok)
	assert.Nil(t, prMsg.err)
	assert.Equal(t, "https://github.com/user/repo/pull/123", prMsg.prURL)

	// Update model with success
	newModel, _ := model.Update(prMsg)
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, CommitStagePRCreated, updatedModel.commitStage)
	assert.Equal(t, "https://github.com/user/repo/pull/123", updatedModel.prURL)
}

func TestCommitWorkflowModel_CreatePR_AlreadyExists(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCreatingPR
	model.createPR = true

	// Simulate PR already exists error
	prExistsErr := &pr.ErrPRAlreadyExists{
		URL: "https://github.com/user/repo/pull/122",
	}

	// Update model with PR exists error
	newModel, _ := model.Update(createPRDoneMsg{err: prExistsErr})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, CommitStagePRCreated, updatedModel.commitStage)
	assert.Equal(t, "https://github.com/user/repo/pull/122", updatedModel.prURL)
}

func TestCommitWorkflowModel_CreatePR_Failure(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCreatingPR
	model.createPR = true

	// Simulate PR creation error
	prErr := errors.New("failed to create PR")

	// Update model with error
	newModel, _ := model.Update(createPRDoneMsg{err: prErr})
	updatedModel := newModel.(*CommitWorkflowModel)
	assert.Equal(t, CommitStagePRFailed, updatedModel.commitStage)
	assert.Equal(t, prErr, updatedModel.err)
}

func TestCommitWorkflowModel_CompleteWorkflow_WithPR(t *testing.T) {
	model, _, _, _, committer := createTestCommitWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: new feature\n\nDetailed description"
	model.createPR = true
	model.enablePush = true

	// Step 1: Accept and go to PR preview
	cmd := model.handleAccept()
	assert.NotNil(t, cmd)

	// Step 2: Transition through PR preview
	model.phase = WorkflowPhasePRPreview
	model.prPreview = &EnhancedPRPreviewModel{} // Mock PR preview
	newModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Equal(t, model, newModel)

	// Step 3: Go to commit phase
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageCommitting

	// Setup mocks for complete flow
	committer.On("StageAll", mock.Anything).Return(nil)
	committer.On("Commit", mock.Anything, model.message).Return(nil)
	committer.On("Push", mock.Anything).Return(nil)
	committer.On("CreatePullRequest", mock.Anything).Return("https://github.com/user/repo/pull/124", nil)

	// Execute commit
	commitCmd := model.startCommit()
	commitMsg := commitCmd().(commitDoneMsg)
	assert.Nil(t, commitMsg.err)

	// Update with commit success
	newModel2, _ := model.Update(commitMsg)
	updatedModel := newModel2.(*CommitWorkflowModel)
	assert.Equal(t, CommitStageCommitted, updatedModel.commitStage)

	// Execute push
	newModel3, _ := updatedModel.Update(delayedPushMsg{})
	updatedModel2 := newModel3.(*CommitWorkflowModel)
	pushCmd := updatedModel2.startPush()
	pushMsg := pushCmd().(pushDoneMsg)
	assert.Nil(t, pushMsg.err)

	// Update with push success
	newModel4, _ := updatedModel2.Update(pushMsg)
	updatedModel3 := newModel4.(*CommitWorkflowModel)
	assert.Equal(t, CommitStagePushed, updatedModel3.commitStage)

	// Execute PR creation
	newModel5, _ := updatedModel3.Update(delayedCreatePRMsg{})
	updatedModel4 := newModel5.(*CommitWorkflowModel)
	prCmd := updatedModel4.startCreatePR()
	prMsg := prCmd().(createPRDoneMsg)
	assert.Nil(t, prMsg.err)
	assert.Equal(t, "https://github.com/user/repo/pull/124", prMsg.prURL)

	// Verify all mocks were called
	committer.AssertExpectations(t)
}
