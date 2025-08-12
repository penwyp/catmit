package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// UIColors defines a unified color theme
type UIColors struct {
	// Basic colors
	Gray    lipgloss.Color
	Blue    lipgloss.Color
	Green   lipgloss.Color
	Yellow  lipgloss.Color
	Red     lipgloss.Color
	White   lipgloss.Color
	Black   lipgloss.Color
	Orange  lipgloss.Color
	HotPink lipgloss.Color

	// Extended colors
	DarkGreen     lipgloss.Color
	DarkBlue      lipgloss.Color
	Cyan          lipgloss.Color
	BrightGreen   lipgloss.Color
	DarkGray      lipgloss.Color
	LightGray     lipgloss.Color
	BrightRed     lipgloss.Color
	BrightYellow  lipgloss.Color
	BrightCyan    lipgloss.Color
	BrightBlue    lipgloss.Color
	DarkOrange    lipgloss.Color
	SecondaryGray lipgloss.Color
}

// DefaultColors returns the default color theme
func DefaultColors() UIColors {
	return UIColors{
		// Basic colors
		Gray:   "245",
		Blue:   "39",
		Green:  "42",
		Yellow: "220",
		Red:    "196",
		White:  "255",
		Black:  "0",
		Orange: "208",

		// Extended colors
		HotPink:       "205",
		DarkGreen:     "22",
		DarkBlue:      "19",
		Cyan:          "86",
		BrightGreen:   "46",
		DarkGray:      "240",
		LightGray:     "241",
		BrightRed:     "9",
		BrightYellow:  "11",
		BrightCyan:    "14",
		BrightBlue:    "12",
		DarkOrange:    "33",
		SecondaryGray: "250",
	}
}

// UIStyles defines a unified style set
type UIStyles struct {
	Colors     UIColors
	Border     lipgloss.Style
	Title      lipgloss.Style
	Lang       lipgloss.Style
	Success    lipgloss.Style
	Warning    lipgloss.Style
	Error      lipgloss.Style
	Progress   lipgloss.Style
	CommitType lipgloss.Style
	CommitDesc lipgloss.Style
	CommitBody lipgloss.Style
	Info       lipgloss.Style
	Dim        lipgloss.Style
	Help       lipgloss.Style
	Framework  lipgloss.Style
}

// DefaultStyles returns the default style set
func DefaultStyles() UIStyles {
	colors := DefaultColors()
	return UIStyles{
		Colors:     colors,
		Border:     lipgloss.NewStyle().Foreground(colors.Blue),
		Title:      lipgloss.NewStyle().Foreground(colors.White).Bold(true),
		Lang:       lipgloss.NewStyle().Foreground(colors.Gray),
		Success:    lipgloss.NewStyle().Foreground(colors.Green),
		Warning:    lipgloss.NewStyle().Foreground(colors.Yellow),
		Error:      lipgloss.NewStyle().Foreground(colors.Red),
		Progress:   lipgloss.NewStyle().Foreground(colors.Yellow),
		CommitType: lipgloss.NewStyle().Foreground(colors.Yellow),
		CommitDesc: lipgloss.NewStyle().Foreground(colors.White),
		CommitBody: lipgloss.NewStyle().Foreground(colors.Gray),
		Info:       lipgloss.NewStyle().Foreground(colors.Cyan),
		Dim:        lipgloss.NewStyle().Foreground(colors.DarkGray),
		Help:       lipgloss.NewStyle().Foreground(colors.LightGray),
		Framework:  lipgloss.NewStyle().Foreground(colors.BrightCyan),
	}
}

// wordWrap wraps text, supporting CJK characters
func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}

	if s == "" {
		return ""
	}

	var finalResult strings.Builder
	paragraphs := strings.Split(s, "\n")

	for i, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) == "" {
			if i > 0 {
				finalResult.WriteString("\n")
			}
			continue
		}

		// Use Lipgloss's text wrapping capability, supporting CJK characters
		wrappedParagraph := wrapParagraph(paragraph, width)
		finalResult.WriteString(wrappedParagraph)

		if i < len(paragraphs)-1 {
			finalResult.WriteString("\n")
		}
	}
	return finalResult.String()
}

// wrapParagraph wraps a single paragraph, supporting CJK characters and smart line breaks
func wrapParagraph(paragraph string, width int) string {
	var result strings.Builder
	var line strings.Builder
	words := strings.Fields(paragraph)

	for _, word := range words {
		// Check if the current line is empty
		if line.Len() == 0 {
			line.WriteString(word)
		} else {
			// Calculate the width after adding a space and the new word
			testLine := line.String() + " " + word
			testWidth := lipgloss.Width(testLine)

			if testWidth <= width {
				line.WriteString(" ")
				line.WriteString(word)
			} else {
				// Current line is full, wrap to next line
				result.WriteString(line.String() + "\n")
				line.Reset()
				line.WriteString(word)
			}
		}

		// If a single word is too long, force a line break
		if lipgloss.Width(line.String()) > width {
			result.WriteString(line.String() + "\n")
			line.Reset()
		}
	}

	// Add the last line
	if line.Len() > 0 {
		result.WriteString(line.String())
	}

	return result.String()
}

// Button represents an interactive button
type Button struct {
	Hint       string
	Text       string
	HintStyle  lipgloss.Style
	TextStyle  lipgloss.Style
	SelectedBg lipgloss.Color
}




// CalculateContentWidth calculates responsive content width
func CalculateContentWidth(terminalWidth int) int {
	const (
		minWidth = 60
		maxWidth = 120
		margin   = 4
	)

	availableWidth := terminalWidth - margin

	if availableWidth < minWidth {
		return minWidth
	}
	if availableWidth > maxWidth {
		return maxWidth
	}

	return availableWidth
}


