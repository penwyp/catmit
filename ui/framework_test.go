package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestBaseModel_NewBaseModel(t *testing.T) {
	actions := []Action{
		{Key: "A", Label: "Accept", Handler: func() tea.Cmd { return nil }},
		{Key: "C", Label: "Cancel", Handler: func() tea.Cmd { return tea.Quit }},
	}

	model := NewBaseModel("Test Title", actions, false)

	assert.Equal(t, "Test Title", model.title)
	assert.Equal(t, 2, len(model.actions))
	assert.Equal(t, 0, model.selected)
	assert.False(t, model.appendMode)
	assert.False(t, model.done)
	assert.Nil(t, model.err)
}

func TestBaseModel_HandleKeyboard(t *testing.T) {
	quitCalled := false
	acceptCalled := false

	actions := []Action{
		{Key: "A", Label: "Accept", Handler: func() tea.Cmd { acceptCalled = true; return nil }},
		{Key: "E", Label: "Edit", Handler: func() tea.Cmd { return nil }},
		{Key: "C", Label: "Cancel", Handler: func() tea.Cmd { quitCalled = true; return tea.Quit }},
	}

	tests := []struct {
		name           string
		key            string
		expectedSelect int
		expectedDone   bool
		checkHandler   func() bool
	}{
		{"Esc quits", "esc", 0, true, nil},
		{"Ctrl+C quits", "ctrl+c", 0, true, nil},
		{"Q quits", "q", 0, true, nil},
		{"Right arrow moves right", "right", 1, false, nil},
		{"Down arrow moves down", "down", 1, false, nil},
		{"Left arrow moves left", "left", 2, false, nil}, // wraps around
		{"Up arrow moves up", "up", 2, false, nil},       // wraps around
		{"Enter triggers action", "enter", 0, false, func() bool { return acceptCalled }},
		{"Letter key triggers action", "a", 0, false, func() bool { return acceptCalled }},
		{"Letter key triggers action (uppercase)", "C", 2, false, func() bool { return quitCalled }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			quitCalled = false
			acceptCalled = false

			model := NewBaseModel("Test", actions, false)
			cmd := model.HandleKeyboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key), Alt: false})

			assert.Equal(t, tt.expectedDone, model.done)
			assert.Equal(t, tt.expectedSelect, model.selected)

			if tt.checkHandler != nil {
				assert.True(t, tt.checkHandler())
			}

			// Check if quit command was returned when expected
			if tt.expectedDone {
				assert.NotNil(t, cmd)
			}
		})
	}
}

func TestBaseModel_NavigationWrapping(t *testing.T) {
	actions := []Action{
		{Key: "1", Label: "One", Handler: func() tea.Cmd { return nil }},
		{Key: "2", Label: "Two", Handler: func() tea.Cmd { return nil }},
		{Key: "3", Label: "Three", Handler: func() tea.Cmd { return nil }},
	}

	model := NewBaseModel("Test", actions, false)

	// Test wrapping from beginning to end
	model.selected = 0
	model.HandleKeyboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("left")})
	assert.Equal(t, 2, model.selected, "Should wrap from first to last")

	// Test wrapping from end to beginning
	model.selected = 2
	model.HandleKeyboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("right")})
	assert.Equal(t, 0, model.selected, "Should wrap from last to first")
}

func TestBaseModel_HandleWindowSize(t *testing.T) {
	model := NewBaseModel("Test", nil, false)
	
	model.HandleWindowSize(tea.WindowSizeMsg{Width: 120, Height: 40})
	
	assert.Equal(t, 120, model.width)
	assert.Equal(t, 40, model.height)
}

func TestBaseModel_RenderActions(t *testing.T) {
	actions := []Action{
		{Key: "A", Label: "ccept", Handler: func() tea.Cmd { return nil }},
		{Key: "E", Label: "dit", Handler: func() tea.Cmd { return nil }},
		{Key: "C", Label: "ancel", Handler: func() tea.Cmd { return nil }},
	}

	model := NewBaseModel("Test", actions, false)
	
	// Test with first action selected
	model.selected = 0
	output := model.RenderActions()
	assert.Contains(t, output, "[A]ccept")
	assert.Contains(t, output, "[E]dit")
	assert.Contains(t, output, "[C]ancel")

	// Test with middle action selected
	model.selected = 1
	output = model.RenderActions()
	assert.Contains(t, output, "[E]dit")
}

func TestBaseModel_StandardView(t *testing.T) {
	actions := []Action{
		{Key: "A", Label: "ccept", Handler: func() tea.Cmd { return nil }},
	}

	model := NewBaseModel("Test Title", actions, false)
	model.SetContentRenderer(func() string {
		return "This is the main content"
	})

	view := model.View()
	
	assert.Contains(t, view, "Test Title")
	assert.Contains(t, view, "This is the main content")
	assert.Contains(t, view, "[A]ccept")
}

func TestBaseModel_ErrorDisplay(t *testing.T) {
	model := NewBaseModel("Test", nil, false)
	model.SetError(assert.AnError)

	view := model.View()
	assert.Contains(t, view, "Error:")
	assert.Contains(t, view, assert.AnError.Error())
}

func TestBaseModel_AppendMode(t *testing.T) {
	model := NewBaseModel("Test", nil, true)
	model.SetContentRenderer(func() string {
		return "Content line 1"
	})

	// First render
	view1 := model.View()
	assert.Contains(t, view1, "Content line 1")
	assert.Greater(t, len(model.renderedLines), 0)

	// Change content
	model.SetContentRenderer(func() string {
		return "Content line 2"
	})

	// Second render should append
	view2 := model.View()
	assert.Contains(t, view2, "─") // separator
	assert.Contains(t, view2, "Content line 2")
}

func TestBaseModel_SettersAndGetters(t *testing.T) {
	model := NewBaseModel("Initial", nil, false)

	// Test SetTitle
	model.SetTitle("New Title")
	assert.Equal(t, "New Title", model.title)

	// Test SetActions
	newActions := []Action{
		{Key: "X", Label: "Execute", Handler: func() tea.Cmd { return nil }},
	}
	model.SetActions(newActions)
	assert.Equal(t, 1, len(model.actions))
	assert.Equal(t, "X", model.actions[0].Key)

	// Test GetSelected
	assert.Equal(t, 0, model.GetSelected())

	// Test IsDone
	assert.False(t, model.IsDone())
	model.done = true
	assert.True(t, model.IsDone())

	// Test GetError
	assert.Nil(t, model.GetError())
	model.SetError(assert.AnError)
	assert.Equal(t, assert.AnError, model.GetError())
}

func TestBaseModel_EmptyActions(t *testing.T) {
	model := NewBaseModel("Test", []Action{}, false)
	
	// Should not panic with empty actions
	cmd := model.HandleKeyboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	assert.Nil(t, cmd)
	
	output := model.RenderActions()
	assert.Empty(t, output)
}

func TestBaseModel_ContentRenderer(t *testing.T) {
	model := NewBaseModel("Test", nil, false)
	
	// No content renderer set
	view := model.View()
	assert.Contains(t, view, "Test")
	
	// Set content renderer
	model.SetContentRenderer(func() string {
		return "Dynamic content"
	})
	
	view = model.View()
	assert.Contains(t, view, "Dynamic content")
}

func TestBaseModel_SelectionBounds(t *testing.T) {
	actions := []Action{
		{Key: "1", Label: "One", Handler: func() tea.Cmd { return nil }},
		{Key: "2", Label: "Two", Handler: func() tea.Cmd { return nil }},
	}
	
	model := NewBaseModel("Test", actions, false)
	
	// Test selection stays in bounds when actions change
	model.selected = 1
	model.SetActions([]Action{{Key: "X", Label: "Only", Handler: func() tea.Cmd { return nil }}})
	assert.Equal(t, 0, model.selected, "Selection should adjust when actions shrink")
	
	// Test with empty actions
	model.SetActions([]Action{})
	assert.Equal(t, -1, model.selected, "Selection should be -1 with no actions")
}