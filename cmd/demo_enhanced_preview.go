package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/ui"
	"github.com/spf13/cobra"
)

// demoEnhancedPreviewCmd represents the demo enhanced preview command
var demoEnhancedPreviewCmd = &cobra.Command{
	Use:    "demo-enhanced-preview",
	Short:  "Demo the enhanced PR preview UI",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create sample PR data with all enhanced fields
		prData := ui.PRPreviewData{
			Title: "feat: implement enhanced PR preview with multiple view modes",
			Body: `## Summary
This PR introduces an enhanced PR preview experience with multiple view modes to help users better understand the PR content before submission.

## Key Features
- **Multiple View Modes**: Switch between rendered, raw response, template debug, and split views
- **Syntax Highlighting**: Markdown content is highlighted for better readability  
- **Template Debugging**: See template variables and how they're substituted
- **Scrolling Support**: Handle long PR descriptions with smooth scrolling
- **Keyboard Navigation**: Intuitive shortcuts for all features

## Changes
- Added EnhancedPRPreviewModel with tea.Model interface
- Implemented four distinct view modes
- Added syntax highlighting for markdown
- Added template variable highlighting
- Improved keyboard navigation

## Testing
- Unit tests for all view modes
- Integration tests for keyboard navigation
- Performance tests for large PR descriptions`,
			Base:     "main",
			Head:     "feature/enhanced-pr-preview",
			Remote:   "origin",
			Provider: "github",
			IsDraft:  false,
			HasChanges: true,
			FileChanges: []ui.FileChange{
				{Path: "internal/ui/pr_preview_enhanced.go", Additions: 650, Deletions: 0, ChangeType: "added"},
				{Path: "internal/ui/pr_workflow_model.go", Additions: 45, Deletions: 20, ChangeType: "modified"},
				{Path: "internal/ui/commit_workflow_model.go", Additions: 35, Deletions: 15, ChangeType: "modified"},
				{Path: "internal/ui/pr_preview.go", Additions: 20, Deletions: 0, ChangeType: "modified"},
				{Path: "internal/ui/loading.go", Additions: 5, Deletions: 2, ChangeType: "modified"},
			},
			UsingTemplate: true,
			TemplateName:  "enhanced-pr-template",
			RawLLMResponse: `feat: implement enhanced PR preview with multiple view modes

I've analyzed the requirements and implemented a comprehensive enhanced PR preview system. The new implementation provides multiple view modes to give users full visibility into the PR creation process.

The key improvements include:
1. A new EnhancedPRPreviewModel that supports four different view modes
2. Syntax highlighting for better readability
3. Template debugging capabilities
4. Full keyboard navigation support
5. Scrolling for long content

This will significantly improve the user experience when reviewing PR content before submission.`,
			TemplateContent: `## Summary
{{ .Summary }}

## Key Features
{{ .Features }}

## Changes
{{ .Changes }}

## Testing
{{ .Testing }}`,
			TemplateVars: map[string]string{
				"Summary":  "This PR introduces an enhanced PR preview experience...",
				"Features": "- Multiple View Modes\n- Syntax Highlighting\n- Template Debugging",
				"Changes":  "- Added EnhancedPRPreviewModel\n- Implemented view modes",
				"Testing":  "- Unit tests\n- Integration tests",
			},
			ProcessingErrors: []string{},
		}

		// Create the enhanced preview model
		width, height := 120, 40 // Default terminal size
		model := ui.NewEnhancedPRPreviewModel(prData, ui.DefaultStyles(), width, height)

		// Create a wrapper model to handle quit
		wrappedModel := &enhancedPreviewWrapper{model: model}

		// Run the program
		p := tea.NewProgram(wrappedModel, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

type enhancedPreviewWrapper struct {
	model tea.Model
}

func (w *enhancedPreviewWrapper) Init() tea.Cmd {
	return w.model.Init()
}

func (w *enhancedPreviewWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return w, tea.Quit
		}
	case tea.WindowSizeMsg:
		// Update the model with new size
		return w.model.Update(msg)
	}
	
	var cmd tea.Cmd
	w.model, cmd = w.model.Update(msg)
	return w, cmd
}

func (w *enhancedPreviewWrapper) View() string {
	var content strings.Builder
	
	// Add header
	content.WriteString("🚀 Enhanced PR Preview Demo\n")
	content.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	
	// Add the model view
	content.WriteString(w.model.View())
	
	// Add footer with quit instruction
	content.WriteString("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	content.WriteString("\n[Q] Quit Demo")
	
	return content.String()
}

func init() {
	rootCmd.AddCommand(demoEnhancedPreviewCmd)
}