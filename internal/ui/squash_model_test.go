package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/squash"
	"github.com/stretchr/testify/assert"
)

// Note: squashMsg is already defined in squash_model.go

func TestNewSquashModel(t *testing.T) {
	// Create a dummy squash instance (we can't easily mock the concrete type)
	var squashInstance *squash.Squash
	messages := []string{"feat: add feature", "fix: bug fix"}

	model := NewSquashModel(squashInstance, messages)

	assert.NotNil(t, model)
	assert.Equal(t, squashInstance, model.squash)
	assert.Equal(t, messages, model.messages)
	assert.Equal(t, SquashPhaseGenerating, model.phase)
	assert.False(t, model.accepted)
	assert.False(t, model.copySuccess)
	assert.Empty(t, model.result)
}

func TestSquashModel_Init(t *testing.T) {
	var squashInstance *squash.Squash
	messages := []string{"msg1", "msg2"}
	model := NewSquashModel(squashInstance, messages)

	cmd := model.Init()

	// Should return a command
	assert.NotNil(t, cmd)
}

func TestSquashModel_Update_Phases(t *testing.T) {
	tests := []struct {
		name           string
		phase          SquashPhase
		msg            tea.Msg
		expectedPhase  SquashPhase
		expectedAction string
		setupModel     func(*SquashModel)
	}{
		{
			name:          "generating phase with result",
			phase:         SquashPhaseGenerating,
			msg:           squashMsg{result: "feat: consolidated", err: nil},
			expectedPhase: SquashPhaseReviewing,
		},
		{
			name:          "generating phase with error",
			phase:         SquashPhaseGenerating,
			msg:           squashMsg{result: "", err: errors.New("API error")},
			expectedPhase: SquashPhaseReviewing,
		},
		{
			name:  "reviewing phase accept",
			phase: SquashPhaseReviewing,
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'a'},
			},
			expectedPhase: SquashPhaseDone,
			setupModel: func(m *SquashModel) {
				m.result = "test result"
			},
		},
		{
			name:  "reviewing phase regenerate",
			phase: SquashPhaseReviewing,
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'r'},
			},
			expectedPhase: SquashPhaseGenerating,
		},
		{
			name:  "reviewing phase quit",
			phase: SquashPhaseReviewing,
			msg: tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'q'},
			},
			expectedPhase: SquashPhaseReviewing, // Phase doesn't change, just returns quit command
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var squashInstance *squash.Squash
			messages := []string{"msg1", "msg2"}
			model := NewSquashModel(squashInstance, messages)
			model.phase = tt.phase

			if tt.setupModel != nil {
				tt.setupModel(model)
			}
			
			// Update actions for reviewing phase tests
			if tt.phase == SquashPhaseReviewing {
				model.updateActionsForPhase()
			}

			newModel, cmd := model.Update(tt.msg)
			updatedModel := newModel.(*SquashModel)

			assert.Equal(t, tt.expectedPhase, updatedModel.phase)

			// Check for quit command
			if tt.expectedPhase == SquashPhaseDone && tt.msg.(tea.KeyMsg).Runes[0] == 'q' {
				assert.NotNil(t, cmd)
			}
		})
	}
}

func TestSquashModel_View(t *testing.T) {
	tests := []struct {
		name          string
		phase         SquashPhase
		result        string
		hasError      bool
		setupModel    func(*SquashModel)
		viewContains  []string
	}{
		{
			name:         "generating phase",
			phase:        SquashPhaseGenerating,
			viewContains: []string{"Generating", "commit message"},
		},
		{
			name:         "reviewing phase with result",
			phase:        SquashPhaseReviewing,
			result:       "feat: test commit",
			viewContains: []string{"Generated commit message:", "feat: test commit", "[A]ccept", "[R]egenerate"},
		},
		{
			name:         "reviewing phase with error",
			phase:        SquashPhaseReviewing,
			hasError:     true,
			viewContains: []string{"Error", "[R]etry", "[Q]uit"},
		},
		{
			name:  "done phase accepted",
			phase: SquashPhaseDone,
			result: "feat: final commit",
			setupModel: func(m *SquashModel) {
				m.accepted = true
				m.copySuccess = true
			},
			viewContains: []string{}, // View returns empty string for done phase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var squashInstance *squash.Squash
			model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})
			model.phase = tt.phase
			model.result = tt.result
			
			if tt.hasError {
				model.SetError(errors.New("API error occurred"))
			}

			if tt.setupModel != nil {
				tt.setupModel(model)
			}
			
			// Update actions based on phase
			model.updateActionsForPhase()

			view := model.View()

			// Done phase returns empty view
			if tt.phase == SquashPhaseDone {
				assert.Empty(t, view)
			} else {
				for _, expected := range tt.viewContains {
					assert.Contains(t, view, expected)
				}
			}
		})
	}
}

func TestSquashModel_GettersAndState(t *testing.T) {
	var squashInstance *squash.Squash
	model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})

	// Test initial state
	assert.False(t, model.IsAccepted())
	assert.False(t, model.IsCopySuccess())
	assert.Empty(t, model.GetResult())

	// Update state
	model.accepted = true
	model.copySuccess = true
	model.result = "feat: test result"

	// Test updated state
	assert.True(t, model.IsAccepted())
	assert.True(t, model.IsCopySuccess())
	assert.Equal(t, "feat: test result", model.GetResult())
}

func TestSquashModel_AcceptAction(t *testing.T) {
	var squashInstance *squash.Squash
	model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})
	model.result = "test result"
	model.phase = SquashPhaseReviewing
	
	// Need to set up the actions first
	model.updateActionsForPhase()

	// Simulate accept action
	msg := tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'a'},
	}

	newModel, cmd := model.Update(msg)
	updatedModel := newModel.(*SquashModel)

	// The handler sets phase to Done and returns tea.Quit
	assert.Equal(t, SquashPhaseDone, updatedModel.phase)
	assert.True(t, updatedModel.accepted)
	assert.NotNil(t, cmd) // Should return tea.Quit command
}

func TestSquashModel_WindowSize(t *testing.T) {
	var squashInstance *squash.Squash
	model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})

	// Test window size update
	msg := tea.WindowSizeMsg{
		Width:  100,
		Height: 50,
	}

	newModel, _ := model.Update(msg)
	updatedModel := newModel.(*SquashModel)

	// The BaseModel should handle window size
	assert.Equal(t, 100, updatedModel.width)
	assert.Equal(t, 50, updatedModel.height)
}

func TestSquashModel_SpinnerUpdate(t *testing.T) {
	var squashInstance *squash.Squash
	model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})
	model.phase = SquashPhaseGenerating

	// Test spinner tick
	spinnerMsg := model.spinner.Tick()
	newModel, cmd := model.Update(spinnerMsg)
	
	assert.NotNil(t, newModel)
	assert.NotNil(t, cmd) // Should return another tick command
}

func TestSquashModel_CompleteWorkflow(t *testing.T) {
	var squashInstance *squash.Squash
	messages := []string{"feat: feature 1", "fix: bug fix", "docs: update readme"}
	model := NewSquashModel(squashInstance, messages)

	// 1. Start with generating
	assert.Equal(t, SquashPhaseGenerating, model.phase)

	// 2. Receive generated result
	generatedMsg := squashMsg{
		result: "feat: comprehensive update with bug fixes and documentation",
		err:    nil,
	}
	newModel, _ := model.Update(generatedMsg)
	model = newModel.(*SquashModel)
	assert.Equal(t, SquashPhaseReviewing, model.phase)
	assert.Equal(t, generatedMsg.result, model.result)

	// 3. Accept the result
	acceptKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	newModel, _ = model.Update(acceptKey)
	model = newModel.(*SquashModel)
	assert.Equal(t, SquashPhaseDone, model.phase)
	assert.True(t, model.accepted)
	// Note: We can't test copySuccess directly since it depends on clipboard.WriteAll
}

func TestSquashModel_ErrorHandling(t *testing.T) {
	var squashInstance *squash.Squash
	model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})
	model.phase = SquashPhaseGenerating

	// Receive error result
	errorMsg := squashMsg{
		result: "",
		err:    errors.New("API timeout"),
	}
	
	newModel, _ := model.Update(errorMsg)
	updatedModel := newModel.(*SquashModel)

	assert.Equal(t, SquashPhaseReviewing, updatedModel.phase)
	assert.NotNil(t, updatedModel.GetError())
	assert.Contains(t, updatedModel.GetError().Error(), "API timeout")
	assert.Empty(t, updatedModel.result)
}

func TestSquashModel_Actions(t *testing.T) {
	tests := []struct {
		name        string
		phase       SquashPhase
		hasError    bool
		viewContains []string
	}{
		{
			name:         "generating phase",
			phase:        SquashPhaseGenerating,
			viewContains: []string{}, // No actions should be shown
		},
		{
			name:         "reviewing phase no error",
			phase:        SquashPhaseReviewing,
			hasError:     false,
			viewContains: []string{"[A]", "ccept", "[R]", "egenerate", "[E]", "dit", "[Q]", "uit"},
		},
		{
			name:         "reviewing phase with error",
			phase:        SquashPhaseReviewing,
			hasError:     true,
			viewContains: []string{"[R]", "etry", "[Q]", "uit"},
		},
		{
			name:         "done phase",
			phase:        SquashPhaseDone,
			viewContains: []string{}, // No view in done phase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var squashInstance *squash.Squash
			model := NewSquashModel(squashInstance, []string{"msg1", "msg2"})
			model.phase = tt.phase
			model.result = "test result" // Set a result for non-error cases
			if tt.hasError {
				model.SetError(errors.New("some error"))
				model.result = "" // Clear result for error cases
			}
			
			// Update actions based on phase
			model.updateActionsForPhase()
			
			// Get the view which will include rendered actions
			view := model.View()
			
			// For done phase, view should be empty
			if tt.phase == SquashPhaseDone {
				assert.Empty(t, view)
			} else {
				// Check that expected action labels appear in the view
				for _, expected := range tt.viewContains {
					assert.Contains(t, view, expected)
				}
			}
		})
	}
}