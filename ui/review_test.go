package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestNewReviewModel(t *testing.T) {
	msg := "feat: add new feature"
	model := NewReviewModel(msg, "en")
	
	require.Equal(t, msg, model.message)
	require.False(t, model.editing)
	require.False(t, model.done)
	require.Equal(t, DecisionNone, model.decision)
	require.Equal(t, msg, model.textInput.Value())
}

func TestReviewModel_Init(t *testing.T) {
	model := NewReviewModel("test", "en")
	cmd := model.Init()
	require.Nil(t, cmd)
}

func TestReviewModel_Update_AcceptKey(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test 'a' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(*ReviewModel)
	require.True(t, rm.done)
	require.Equal(t, DecisionAccept, rm.decision)
	require.NotNil(t, cmd) // Should return tea.Quit
}

func TestReviewModel_Update_AcceptKeyUppercase(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test 'A' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(*ReviewModel)
	require.True(t, rm.done)
	require.Equal(t, DecisionAccept, rm.decision)
	require.NotNil(t, cmd) // Should return tea.Quit
}

func TestReviewModel_Update_CancelKey(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test 'c' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(*ReviewModel)
	require.True(t, rm.done)
	require.Equal(t, DecisionCancel, rm.decision)
	require.NotNil(t, cmd) // Should return tea.Quit
}

func TestReviewModel_Update_CancelKeyVariants(t *testing.T) {
	testCases := []rune{'C', 'q', 'Q'}
	
	for _, key := range testCases {
		model := NewReviewModel("feat: test", "en")
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}}
		updatedModel, cmd := model.Update(keyMsg)
		
		rm := updatedModel.(*ReviewModel)
		require.True(t, rm.done, "Key %c should trigger cancel", key)
		require.Equal(t, DecisionCancel, rm.decision, "Key %c should set DecisionCancel", key)
		require.NotNil(t, cmd, "Key %c should return tea.Quit", key)
	}
}

func TestReviewModel_Update_EditKey(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test 'e' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(*ReviewModel)
	require.True(t, rm.editing)
	require.False(t, rm.done)
	require.Equal(t, DecisionNone, rm.decision)
	require.Nil(t, cmd) // Should not quit
}

func TestReviewModel_Update_EditKeyUppercase(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test 'E' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(*ReviewModel)
	require.True(t, rm.editing)
	require.False(t, rm.done)
	require.Equal(t, DecisionNone, rm.decision)
	require.Nil(t, cmd) // Should not quit
}

func TestReviewModel_Update_EditingMode(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	model.editing = true
	
	// Test enter key to save changes
	model.textInput.SetValue("feat: updated message")
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	updatedModel, _ := model.Update(enterKey)
	
	rm := updatedModel.(*ReviewModel)
	require.False(t, rm.editing)
	require.Equal(t, "feat: updated message", rm.message)
}

func TestReviewModel_Update_EditingModeEscape(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	model.editing = true
	originalMessage := model.message
	
	// Test escape key to cancel editing
	model.textInput.SetValue("feat: changed message")
	escKey := tea.KeyMsg{Type: tea.KeyEscape}
	updatedModel, _ := model.Update(escKey)
	
	rm := updatedModel.(*ReviewModel)
	require.False(t, rm.editing)
	require.Equal(t, originalMessage, rm.message) // Should remain unchanged
}

func TestReviewModel_Update_UnknownKey(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test unknown key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(*ReviewModel)
	require.False(t, rm.done)
	require.Equal(t, DecisionNone, rm.decision)
	require.Nil(t, cmd)
}

func TestReviewModel_Update_NonKeyMessage(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test non-key message
	updatedModel, cmd := model.Update("some string")
	
	rm := updatedModel.(*ReviewModel)
	require.False(t, rm.done)
	require.Equal(t, DecisionNone, rm.decision)
	require.Nil(t, cmd)
}

func TestReviewModel_View_Normal(t *testing.T) {
	model := NewReviewModel("feat: add new feature", "en")
	view := model.View()
	
	require.Contains(t, view, "Commit Preview")
	require.Contains(t, view, "feat: add new feature")
	require.Contains(t, view, "A")
	require.Contains(t, view, "Accept")
	require.Contains(t, view, "E")
	require.Contains(t, view, "Edit")
	require.Contains(t, view, "C")
	require.Contains(t, view, "Cancel")
}

func TestReviewModel_View_Editing(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	model.editing = true
	view := model.View()
	
	require.Contains(t, view, "Editing commit message")
	require.Contains(t, view, "enter to save")
	require.Contains(t, view, "esc to cancel")
}

func TestReviewModel_View_MultilineMessage(t *testing.T) {
	model := NewReviewModel("feat: add new feature\n\nThis is a detailed description", "en")
	view := model.View()
	
	require.Contains(t, view, "feat: add new feature")
	require.Contains(t, view, "This is a detailed description")
}

func TestReviewModel_IsDone(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Initially not done
	done, decision, message := model.IsDone()
	require.False(t, done)
	require.Equal(t, DecisionNone, decision)
	require.Equal(t, "feat: test", message)
	
	// After accept
	model.done = true
	model.decision = DecisionAccept
	model.message = "feat: updated"
	
	done, decision, message = model.IsDone()
	require.True(t, done)
	require.Equal(t, DecisionAccept, decision)
	require.Equal(t, "feat: updated", message)
}

func TestReviewModel_Decision_Constants(t *testing.T) {
	// Test that decision constants have expected values
	require.Equal(t, Decision(0), DecisionNone)
	require.Equal(t, Decision(1), DecisionAccept)
	require.Equal(t, Decision(2), DecisionCancel)
}

// Test that ReviewModel implements tea.Model interface
var _ tea.Model = (*ReviewModel)(nil)

// TestReviewModel_WindowSizeMsg tests terminal resize handling
func TestReviewModel_WindowSizeMsg(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test initial size
	require.Equal(t, 80, model.terminalWidth)
	require.Equal(t, 24, model.terminalHeight)
	
	// Test window resize
	windowMsg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, cmd := model.Update(windowMsg)
	
	rm := updatedModel.(*ReviewModel)
	require.Equal(t, 120, rm.terminalWidth)
	require.Equal(t, 40, rm.terminalHeight)
	require.Nil(t, cmd)
}

// TestReviewModel_CalculateContentWidth tests dynamic width calculation
func TestReviewModel_CalculateContentWidth(t *testing.T) {
	model := NewReviewModel("feat: test", "en")
	
	// Test minimum width constraint
	model.terminalWidth = 50 // Very narrow terminal
	width := model.calculateContentWidth()
	require.Equal(t, 60, width) // Should use minimum width
	
	// Test maximum width constraint
	model.terminalWidth = 200 // Very wide terminal
	width = model.calculateContentWidth()
	require.Equal(t, 120, width) // Should use maximum width
	
	// Test normal width
	model.terminalWidth = 100
	width = model.calculateContentWidth()
	require.Equal(t, 96, width) // 100 - 4 (margin)
}

// TestWordWrap tests enhanced text wrapping
func TestWordWrap(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			width:    10,
			expected: "",
		},
		{
			name:     "single word",
			input:    "hello",
			width:    10,
			expected: "hello",
		},
		{
			name:     "multiple words fitting in one line",
			input:    "hello world",
			width:    20,
			expected: "hello world",
		},
		{
			name:     "multiple words requiring wrap",
			input:    "hello world this is a test",
			width:    10,
			expected: "hello\nworld this\nis a test",
		},
		{
			name:     "paragraph with newlines",
			input:    "hello\n\nworld",
			width:    10,
			expected: "hello\n\nworld",
		},
		{
			name:     "zero width",
			input:    "hello world",
			width:    0,
			expected: "hello world",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wordWrap(tt.input, tt.width)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestTruncateContent tests content truncation
func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		expected string
	}{
		{
			name:     "content fits within limit",
			input:    "hello",
			maxWidth: 10,
			expected: "hello",
		},
		{
			name:     "content exceeds limit",
			input:    "hello world this is too long",
			maxWidth: 10,
			expected: "hello worl",
		},
		{
			name:     "zero width",
			input:    "hello",
			maxWidth: 0,
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			maxWidth: 10,
			expected: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateContent(tt.input, tt.maxWidth)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestReviewModel_View_Responsive tests responsive view rendering
func TestReviewModel_View_Responsive(t *testing.T) {
	model := NewReviewModel("feat: add responsive support", "en")
	
	// Test with narrow terminal
	model.terminalWidth = 70
	view := model.View()
	require.Contains(t, view, "feat: add responsive support")
	require.Contains(t, view, "Accept")
	
	// Test with wide terminal
	model.terminalWidth = 150
	view = model.View()
	require.Contains(t, view, "feat: add responsive support")
	require.Contains(t, view, "Accept")
	
	// Test with very narrow terminal
	model.terminalWidth = 40
	view = model.View()
	require.Contains(t, view, "feat:")
	require.Contains(t, view, "Accept")
}

// TestReviewModel_View_LongContent tests handling of long content
func TestReviewModel_View_LongContent(t *testing.T) {
	longMessage := "feat: add a very long feature description that should be wrapped properly when the terminal is narrow and the content exceeds the available width"
	model := NewReviewModel(longMessage, "en")
	model.terminalWidth = 80
	
	view := model.View()
	require.Contains(t, view, "feat:")
	require.Contains(t, view, "Accept")
	
	// Should not contain extremely long lines
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		// Allow some tolerance for styling characters and ANSI escape codes
		// The actual content width is constrained, but the view includes borders and styling
		require.LessOrEqual(t, len(line), 250, "Line too long: %s", line)
	}
}

// TestReviewModel_View_CJKSupport tests CJK character support
func TestReviewModel_View_CJKSupport(t *testing.T) {
	cjkMessage := "feat: 添加中文支持功能"
	model := NewReviewModel(cjkMessage, "zh")
	model.terminalWidth = 80
	
	view := model.View()
	require.Contains(t, view, "feat:")
	require.Contains(t, view, "添加中文支持功能")
	require.Contains(t, view, "Accept")
}