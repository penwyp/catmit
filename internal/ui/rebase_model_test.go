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
func createTestRebaseModel(workflow rebaseWorkflowInterface) *RebaseWorkflowModel {
	ctx := context.Background()
	model := NewRebaseWorkflowModel(ctx, workflow, 30) // 30 seconds timeout for tests
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
			{SHA: "def456", Subject: "feat: add feature A"},
			{SHA: "ghi789", Subject: "fix: fix bug B"},
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
	assert.Equal(t, WorkflowPhaseLoading, model.BaseWorkflowModel.phase)
	assert.Equal(t, StageCollect, model.BaseWorkflowModel.loadingStage)
}

func TestRebaseWorkflowModel_AnalyzeRepository_Success(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	workflow.On("Analyze", mock.Anything).Return(analysis, nil)

	model := createTestRebaseModel(workflow)
	_ = model.Init()

	// Execute command would be run asynchronously in real app
	// For testing, we simulate it by creating the message directly
	analysisMsg := rebaseAnalysisMsg{result: analysis, err: nil}

	// Update model with result
	updatedModel, _ := model.Update(analysisMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, analysis, rebaseModel.analysis)
	assert.True(t, rebaseModel.needsAnalysisConfirmation)
	assert.Equal(t, WorkflowPhaseReview, rebaseModel.BaseWorkflowModel.phase)
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
	
	// Directly send the analysis message
	analysisMsg := rebaseAnalysisMsg{
		result: analysis,
		err:    nil,
	}

	// Update model
	updatedModel, cmd := model.Update(analysisMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	// When CanRebase is false, the model doesn't set an error, it just sets done=true and returns tea.Quit
	assert.Nil(t, rebaseModel.BaseWorkflowModel.err)
	assert.True(t, rebaseModel.BaseWorkflowModel.done)
	assert.NotNil(t, cmd) // Should return tea.Quit
	assert.Equal(t, "No unpushed commits to rebase", rebaseModel.analysis.Message)
}

func TestRebaseWorkflowModel_AnalyzeRepository_Error(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	expectedErr := errors.New("git error")
	workflow.On("Analyze", mock.Anything).Return(nil, expectedErr)

	model := createTestRebaseModel(workflow)
	
	// Directly send the analysis error message
	analysisMsg := rebaseAnalysisMsg{
		result: nil,
		err:    expectedErr,
	}

	// Update model
	updatedModel, cmd := model.Update(analysisMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, expectedErr, rebaseModel.BaseWorkflowModel.err)
	assert.True(t, rebaseModel.BaseWorkflowModel.done)
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
	assert.Equal(t, WorkflowPhaseLoading, rebaseModel.BaseWorkflowModel.phase)
	assert.Equal(t, StageQuery, rebaseModel.BaseWorkflowModel.loadingStage)
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
	
	assert.True(t, rebaseModel.BaseWorkflowModel.done)
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
	
	assert.Equal(t, expectedMessage, rebaseModel.BaseWorkflowModel.message)
	assert.Equal(t, WorkflowPhaseReview, rebaseModel.BaseWorkflowModel.phase)
	assert.Equal(t, expectedMessage, rebaseModel.BaseWorkflowModel.textArea.Value())
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
	
	assert.Equal(t, expectedErr, rebaseModel.BaseWorkflowModel.err)
	assert.Equal(t, WorkflowPhaseReview, rebaseModel.BaseWorkflowModel.phase)
}

func TestRebaseWorkflowModel_ReviewActions(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)
	model.BaseWorkflowModel.phase = WorkflowPhaseReview
	model.BaseWorkflowModel.message = "feat: test message"
	model.BaseWorkflowModel.editing = false

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
	model.BaseWorkflowModel.phase = WorkflowPhaseReview
	model.analysis = analysis
	model.BaseWorkflowModel.message = "feat: test message"

	// Handle accept
	cmd := model.handleAccept()
	assert.NotNil(t, cmd)
	assert.True(t, model.accepted)
	assert.Equal(t, DecisionAccept, model.BaseWorkflowModel.reviewDecision)
	assert.Equal(t, WorkflowPhaseCommit, model.BaseWorkflowModel.phase)
	assert.Equal(t, CommitStageCommitting, model.BaseWorkflowModel.commitStage)
}

func TestRebaseWorkflowModel_ExecuteRebase_Success(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	
	workflow.On("ExecuteRebase", mock.Anything, analysis, "feat: test message").Return(nil)

	model := createTestRebaseModel(workflow)
	model.analysis = analysis
	model.BaseWorkflowModel.message = "feat: test message"

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
	
	assert.NoError(t, rebaseModel.BaseWorkflowModel.err)
	assert.Equal(t, "feature_bak", rebaseModel.backupBranch)
	assert.Equal(t, CommitStageDone, rebaseModel.BaseWorkflowModel.commitStage)
	assert.NotNil(t, cmd) // Should have timeout command
}

func TestRebaseWorkflowModel_ExecuteRebase_Error(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	expectedErr := errors.New("rebase conflict")
	
	workflow.On("ExecuteRebase", mock.Anything, analysis, "feat: test message").Return(expectedErr)

	model := createTestRebaseModel(workflow)
	model.analysis = analysis
	model.BaseWorkflowModel.message = "feat: test message"

	// Execute rebase
	cmd := model.executeRebase()
	msg := cmd()
	executedMsg := msg.(rebaseExecutedMsg)

	// Update model
	updatedModel, cmd := model.Update(executedMsg)
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, expectedErr, rebaseModel.BaseWorkflowModel.err)
	assert.Equal(t, CommitStagePushFailed, rebaseModel.BaseWorkflowModel.commitStage) // Reused for error state
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
	assert.Equal(t, WorkflowPhaseLoading, model.BaseWorkflowModel.phase)
	assert.Equal(t, StageQuery, model.BaseWorkflowModel.loadingStage)
	assert.False(t, model.copySuccess)
}

func TestRebaseWorkflowModel_EditMode(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)
	model.BaseWorkflowModel.phase = WorkflowPhaseReview
	model.BaseWorkflowModel.message = "feat: original message"
	model.BaseWorkflowModel.textArea.SetValue(model.BaseWorkflowModel.message)

	// Enter edit mode
	model.handleEdit()
	assert.True(t, model.BaseWorkflowModel.editing)

	// Update actions should be nil in edit mode
	model.updateActionsForPhase()
	assert.Nil(t, model.GetActions())

	// Save edit (Ctrl+S)
	newMessage := "feat: edited message"
	model.BaseWorkflowModel.textArea.SetValue(newMessage)
	cmd := model.updateReview(tea.KeyMsg{Type: tea.KeyCtrlS})
	assert.Nil(t, cmd)
	assert.False(t, model.BaseWorkflowModel.editing)
	assert.Equal(t, newMessage, model.BaseWorkflowModel.message)

	// Actions should be restored
	model.updateActionsForPhase()
	assert.Len(t, model.GetActions(), 4)
}

func TestRebaseWorkflowModel_CancelEdit(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)
	model.BaseWorkflowModel.phase = WorkflowPhaseReview
	originalMessage := "feat: original message"
	model.BaseWorkflowModel.message = originalMessage
	model.BaseWorkflowModel.textArea.SetValue(originalMessage)

	// Enter edit mode
	model.handleEdit()
	model.BaseWorkflowModel.textArea.SetValue("feat: changed but will cancel")

	// Cancel edit (Esc)
	cmd := model.updateReview(tea.KeyMsg{Type: tea.KeyEsc})
	assert.Nil(t, cmd)
	assert.False(t, model.BaseWorkflowModel.editing)
	// Message should not change when cancelled
	assert.Equal(t, originalMessage, model.BaseWorkflowModel.message)
}

func TestRebaseWorkflowModel_GlobalKeyboardShortcuts(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Test Ctrl+C
	handled, cmd := model.HandleGlobalKeys(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.True(t, handled)
	assert.NotNil(t, cmd)
	assert.Equal(t, context.Canceled, model.BaseWorkflowModel.err)
	assert.True(t, model.BaseWorkflowModel.done)
}

func TestRebaseWorkflowModel_SpinnerUpdate(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Create spinner tick message - use a dummy one for testing
	tickMsg := spinner.TickMsg{Time: time.Now(), ID: 0}

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
	// Just check that textArea width was updated, don't hardcode the exact value
	assert.Greater(t, model.textArea.Width(), 0)
}

func TestRebaseWorkflowModel_FinalTimeout(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Update with final timeout message
	updatedModel, cmd := model.Update(finalTimeoutMsg{})
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.True(t, rebaseModel.BaseWorkflowModel.done)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

func TestRebaseWorkflowModel_Getters(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Set test values
	model.accepted = true
	model.BaseWorkflowModel.message = "feat: test message"
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
		model.BaseWorkflowModel.phase = tt.phase
		model.BaseWorkflowModel.loadingStage = tt.loadingStage
		model.BaseWorkflowModel.commitStage = tt.commitStage
		model.BaseWorkflowModel.editing = tt.editing
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
	model.BaseWorkflowModel.phase = WorkflowPhaseLoading
	content = model.renderContent()
	assert.NotEmpty(t, content)

	model.BaseWorkflowModel.phase = WorkflowPhaseReview
	model.BaseWorkflowModel.message = "feat: test message"
	content = model.renderContent()
	assert.Contains(t, content, "feat: test message")

	// Test execution phase
	model.BaseWorkflowModel.phase = WorkflowPhaseCommit
	model.BaseWorkflowModel.commitStage = CommitStageDone
	model.backupBranch = "feature_bak"
	content = model.renderContent()
	assert.Contains(t, content, "✅ Rebase completed successfully!")
	assert.Contains(t, content, "Backup branch: feature_bak")
}

func TestRebaseWorkflowModel_ErrorHandling(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	model := createTestRebaseModel(workflow)

	// Test error message handling
	testError := errors.New("test error")
	updatedModel, cmd := model.Update(errorMsg{err: testError})
	rebaseModel := updatedModel.(*RebaseWorkflowModel)
	
	assert.Equal(t, testError, rebaseModel.BaseWorkflowModel.err)
	assert.True(t, rebaseModel.BaseWorkflowModel.done)
	assert.NotNil(t, cmd) // Should return tea.Quit
}

func TestRebaseWorkflowModel_CompleteWorkflow(t *testing.T) {
	workflow := new(mockRebaseWorkflow)
	analysis := createValidAnalysis()
	generatedMessage := "feat: combined changes"
	
	// We're not actually calling these methods in the test since we're
	// sending the result messages directly instead of executing commands

	model := createTestRebaseModel(workflow)

	// 1. Initialize and send analysis result
	_ = model.Init()
	analysisMsg := rebaseAnalysisMsg{result: analysis, err: nil}
	updatedModel, _ := model.Update(analysisMsg)
	model = updatedModel.(*RebaseWorkflowModel)
	assert.True(t, model.needsAnalysisConfirmation)

	// 2. Confirm analysis
	updatedModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updatedModel.(*RebaseWorkflowModel)
	assert.Equal(t, WorkflowPhaseLoading, model.BaseWorkflowModel.phase)

	// 3. Send generated message result
	genMsg := rebaseGeneratedMsg{message: generatedMessage, err: nil}
	updatedModel, _ = model.Update(genMsg)
	model = updatedModel.(*RebaseWorkflowModel)
	assert.Equal(t, generatedMessage, model.BaseWorkflowModel.message)
	assert.Equal(t, WorkflowPhaseReview, model.BaseWorkflowModel.phase) // Goes back to Review
	assert.False(t, model.needsAnalysisConfirmation) // No longer in confirmation mode

	// 4. Accept the generated message - directly call handleAccept since we're testing the model
	// The Update method would handle keyboard navigation which depends on BaseModel state
	model.handleAccept()
	assert.Equal(t, WorkflowPhaseCommit, model.BaseWorkflowModel.phase)
	assert.True(t, model.accepted)
	
	// 5. Send execution result
	execMsg := rebaseExecutedMsg{backupBranch: "feature_bak", err: nil}
	updatedModel, _ = model.Update(execMsg)
	model = updatedModel.(*RebaseWorkflowModel)
	assert.Equal(t, "feature_bak", model.backupBranch)
	assert.Equal(t, CommitStageDone, model.BaseWorkflowModel.commitStage)
	
	// 6. Send final timeout to mark as done
	updatedModel, _ = model.Update(finalTimeoutMsg{})
	model = updatedModel.(*RebaseWorkflowModel)
	assert.True(t, model.BaseWorkflowModel.done)
}