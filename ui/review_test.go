package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestNewReviewModel(t *testing.T) {
	msg := "feat: add new feature"
	model := NewReviewModel(msg)
	
	require.Equal(t, msg, model.message)
	require.False(t, model.editing)
	require.False(t, model.done)
	require.Equal(t, DecisionNone, model.decision)
	require.Equal(t, msg, model.textInput.Value())
}

func TestReviewModel_Init(t *testing.T) {
	model := NewReviewModel("test")
	cmd := model.Init()
	require.Nil(t, cmd)
}

func TestReviewModel_Update_AcceptKey(t *testing.T) {
	model := NewReviewModel("feat: test")
	
	// Test 'a' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(ReviewModel)
	require.True(t, rm.done)
	require.Equal(t, DecisionAccept, rm.decision)
	require.NotNil(t, cmd) // Should return tea.Quit
}

func TestReviewModel_Update_AcceptKeyUppercase(t *testing.T) {
	model := NewReviewModel("feat: test")
	
	// Test 'A' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(ReviewModel)
	require.True(t, rm.done)
	require.Equal(t, DecisionAccept, rm.decision)
	require.NotNil(t, cmd) // Should return tea.Quit
}

func TestReviewModel_Update_CancelKey(t *testing.T) {
	model := NewReviewModel("feat: test")
	
	// Test 'c' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(ReviewModel)
	require.True(t, rm.done)
	require.Equal(t, DecisionCancel, rm.decision)
	require.NotNil(t, cmd) // Should return tea.Quit
}

func TestReviewModel_Update_CancelKeyVariants(t *testing.T) {
	testCases := []rune{'C', 'q', 'Q'}
	
	for _, key := range testCases {
		model := NewReviewModel("feat: test")
		keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}}
		updatedModel, cmd := model.Update(keyMsg)
		
		rm := updatedModel.(ReviewModel)
		require.True(t, rm.done, "Key %c should trigger cancel", key)
		require.Equal(t, DecisionCancel, rm.decision, "Key %c should set DecisionCancel", key)
		require.NotNil(t, cmd, "Key %c should return tea.Quit", key)
	}
}

func TestReviewModel_Update_EditKey(t *testing.T) {
	model := NewReviewModel("feat: test")
	
	// Test 'e' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(ReviewModel)
	require.True(t, rm.editing)
	require.False(t, rm.done)
	require.Equal(t, DecisionNone, rm.decision)
	require.Nil(t, cmd) // Should not quit
}

func TestReviewModel_Update_EditKeyUppercase(t *testing.T) {
	model := NewReviewModel("feat: test")
	
	// Test 'E' key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(ReviewModel)
	require.True(t, rm.editing)
	require.False(t, rm.done)
	require.Equal(t, DecisionNone, rm.decision)
	require.Nil(t, cmd) // Should not quit
}

func TestReviewModel_Update_EditingMode(t *testing.T) {
	model := NewReviewModel("feat: test")
	model.editing = true
	
	// Test enter key to save changes
	model.textInput.SetValue("feat: updated message")
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	updatedModel, _ := model.Update(enterKey)
	
	rm := updatedModel.(ReviewModel)
	require.False(t, rm.editing)
	require.Equal(t, "feat: updated message", rm.message)
}

func TestReviewModel_Update_EditingModeEscape(t *testing.T) {
	model := NewReviewModel("feat: test")
	model.editing = true
	originalMessage := model.message
	
	// Test escape key to cancel editing
	model.textInput.SetValue("feat: changed message")
	escKey := tea.KeyMsg{Type: tea.KeyEscape}
	updatedModel, _ := model.Update(escKey)
	
	rm := updatedModel.(ReviewModel)
	require.False(t, rm.editing)
	require.Equal(t, originalMessage, rm.message) // Should remain unchanged
}

func TestReviewModel_Update_UnknownKey(t *testing.T) {
	model := NewReviewModel("feat: test")
	
	// Test unknown key
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	updatedModel, cmd := model.Update(keyMsg)
	
	rm := updatedModel.(ReviewModel)
	require.False(t, rm.done)
	require.Equal(t, DecisionNone, rm.decision)
	require.Nil(t, cmd)
}

func TestReviewModel_Update_NonKeyMessage(t *testing.T) {
	model := NewReviewModel("feat: test")
	
	// Test non-key message
	updatedModel, cmd := model.Update("some string")
	
	rm := updatedModel.(ReviewModel)
	require.False(t, rm.done)
	require.Equal(t, DecisionNone, rm.decision)
	require.Nil(t, cmd)
}

func TestReviewModel_View_Normal(t *testing.T) {
	model := NewReviewModel("feat: add new feature")
	view := model.View()
	
	require.Contains(t, view, "Commit Preview")
	require.Contains(t, view, "feat: add new feature")
	require.Contains(t, view, "[A] Accept")
	require.Contains(t, view, "[E] Edit")
	require.Contains(t, view, "[C] Cancel")
}

func TestReviewModel_View_Editing(t *testing.T) {
	model := NewReviewModel("feat: test")
	model.editing = true
	view := model.View()
	
	require.Contains(t, view, "Editing commit message")
	require.Contains(t, view, "enter to save")
	require.Contains(t, view, "esc to cancel")
}

func TestReviewModel_View_MultilineMessage(t *testing.T) {
	model := NewReviewModel("feat: add new feature\n\nThis is a detailed description")
	view := model.View()
	
	require.Contains(t, view, "feat: add new feature")
	require.Contains(t, view, "This is a detailed description")
}

func TestReviewModel_IsDone(t *testing.T) {
	model := NewReviewModel("feat: test")
	
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