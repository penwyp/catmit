package workflow

import (
	"github.com/charmbracelet/lipgloss"
)

// RenderStatusBar renders a styled status bar message
func RenderStatusBar(message string, isSuccess bool) string {
	var style lipgloss.Style
	if isSuccess {
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")). // Green
			Background(lipgloss.Color("22")). // Dark green
			Bold(true).
			Padding(0, 1)
	} else {
		style = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")). // Blue
			Background(lipgloss.Color("19")). // Dark blue
			Bold(true).
			Padding(0, 1)
	}

	// Create progress indicator
	indicator := "▶"
	if isSuccess {
		indicator = "✓"
	}

	styledMessage := style.Render(indicator + " " + message)
	return styledMessage
}
