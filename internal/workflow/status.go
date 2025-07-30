package workflow

import (
	"github.com/charmbracelet/lipgloss"
)

// RenderStatusBar renders a styled status bar message
func RenderStatusBar(message string, isSuccess bool) string {
	var style lipgloss.Style
	if isSuccess {
		style = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Background(ColorDarkGreen).
			Bold(true).
			Padding(0, 1)
	} else {
		style = lipgloss.NewStyle().
			Foreground(ColorBlue).
			Background(ColorDarkBlue).
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
