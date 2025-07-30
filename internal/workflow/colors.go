package workflow

import "github.com/charmbracelet/lipgloss"

// Color constants used by status bar to avoid import cycle
var (
	ColorGreen     = lipgloss.Color("42")
	ColorDarkGreen = lipgloss.Color("22")
	ColorBlue      = lipgloss.Color("39")
	ColorDarkBlue  = lipgloss.Color("19")
)