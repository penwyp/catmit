package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/rebase"
	"github.com/penwyp/catmit/pkg/githistory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock for rebase.Workflow
type mockRebaseWorkflow struct {
	mock.Mock
}

func (m *mockRebaseWorkflow) Analyze(ctx context.Context) (*rebase.AnalysisResult, error) {
	args := m.Called(ctx)
	if result := args.Get(0); result != nil {
		return result.(*rebase.AnalysisResult), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRebaseWorkflow) GenerateCommitMessage(ctx context.Context, commits []githistory.Commit) (string, error) {
	args := m.Called(ctx, commits)
	return args.String(0), args.Error(1)
}

func (m *mockRebaseWorkflow) ExecuteRebase(ctx context.Context, analysis *rebase.AnalysisResult, message string) error {
	args := m.Called(ctx, analysis, message)
	return args.Error(0)
}

// Helper to create test model
func createTestRebaseModel(workflow *mockRebaseWorkflow) *RebaseWorkflowModel {
	ctx := context.Background()
	model := NewRebaseWorkflowModel(ctx, workflow)
	model.width = 80
	model.height = 24
	return model
}

// Helper to create a valid analysis result
func createValidAnalysis() *rebase.AnalysisResult {
	return &rebase.AnalysisResult{
		CurrentBranch: "feature",
		BaseBranch:    "main",
		MergeBase:     "abc123",
		UnpushedCommits: []githistory.Commit{
			{Hash: "def456", Subject: "feat: add feature A"},
			{Hash: "ghi789", Subject: "fix: fix bug B"},
		},
		HasChanges: true,
		CanRebase:  true,
		Message:    "Ready to rebase",
	}
}

func TestRebaseWorkflowModel_Init(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Test initialization
	cmd := model.Init()
	assert.NotNil(t, cmd)
	assert.Equal(t, WorkflowPhaseLoading, model.phase)
	assert.Equal(t, StageCollect, model.loadingStage)
}

func TestRebaseWorkflowModel_AnalyzeRepository_Success(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	workflow.On("Analyze", mock.Anything).Return(analysis, nil)

	model := createTestRebaseModel(workflow)
	cmd := model.Init()

	// Execute command (analyze repository)
	msg := cmd()
	analysisMsg, ok := msg.(rebaseAnalysisMsg)
	assert.True(t, ok)
	assert.NoError(t, analysisMsg.err)
	assert.Equal(t, analysis, analysisMsg.result)

	// Update model with result
	updatedModel, _ := model.Update(analysisMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, analysis, rebaseModel.analysis)
	assert.True(t, rebaseModel.needsAnalysisConfirmation)
	assert.Equal(t, WorkflowPhaseReview, rebaseModel.phase)
}

func TestRebaseWorkflowModel_AnalyzeRepository_CannotRebase(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := &rebase.AnalysisResult{
		CurrentBranch: "feature",
		BaseBranch:    "main",
		CanRebase:     false,
		Message:       "No unpushed commits to rebase",
	}
	workflow.On("Analyze", mock.Anything).Return(analysis, nil)

	model := createTestRebaseModel(workflow)
	cmd := model.Init()

	// Execute command
	msg := cmd()
	analysisMsg := msg.(rebaseAnalysisMsg)

	// Update model
	updatedModel, cmd := model.Update(analysisMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Error(t, rebaseModel.err)
	assert.Contains(t, rebaseModel.err.Error(), "No unpushed commits to rebase")
	assert.True(t, rebaseModel.done)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

func TestRebaseWorkflowModel_AnalyzeRepository_Error(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	expectedErr := errors.New("git error")
	workflow.On("Analyze", mock.Anything).Return(nil, expectedErr)

	model := createTestRebaseModel(workflow)
	cmd := model.Init()

	// Execute command
	msg := cmd()
	analysisMsg := msg.(rebaseAnalysisMsg)

	// Update model
	updatedModel, cmd := model.Update(analysisMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, expectedErr, rebaseModel.err)
	assert.True(t, rebaseModel.done)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

func TestRebaseWorkflowModel_ConfirmAnalysis(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	workflow.On("Analyze", mock.Anything).Return(analysis, nil)
	workflow.On("GenerateCommitMessage", mock.Anything, analysis.UnpushedCommits).Return("feat: combined changes", nil)

	model := createTestRebaseModel(workflow)
	
	// First analyze
	cmd := model.Init()
	msg := cmd()
	model.Update(msg)
	
	// Test confirmation - accept (y)
	model.needsAnalysisConfirmation = true
	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.False(t, rebaseModel.needsAnalysisConfirmation)
	assert.Equal(t, WorkflowPhaseLoading, rebaseModel.phase)
	assert.Equal(t, StageQuery, rebaseModel.loadingStage)
	assert.NotNil(t, cmd)
}

func TestRebaseWorkflowModel_ConfirmAnalysis_Reject(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	workflow.On("Analyze", mock.Anything).Return(analysis, nil)

	model := createTestRebaseModel(workflow)
	
	// First analyze
	cmd := model.Init()
	msg := cmd()
	model.Update(msg)
	
	// Test confirmation - reject (n)
	model.needsAnalysisConfirmation = true
	updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.True(t, rebaseModel.done)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

func TestRebaseWorkflowModel_GenerateMessage_Success(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	expectedMessage := "feat: combined feature A and bug fix B\n\n- Add feature A\n- Fix bug B"
	
	workflow.On("GenerateCommitMessage", mock.Anything, analysis.UnpushedCommits).Return(expectedMessage, nil)

	model := createTestRebaseModel(workflow)
	model.analysis = analysis
	
	// Generate message
	cmd := model.generateRebaseMessage()
	msg := cmd()
	generatedMsg, ok := msg.(rebaseGeneratedMsg)
	assert.True(t, ok)
	assert.NoError(t, generatedMsg.err)
	assert.Equal(t, expectedMessage, generatedMsg.message)

	// Update model
	updatedModel, _ := model.Update(generatedMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, expectedMessage, rebaseModel.message)
	assert.Equal(t, WorkflowPhaseReview, rebaseModel.phase)
	assert.Equal(t, expectedMessage, rebaseModel.textArea.Value())
	// Note: We can't test clipboard in unit tests, but copySuccess should be attempted
}

func TestRebaseWorkflowModel_GenerateMessage_Error(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	expectedErr := errors.New("API error")
	
	workflow.On("GenerateCommitMessage", mock.Anything, analysis.UnpushedCommits).Return("", expectedErr)

	model := createTestRebaseModel(workflow)
	model.analysis = analysis
	
	// Generate message
	cmd := model.generateRebaseMessage()
	msg := cmd()
	generatedMsg := msg.(rebaseGeneratedMsg)

	// Update model
	updatedModel, _ := model.Update(generatedMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, expectedErr, rebaseModel.err)
	assert.Equal(t, WorkflowPhaseReview, rebaseModel.phase)
}

func TestRebaseWorkflowModel_ReviewActions(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)
	model.phase = WorkflowPhaseReview
	model.message = "feat: test message"
	model.editing = false

	// Update actions for review phase
	model.updateActionsForPhase()
	
	actions := model.GetActions()
	assert.Len(t, actions, 4)
	assert.Equal(t, "A", actions[0].Key)
	assert.Equal(t, "ccept", actions[0].Label)
	assert.Equal(t, "E", actions[1].Key)
	assert.Equal(t, "dit", actions[1].Label)
	assert.Equal(t, "R", actions[2].Key)
	assert.Equal(t, "egenerate", actions[2].Label)
	assert.Equal(t, "C", actions[3].Key)
	assert.Equal(t, "ancel", actions[3].Label)
}

func TestRebaseWorkflowModel_HandleAccept(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	
	workflow.On("ExecuteRebase", mock.Anything, analysis, "feat: test message").Return(nil)

	model := createTestRebaseModel(workflow)
	model.phase = WorkflowPhaseReview
	model.analysis = analysis
	model.message = "feat: test message"

	// Handle accept
	cmd := model.handleAccept()
	assert.NotNil(t, cmd)
	assert.True(t, model.accepted)
	assert.Equal(t, DecisionAccept, model.reviewDecision)
	assert.Equal(t, WorkflowPhaseCommit, model.phase)
	assert.Equal(t, CommitStageCommitting, model.commitStage)
}

func TestRebaseWorkflowModel_ExecuteRebase_Success(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	
	workflow.On("ExecuteRebase", mock.Anything, analysis, "feat: test message").Return(nil)

	model := createTestRebaseModel(workflow)
	model.analysis = analysis
	model.message = "feat: test message"

	// Execute rebase
	cmd := model.executeRebase()
	msg := cmd()
	executedMsg, ok := msg.(rebaseExecutedMsg)
	assert.True(t, ok)
	assert.NoError(t, executedMsg.err)
	assert.Equal(t, "feature_bak", executedMsg.backupBranch)

	// Update model
	updatedModel, cmd := model.Update(executedMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.NoError(t, rebaseModel.err)
	assert.Equal(t, "feature_bak", rebaseModel.backupBranch)
	assert.Equal(t, CommitStageDone, rebaseModel.commitStage)
	assert.NotNil(t, cmd) // Should have timeout command
}

func TestRebaseWorkflowModel_ExecuteRebase_Error(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	expectedErr := errors.New("rebase conflict")
	
	workflow.On("ExecuteRebase", mock.Anything, analysis, "feat: test message").Return(expectedErr)

	model := createTestRebaseModel(workflow)
	model.analysis = analysis
	model.message = "feat: test message"

	// Execute rebase
	cmd := model.executeRebase()
	msg := cmd()
	executedMsg := msg.(rebaseExecutedMsg)

	// Update model
	updatedModel, cmd := model.Update(executedMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, expectedErr, rebaseModel.err)
	assert.Equal(t, CommitStagePushFailed, rebaseModel.commitStage) // Reused for error state
	assert.NotNil(t, cmd) // Should have timeout command
}

func TestRebaseWorkflowModel_HandleRegenerate(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	
	workflow.On("GenerateCommitMessage", mock.Anything, analysis.UnpushedCommits).Return("feat: regenerated message", nil)

	model := createTestRebaseModel(workflow)
	model.analysis = analysis
	model.copySuccess = true

	// Handle regenerate
	cmd := model.handleRegenerate()
	assert.NotNil(t, cmd)
	assert.Equal(t, WorkflowPhaseLoading, model.phase)
	assert.Equal(t, StageQuery, model.loadingStage)
	assert.False(t, model.copySuccess)
}

func TestRebaseWorkflowModel_EditMode(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)
	model.phase = WorkflowPhaseReview
	model.message = "feat: original message"
	model.textArea.SetValue(model.message)

	// Enter edit mode
	model.handleEdit()
	assert.True(t, model.editing)

	// Update actions should be nil in edit mode
	model.updateActionsForPhase()
	assert.Nil(t, model.GetActions())

	// Save edit (Ctrl+S)
	newMessage := "feat: edited message"
	model.textArea.SetValue(newMessage)
	cmd := model.updateReview(tea.KeyMsg{Type: tea.KeyString, String: "ctrl+s"})
	assert.Nil(t, cmd)
	assert.False(t, model.editing)
	assert.Equal(t, newMessage, model.message)

	// Actions should be restored
	model.updateActionsForPhase()
	assert.Len(t, model.GetActions(), 4)
}

func TestRebaseWorkflowModel_CancelEdit(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)
	model.phase = WorkflowPhaseReview
	originalMessage := "feat: original message"
	model.message = originalMessage
	model.textArea.SetValue(originalMessage)

	// Enter edit mode
	model.handleEdit()
	model.textArea.SetValue("feat: changed but will cancel")

	// Cancel edit (Esc)
	cmd := model.updateReview(tea.KeyMsg{Type: tea.KeyString, String: "esc"})
	assert.Nil(t, cmd)
	assert.False(t, model.editing)
	// Message should not change when cancelled
	assert.Equal(t, originalMessage, model.message)
}

func TestRebaseWorkflowModel_GlobalKeyboardShortcuts(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Test Ctrl+C
	handled, cmd := model.HandleGlobalKeys(tea.KeyMsg{Type: tea.KeyString, String: "ctrl+c"})
	assert.True(t, handled)
	assert.NotNil(t, cmd)
	assert.Equal(t, context.Canceled, model.err)
	assert.True(t, model.done)
}

func TestRebaseWorkflowModel_SpinnerUpdate(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Create spinner tick message
	spinnerMsg := model.spinner.Tick()
	tickMsg := spinnerMsg().(spinner.TickMsg)

	// Update spinner
	cmd := model.UpdateSpinner(tickMsg)
	assert.NotNil(t, cmd)
}

func TestRebaseWorkflowModel_WindowResize(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Resize window
	model.HandleWindowSize(tea.WindowSizeMsg{Width: 100, Height: 30})
	assert.Equal(t, 100, model.width)
	assert.Equal(t, 30, model.height)
	
	// TextArea should be resized accordingly
	expectedWidth := CalculateContentWidth(100) - 4
	assert.Equal(t, expectedWidth, model.textArea.Width())
}

func TestRebaseWorkflowModel_FinalTimeout(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Update with final timeout message
	updatedModel, cmd := model.Update(finalTimeoutMsg{})
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.True(t, rebaseModel.done)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

func TestRebaseWorkflowModel_Getters(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Set test values
	model.accepted = true
	model.message = "feat: test message"
	model.copySuccess = true
	model.backupBranch = "feature_bak"

	// Test getters
	assert.True(t, model.IsAccepted())
	assert.Equal(t, "feat: test message", model.GetResult())
	assert.True(t, model.IsCopySuccess())
	assert.Equal(t, "feature_bak", model.GetBackupBranch())
}

func TestRebaseWorkflowModel_PhaseTitles(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	tests := []struct {
		phase                     WorkflowPhase
		loadingStage              Stage
		commitStage               CommitStage
		editing                   bool
		needsAnalysisConfirmation bool
		expectedTitle             string
	}{
		{
			phase:                     WorkflowPhaseReview,
			needsAnalysisConfirmation: true,
			expectedTitle:             "📋 Commits to Squash",
		},
		{
			phase:        WorkflowPhaseLoading,
			loadingStage: StageCollect,
			expectedTitle: "🔍 Analyzing repository state...",
		},
		{
			phase:        WorkflowPhaseLoading,
			loadingStage: StageQuery,
			expectedTitle: "🤖 Generating commit message...",
		},
		{
			phase:         WorkflowPhaseReview,
			editing:       false,
			expectedTitle: "📝 Generated Commit Message",
		},
		{
			phase:         WorkflowPhaseReview,
			editing:       true,
			expectedTitle: "📝 Edit Message",
		},
		{
			phase:       WorkflowPhaseCommit,
			commitStage: CommitStageCommitting,
			expectedTitle: "🔄 Executing rebase...",
		},
		{
			phase:       WorkflowPhaseCommit,
			commitStage: CommitStageDone,
			expectedTitle: "✅ Rebase Complete",
		},
	}

	for _, tt := range tests {
		model.phase = tt.phase
		model.loadingStage = tt.loadingStage
		model.commitStage = tt.commitStage
		model.editing = tt.editing
		model.needsAnalysisConfirmation = tt.needsAnalysisConfirmation

		title := model.getPhaseTitle()
		assert.Equal(t, tt.expectedTitle, title, "Phase: %v, Stage: %v", tt.phase, tt.loadingStage)
	}
}

func TestRebaseWorkflowModel_RenderContent(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Test analysis confirmation rendering
	model.needsAnalysisConfirmation = true
	model.analysis = createValidAnalysis()
	content := model.renderContent()
	assert.Contains(t, content, "Branch: feature → main")
	assert.Contains(t, content, "Commits to squash: 2")
	assert.Contains(t, content, "Continue? (y/n):")

	// Test other phases
	model.needsAnalysisConfirmation = false
	model.phase = WorkflowPhaseLoading
	content = model.renderContent()
	assert.NotEmpty(t, content)

	model.phase = WorkflowPhaseReview
	model.message = "feat: test message"
	content = model.renderContent()
	assert.Contains(t, content, "feat: test message")

	// Test execution phase
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStageDone
	model.backupBranch = "feature_bak"
	content = model.renderContent()
	assert.Contains(t, content, "✅ Rebase completed successfully!")
	assert.Contains(t, content, "Backup branch: feature_bak")
}

func TestRebaseWorkflowModel_ErrorHandling(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Test error message handling
	errorMsg := errors.New("test error")
	updatedModel, cmd := model.Update(errorMsg{err: errorMsg})
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, errorMsg, rebaseModel.err)
	assert.True(t, rebaseModel.done)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

func TestRebaseWorkflowModel_CompleteWorkflow(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	generatedMessage := "feat: combined changes"
	
	// Setup all expected calls in order
	workflow.On("Analyze", mock.Anything).Return(analysis, nil).Once()
	workflow.On("GenerateCommitMessage", mock.Anything, analysis.UnpushedCommits).Return(generatedMessage, nil).Once()
	workflow.On("ExecuteRebase", mock.Anything, analysis, generatedMessage).Return(nil).Once()

	model := createTestRebaseModel(workflow)

	// 1. Initialize and analyze
	cmd := model.Init()
	msg := cmd()
	model, _ = model.Update(msg)

	// 2. Confirm analysis
	model, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	assert.NotNil(t, cmd)

	// 3. Generate message
	msg = cmd()
	model, _ = model.Update(msg)
	rebaseModel := model.(*RebaseWorkflowModel)
	assert.Equal(t, generatedMessage, rebaseModel.message)

	// 4. Accept and execute
	cmd = rebaseModel.handleAccept()
	msg = cmd()
	model, cmd = model.Update(msg)
	
	rebaseModel = model.(*RebaseWorkflowModel)
	assert.Equal(t, CommitStageDone, rebaseModel.commitStage)
	assert.Equal(t, "feature_bak", rebaseModel.backupBranch)
	assert.True(t, rebaseModel.accepted)

	workflow.AssertExpectations(t)
}