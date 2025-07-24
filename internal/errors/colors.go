package errors

import "github.com/charmbracelet/lipgloss"

// Color constants used by error handler to avoid import cycle
var (
	ColorBrightRed    = lipgloss.Color("9")
	ColorBrightYellow = lipgloss.Color("11")
)