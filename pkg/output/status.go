package output

import (
	"github.com/charmbracelet/lipgloss"
)

// StatusType represents different types of status messages
type StatusType int

const (
	StatusSuccess  StatusType = iota // ✓ with green background
	StatusProgress                   // ▶ with blue background
	StatusInfo                       // ℹ️ with cyan background
	StatusWarning                    // ⚠️ with yellow background
	StatusError                      // ❌ with red background
)

// Color constants for status bar styling
var (
	ColorGreen      = lipgloss.Color("42")
	ColorDarkGreen  = lipgloss.Color("22")
	ColorBlue       = lipgloss.Color("39")
	ColorDarkBlue   = lipgloss.Color("19")
	ColorCyan       = lipgloss.Color("51")
	ColorDarkCyan   = lipgloss.Color("23")
	ColorYellow     = lipgloss.Color("226")
	ColorDarkYellow = lipgloss.Color("136")
	ColorRed        = lipgloss.Color("196")
	ColorDarkRed    = lipgloss.Color("88")
)

// RenderStatusBar renders a styled status bar message with appropriate icon and colors
func RenderStatusBar(message string, isSuccess bool) string {
	if isSuccess {
		return RenderStatusBarWithType(message, StatusSuccess)
	}
	return RenderStatusBarWithType(message, StatusProgress)
}

// RenderStatusBarWithType renders a styled status bar message with specific status type
func RenderStatusBarWithType(message string, statusType StatusType) string {
	var style lipgloss.Style
	var indicator string

	switch statusType {
	case StatusSuccess:
		style = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Background(ColorDarkGreen).
			Bold(true).
			Padding(0, 1)
		indicator = "✓"

	case StatusProgress:
		style = lipgloss.NewStyle().
			Foreground(ColorBlue).
			Background(ColorDarkBlue).
			Bold(true).
			Padding(0, 1)
		indicator = "▶"

	case StatusInfo:
		style = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Background(ColorDarkCyan).
			Bold(true).
			Padding(0, 1)
		indicator = "💭"

	case StatusWarning:
		style = lipgloss.NewStyle().
			Foreground(ColorYellow).
			Background(ColorDarkYellow).
			Bold(true).
			Padding(0, 1)
		indicator = "⚠️"

	case StatusError:
		style = lipgloss.NewStyle().
			Foreground(ColorRed).
			Background(ColorDarkRed).
			Bold(true).
			Padding(0, 1)
		indicator = "❌"

	default:
		// Fallback to progress style
		style = lipgloss.NewStyle().
			Foreground(ColorBlue).
			Background(ColorDarkBlue).
			Bold(true).
			Padding(0, 1)
		indicator = "▶"
	}

	styledMessage := style.Render(indicator + " " + message)
	return styledMessage
}