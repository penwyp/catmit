package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// Test cancel behavior in different phases
func TestPRWorkflowModel_CancelBehavior(t *testing.T) {
	tests := []struct {
		name           string
		phase          WorkflowPhase
		keyMsg         string
		expectDone     bool
		expectDecision Decision
	}{
		// Review phase
		{
			name:           "Review phase - C key",
			phase:          WorkflowPhaseReview,
			keyMsg:         "c",
			expectDone:     true,
			expectDecision: DecisionCancel,
		},
		{
			name:           "Review phase - Esc key",
			phase:          WorkflowPhaseReview,
			keyMsg:         "esc",
			expectDone:     true,
			expectDecision: DecisionCancel,
		},
		{
			name:           "Review phase - Ctrl+C",
			phase:          WorkflowPhaseReview,
			keyMsg:         "ctrl+c",
			expectDone:     true,
			expectDecision: DecisionCancel,
		},
		{
			name:           "Review phase - Q key",
			phase:          WorkflowPhaseReview,
			keyMsg:         "q",
			expectDone:     true,
			expectDecision: DecisionNone, // Q just quits, doesn't set cancel decision
		},
		// PR Preview phase
		{
			name:           "PR Preview phase - C key",
			phase:          WorkflowPhasePRPreview,
			keyMsg:         "c",
			expectDone:     true,
			expectDecision: DecisionCancel,
		},
		{
			name:           "PR Preview phase - Esc key",
			phase:          WorkflowPhasePRPreview,
			keyMsg:         "esc",
			expectDone:     true,
			expectDecision: DecisionCancel,
		},
		{
			name:           "PR Preview phase - Ctrl+C",
			phase:          WorkflowPhasePRPreview,
			keyMsg:         "ctrl+c",
			expectDone:     true,
			expectDecision: DecisionCancel,
		},
		{
			name:           "PR Preview phase - Q key",
			phase:          WorkflowPhasePRPreview,
			keyMsg:         "q",
			expectDone:     true,
			expectDecision: DecisionNone, // Q just quits, doesn't set cancel decision
		},
		// Loading phase
		{
			name:           "Loading phase - Ctrl+C",
			phase:          WorkflowPhaseLoading,
			keyMsg:         "ctrl+c",
			expectDone:     true,
			expectDecision: DecisionNone, // No decision in loading phase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test model
			model, _, _, _, _, _ := createTestPRWorkflowModel()
			model.phase = tt.phase
			model.message = "feat: test PR"
			
			// Set up actions for phases that need them
			model.updateActionsForPhase()
			
			// For PR preview phase, set up the preview
			if tt.phase == WorkflowPhasePRPreview {
				model.prPreviewData = PRPreviewData{
					Title: "feat: test PR",
					Body:  "Test body",
				}
				model.prPreview = NewEnhancedPRPreviewModel(model.prPreviewData, DefaultStyles(), 80, 24)
			}
			
			// Create key message
			var keyMsg tea.KeyMsg
			switch tt.keyMsg {
			case "ctrl+c":
				keyMsg = tea.KeyMsg{Type: tea.KeyCtrlC}
			case "esc":
				keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
			default:
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.keyMsg)}
			}
			
			// Update model
			newModel, cmd := model.Update(keyMsg)
			updatedModel := newModel.(*PRWorkflowModel)
			
			// Verify expectations
			assert.Equal(t, tt.expectDone, updatedModel.done, "done flag mismatch")
			assert.Equal(t, tt.expectDecision, updatedModel.reviewDecision, "review decision mismatch")
			
			// Verify quit command is returned when expected
			if tt.expectDone {
				assert.NotNil(t, cmd, "expected quit command")
			}
			
			// Special case: verify context.Canceled is set for ctrl+c
			if tt.keyMsg == "ctrl+c" && tt.expectDone {
				assert.Equal(t, context.Canceled, updatedModel.err, "expected context.Canceled error")
			}
		})
	}
}

// Test that non-cancel keys don't trigger cancel
func TestPRWorkflowModel_NonCancelKeys(t *testing.T) {
	// Create test model
	model, _, _, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.message = "feat: test PR"
	model.updateActionsForPhase()
	
	// Test various non-cancel keys
	nonCancelKeys := []string{"a", "e", "r", "d", "space", "enter"}
	
	for _, key := range nonCancelKeys {
		t.Run("Key: "+key, func(t *testing.T) {
			// Reset model state
			model.done = false
			model.reviewDecision = DecisionNone
			
			var keyMsg tea.KeyMsg
			if key == "space" {
				keyMsg = tea.KeyMsg{Type: tea.KeySpace}
			} else if key == "enter" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEnter}
			} else {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
			}
			
			// Update model
			newModel, _ := model.Update(keyMsg)
			updatedModel := newModel.(*PRWorkflowModel)
			
			// Verify cancel was not triggered
			assert.False(t, updatedModel.done, "done flag should be false")
			assert.NotEqual(t, DecisionCancel, updatedModel.reviewDecision, "should not have cancel decision")
		})
	}
}

// Test cancel during editing mode
func TestPRWorkflowModel_CancelDuringEdit(t *testing.T) {
	// Create test model
	model, _, _, _, _, _ := createTestPRWorkflowModel()
	model.phase = WorkflowPhaseReview
	model.editing = true
	model.message = "feat: original message"
	model.textArea.SetValue("feat: edited message")
	model.updateActionsForPhase()
	
	// Press Esc to cancel edit
	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	newModel, _ := model.Update(keyMsg)
	updatedModel := newModel.(*PRWorkflowModel)
	
	// Verify edit was cancelled but not the whole operation
	assert.False(t, updatedModel.editing, "should exit editing mode")
	assert.False(t, updatedModel.done, "should not quit")
	assert.Equal(t, DecisionNone, updatedModel.reviewDecision, "should not have cancel decision")
	
	// Now test that C key cancels after exiting edit mode
	updatedModel.updateActionsForPhase() // Refresh actions after exiting edit
	keyMsg2 := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}
	newModel2, _ := updatedModel.Update(keyMsg2)
	finalModel := newModel2.(*PRWorkflowModel)
	
	assert.True(t, finalModel.done, "should quit")
	assert.Equal(t, DecisionCancel, finalModel.reviewDecision, "should have cancel decision")
}