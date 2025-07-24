package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/rebase"
	"github.com/penwyp/catmit/pkg/githistory"
	"github.com/stretchr/testify/assert"
)

// Note: Message types analysisMsg, generatedMsg, and executedMsg are already defined in rebase_model.go

func TestNewRebaseModel(t *testing.T) {
	// Create a dummy workflow (we can't easily mock the concrete type)
	var workflow *rebase.Workflow
	model := NewRebaseModel(workflow)

	assert.NotNil(t, model)
	assert.Equal(t, workflow, model.workflow)
	assert.Equal(t, RebasePhaseAnalyzing, model.phase)
	assert.False(t, model.accepted)
	assert.Empty(t, model.backupBranch)
	assert.Nil(t, model.analysis)
	assert.Empty(t, model.message)
	assert.Nil(t, model.error)
}

func TestRebaseModel_Init(t *testing.T) {
	var workflow *rebase.Workflow
	model := NewRebaseModel(workflow)

	cmd := model.Init()

	// Should return a batch command with spinner tick and analyze command
	assert.NotNil(t, cmd)
}

func TestRebaseModel_Update_Phases(t *testing.T) {
	tests := []struct {
		name           string
		phase          RebasePhase
		msg            tea.Msg
		setupModel     func(*RebaseModel)
		expectedPhase  RebasePhase
		expectQuit     bool
	}{
		{
			name:  "analyzing phase with success",
			phase: RebasePhaseAnalyzing,
			msg: analysisMsg{
				result: &rebase.AnalysisResult{
					CanRebase:       true,
					UnpushedCommits: []githistory.Commit{{}, {}, {}},
					Message:         "Found 3 commits",
				},
				err: nil,
			},
			expectedPhase: RebasePhaseReviewing,
		},
		{
			name:  "analyzing phase with error",
			phase: RebasePhaseAnalyzing,
			msg: analysisMsg{
				result: nil,
				err:    errors.New("analysis failed"),
			},
			expectedPhase: RebasePhaseError,
		},
		{
			name:  "analyzing phase cannot rebase",
			phase: RebasePhaseAnalyzing,
			msg: analysisMsg{
				result: &rebase.AnalysisResult{
					CanRebase: false,
					Message:   "No commits to rebase",
				},
				err: nil,
			},
			expectedPhase: RebasePhaseDone,
		},
		{
			name:  "reviewing phase continue",
			phase: RebasePhaseReviewing,
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'y'},
			},
			setupModel: func(m *RebaseModel) {
				m.analysis = &rebase.AnalysisResult{
					UnpushedCommits: []githistory.Commit{{}, {}},
				}
			},
			expectedPhase: RebasePhaseGenerating,
		},
		{
			name:  "reviewing phase cancel",
			phase: RebasePhaseReviewing,
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'q'},
			},
			expectedPhase: RebasePhaseReviewing,
			expectQuit:    true,
		},
		{
			name:  "generating phase with success",
			phase: RebasePhaseGenerating,
			msg: generatedMsg{
				message: "feat: consolidated commit",
				err:     nil,
			},
			expectedPhase: RebasePhaseConfirming,
		},
		{
			name:  "generating phase with error",
			phase: RebasePhaseGenerating,
			msg: generatedMsg{
				message: "",
				err:     errors.New("generation failed"),
			},
			expectedPhase: RebasePhaseError,
		},
		{
			name:  "confirming phase accept",
			phase: RebasePhaseConfirming,
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'a'},
			},
			setupModel: func(m *RebaseModel) {
				m.analysis = &rebase.AnalysisResult{}
				m.result = "test message"
			},
			expectedPhase: RebasePhaseExecuting,
		},
		{
			name:  "confirming phase regenerate",
			phase: RebasePhaseConfirming,
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'r'},
			},
			setupModel: func(m *RebaseModel) {
				m.analysis = &rebase.AnalysisResult{
					UnpushedCommits: []githistory.Commit{{}, {}},
				}
			},
			expectedPhase: RebasePhaseGenerating,
		},
		{
			name:  "executing phase success",
			phase: RebasePhaseExecuting,
			msg: executedMsg{
				backupBranch: "backup-feature-123",
				err:          nil,
			},
			expectedPhase: RebasePhaseDone,
		},
		{
			name:  "executing phase error",
			phase: RebasePhaseExecuting,
			msg: executedMsg{
				backupBranch: "",
				err:          errors.New("rebase failed"),
			},
			expectedPhase: RebasePhaseError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var workflow *rebase.Workflow
			model := NewRebaseModel(workflow)
			model.phase = tt.phase

			if tt.setupModel != nil {
				tt.setupModel(model)
			}

			newModel, cmd := model.Update(tt.msg)
			updatedModel := newModel.(*RebaseModel)

			assert.Equal(t, tt.expectedPhase, updatedModel.phase)

			if tt.expectQuit {
				assert.NotNil(t, cmd)
			}
		})
	}
}

func TestRebaseModel_View(t *testing.T) {
	tests := []struct {
		name         string
		phase        RebasePhase
		setupModel   func(*RebaseModel)
		viewContains []string
	}{
		{
			name:         "analyzing phase",
			phase:        RebasePhaseAnalyzing,
			viewContains: []string{"Analyzing repository"},
		},
		{
			name:  "reviewing phase",
			phase: RebasePhaseReviewing,
			setupModel: func(m *RebaseModel) {
				m.analysis = &rebase.AnalysisResult{
					CurrentBranch:   "feature",
					BaseBranch:      "main",
					UnpushedCommits: []githistory.Commit{
						{ShortSHA: "abc123", Subject: "feat: add feature"},
						{ShortSHA: "def456", Subject: "fix: bug fix"},
					},
					Message: "Found 2 commits",
				}
			},
			viewContains: []string{"Commits to Squash", "feature", "main", "Commits to squash: 2", "Continue? (y/n):"},
		},
		{
			name:         "generating phase",
			phase:        RebasePhaseGenerating,
			viewContains: []string{"Generating", "commit message"},
		},
		{
			name:  "confirming phase",
			phase: RebasePhaseConfirming,
			setupModel: func(m *RebaseModel) {
				m.result = "feat: consolidated feature"
			},
			viewContains: []string{"Generated Commit Message", "feat: consolidated feature", "[A]ccept", "[R]egenerate"},
		},
		{
			name:         "executing phase",
			phase:        RebasePhaseExecuting,
			viewContains: []string{"Executing rebase"},
		},
		{
			name:  "done phase with acceptance",
			phase: RebasePhaseDone,
			setupModel: func(m *RebaseModel) {
				m.accepted = true
				m.backupBranch = "backup-feature-123"
			},
			viewContains: []string{"✅", "Rebase completed successfully", "backup-feature-123"},
		},
		{
			name:  "error phase",
			phase: RebasePhaseError,
			setupModel: func(m *RebaseModel) {
				m.error = errors.New("Failed to analyze repository")
			},
			viewContains: []string{"Error", "Failed to analyze repository", "Press any key to exit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var workflow *rebase.Workflow
			model := NewRebaseModel(workflow)
			model.phase = tt.phase

			if tt.setupModel != nil {
				tt.setupModel(model)
			}

			view := model.View()

			for _, expected := range tt.viewContains {
				assert.Contains(t, view, expected)
			}
		})
	}
}

func TestRebaseModel_GettersAndState(t *testing.T) {
	var workflow *rebase.Workflow
	model := NewRebaseModel(workflow)

	// Test initial state
	assert.False(t, model.IsAccepted())
	assert.Empty(t, model.GetBackupBranch())

	// Update state
	model.accepted = true
	model.backupBranch = "backup-feature-123"

	// Test updated state
	assert.True(t, model.IsAccepted())
	assert.Equal(t, "backup-feature-123", model.GetBackupBranch())
}

func TestRebaseModel_WindowSize(t *testing.T) {
	var workflow *rebase.Workflow
	model := NewRebaseModel(workflow)

	// Test window size update
	msg := tea.WindowSizeMsg{
		Width:  120,
		Height: 60,
	}

	newModel, _ := model.Update(msg)
	updatedModel := newModel.(*RebaseModel)

	// The BaseModel should handle window size
	assert.Equal(t, 120, updatedModel.width)
	assert.Equal(t, 60, updatedModel.height)
}

func TestRebaseModel_SpinnerUpdate(t *testing.T) {
	var workflow *rebase.Workflow
	model := NewRebaseModel(workflow)
	model.phase = RebasePhaseAnalyzing

	// Test spinner tick
	spinnerMsg := model.spinner.Tick()
	newModel, cmd := model.Update(spinnerMsg)
	
	assert.NotNil(t, newModel)
	assert.NotNil(t, cmd) // Should return another tick command
}

func TestRebaseModel_CompleteWorkflow(t *testing.T) {
	// Test a complete workflow from analysis to execution
	var workflow *rebase.Workflow
	model := NewRebaseModel(workflow)

	// 1. Start with analyzing
	assert.Equal(t, RebasePhaseAnalyzing, model.phase)

	// 2. Receive analysis result
	analysis := &rebase.AnalysisResult{
		CurrentBranch:   "feature",
		BaseBranch:      "main",
		CanRebase:       true,
		UnpushedCommits: []githistory.Commit{
			{ShortSHA: "abc123", Subject: "feat: add feature"},
			{ShortSHA: "def456", Subject: "fix: bug fix"},
		},
		Message: "Found 2 commits that can be squashed.",
	}
	
	newModel, _ := model.Update(analysisMsg{result: analysis, err: nil})
	model = newModel.(*RebaseModel)
	assert.Equal(t, RebasePhaseReviewing, model.phase)
	assert.Equal(t, analysis, model.analysis)

	// 3. Continue to generate message
	continueKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}
	newModel, _ = model.Update(continueKey)
	model = newModel.(*RebaseModel)
	assert.Equal(t, RebasePhaseGenerating, model.phase)

	// 4. Receive generated message
	message := "feat: comprehensive feature update with bug fixes"
	newModel, _ = model.Update(generatedMsg{message: message, err: nil})
	model = newModel.(*RebaseModel)
	assert.Equal(t, RebasePhaseConfirming, model.phase)
	assert.Equal(t, message, model.result)

	// 5. Accept and execute
	acceptKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	newModel, _ = model.Update(acceptKey)
	model = newModel.(*RebaseModel)
	assert.Equal(t, RebasePhaseExecuting, model.phase)

	// 6. Execution completes
	backupBranch := "backup-feature-20240123"
	newModel, _ = model.Update(executedMsg{backupBranch: backupBranch, err: nil})
	model = newModel.(*RebaseModel)
	assert.Equal(t, RebasePhaseDone, model.phase)
	assert.True(t, model.accepted)
	assert.Equal(t, backupBranch, model.backupBranch)
}

func TestRebaseModel_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		phase       RebasePhase
		errorMsg    tea.Msg
		expectedErr string
	}{
		{
			name:        "analysis error",
			phase:       RebasePhaseAnalyzing,
			errorMsg:    analysisMsg{nil, errors.New("not a git repository")},
			expectedErr: "not a git repository",
		},
		{
			name:        "generation error",
			phase:       RebasePhaseGenerating,
			errorMsg:    generatedMsg{"", errors.New("API timeout")},
			expectedErr: "API timeout",
		},
		{
			name:        "execution error",
			phase:       RebasePhaseExecuting,
			errorMsg:    executedMsg{"", errors.New("rebase conflict")},
			expectedErr: "rebase conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var workflow *rebase.Workflow
			model := NewRebaseModel(workflow)
			model.phase = tt.phase

			newModel, _ := model.Update(tt.errorMsg)
			updatedModel := newModel.(*RebaseModel)

			assert.Equal(t, RebasePhaseError, updatedModel.phase)
			assert.Contains(t, updatedModel.error.Error(), tt.expectedErr)
		})
	}
}