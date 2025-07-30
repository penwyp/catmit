package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestEnhancedPRPreviewModel(t *testing.T) {
	// Test data
	prData := PRPreviewData{
		Title:          "feat: add enhanced PR preview",
		Body:           "This PR adds enhanced preview functionality\n\n## Changes\n- Added multiple view modes\n- Added keyboard navigation",
		Base:           "main",
		Head:           "feature/enhanced-preview",
		Remote:         "origin",
		Provider:       "github",
		IsDraft:        false,
		HasChanges:     true,
		FileChanges:    []FileChange{{Path: "ui/preview.go", Additions: 100, Deletions: 20, ChangeType: "modified"}},
		UsingTemplate:  true,
		TemplateName:   "Default",
		RawLLMResponse: "feat: add enhanced PR preview\n\nImplemented multiple view modes for better PR review experience",
		TemplateVars: map[string]string{
			"Title":  "feat: add enhanced PR preview",
			"Branch": "feature/enhanced-preview",
		},
	}

	// Create model
	model := NewEnhancedPRPreviewModel(prData, DefaultStyles(), 100, 40)

	// Test initial state
	assert.Equal(t, ViewModeRendered, model.viewMode)
	assert.True(t, model.showLineNumbers)
	assert.True(t, model.syntaxHighlight)

	// Test view output
	view := model.View()
	assert.Contains(t, view, "Enhanced Pull Request Preview")
	assert.Contains(t, view, "feat: add enhanced PR preview")

	// Test view mode switching
	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	enhancedModel := updatedModel.(*EnhancedPRPreviewModel)
	assert.Equal(t, ViewModeRawResponse, enhancedModel.viewMode)

	// Test tab cycling
	updatedModel, _ = enhancedModel.Update(tea.KeyMsg{Type: tea.KeyTab})
	enhancedModel = updatedModel.(*EnhancedPRPreviewModel)
	assert.Equal(t, ViewModeTemplateDebug, enhancedModel.viewMode)

	// Test line numbers toggle
	updatedModel, _ = enhancedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	enhancedModel = updatedModel.(*EnhancedPRPreviewModel)
	assert.False(t, enhancedModel.showLineNumbers)

	// Test scrolling
	enhancedModel.maxScrollOffset = 10
	updatedModel, _ = enhancedModel.Update(tea.KeyMsg{Type: tea.KeyDown})
	enhancedModel = updatedModel.(*EnhancedPRPreviewModel)
	assert.Equal(t, 1, enhancedModel.scrollOffset)
}

func TestViewModes(t *testing.T) {
	prData := PRPreviewData{
		Title:          "test: view modes",
		Body:           "Testing different view modes",
		RawLLMResponse: "Raw LLM response content",
		TemplateContent: "Template: {{ .Title }}",
		TemplateVars: map[string]string{
			"Title": "test: view modes",
		},
	}

	model := NewEnhancedPRPreviewModel(prData, DefaultStyles(), 100, 40)

	// Test each view mode
	testCases := []struct {
		mode     ViewMode
		contains []string
	}{
		{
			mode:     ViewModeRendered,
			contains: []string{"Title:", "test: view modes", "Description:"},
		},
		{
			mode:     ViewModeRawResponse,
			contains: []string{"Original LLM Response:", "Raw LLM response content"},
		},
		{
			mode:     ViewModeTemplateDebug,
			contains: []string{"Template Processing Debug:", "Variables:"},
		},
		{
			mode:     ViewModeSplit,
			contains: []string{"Raw Response", "Rendered"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.mode.String(), func(t *testing.T) {
			model.viewMode = tc.mode
			model.updateContentCache()
			view := model.View()
			
			for _, expected := range tc.contains {
				assert.Contains(t, view, expected)
			}
		})
	}
}

func TestMarkdownHighlighting(t *testing.T) {
	model := &EnhancedPRPreviewModel{
		styles: DefaultStyles(),
	}

	testCases := []struct {
		input    string
		contains string
	}{
		{
			input:    "# Header 1",
			contains: "Header 1",
		},
		{
			input:    "## Header 2",
			contains: "Header 2",
		},
		{
			input:    "- Bullet point",
			contains: "Bullet point",
		},
		{
			input:    "This has `code` inline",
			contains: "code",
		},
	}

	for _, tc := range testCases {
		result := model.highlightMarkdown(tc.input)
		assert.Contains(t, result, tc.contains)
	}
}

func TestTemplateVarHighlighting(t *testing.T) {
	model := &EnhancedPRPreviewModel{
		styles: DefaultStyles(),
	}

	testCases := []struct {
		input     string
		hasVars   bool
		varCount  int
	}{
		{
			input:    "Template: {{ .Title }}",
			hasVars:  true,
			varCount: 1,
		},
		{
			input:    "No variables here",
			hasVars:  false,
			varCount: 0,
		},
		{
			input:    "Multiple {{ .Var1 }} and {{ .Var2 }}",
			hasVars:  true,
			varCount: 2,
		},
	}

	for _, tc := range testCases {
		result := model.highlightTemplateVars(tc.input)
		// In tests without terminal, styling might not change the string
		// So just verify the function handles template variables correctly
		if tc.hasVars {
			// Verify all template variables are still present
			varCount := strings.Count(tc.input, "{{")
			resultVarCount := strings.Count(result, "{{")
			assert.GreaterOrEqual(t, resultVarCount, varCount, "Template variables should be preserved")
		} else {
			assert.Equal(t, tc.input, result)
		}
	}
}

func TestScrolling(t *testing.T) {
	prData := PRPreviewData{
		Title: "test",
		Body:  strings.Repeat("Line\n", 100), // Create many lines
	}

	model := NewEnhancedPRPreviewModel(prData, DefaultStyles(), 100, 20)
	model.updateContentCache()

	// Test scroll down
	initialOffset := model.scrollOffset
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Greater(t, model.scrollOffset, initialOffset)

	// Test scroll to end
	model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	assert.Equal(t, model.maxScrollOffset, model.scrollOffset)

	// Test scroll to home
	model.Update(tea.KeyMsg{Type: tea.KeyHome})
	assert.Equal(t, 0, model.scrollOffset)

	// Test page down
	model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	assert.Greater(t, model.scrollOffset, 0)
}