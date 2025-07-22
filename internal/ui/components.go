package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// UIColors defines a unified color theme
type UIColors struct {
	Gray   lipgloss.Color
	Blue   lipgloss.Color
	Green  lipgloss.Color
	Yellow lipgloss.Color
	Red    lipgloss.Color
	White  lipgloss.Color
	Black  lipgloss.Color
	Orange lipgloss.Color
}

// DefaultColors returns the default color theme
func DefaultColors() UIColors {
	return UIColors{
		Gray:   lipgloss.Color("245"),
		Blue:   lipgloss.Color("39"),
		Green:  lipgloss.Color("42"),
		Yellow: lipgloss.Color("220"),
		Red:    lipgloss.Color("196"),
		White:  lipgloss.Color("255"),
		Black:  lipgloss.Color("0"),
		Orange: lipgloss.Color("208"),
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
	}
}

// truncateContent intelligently truncates content, preserving important information
func truncateContent(content string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	// If the content length is within the limit, return directly
	if lipgloss.Width(content) <= maxWidth {
		return content
	}

	// Check character by character to ensure the truncated width does not exceed the limit
	var result strings.Builder
	for _, r := range content {
		testStr := result.String() + string(r)
		if lipgloss.Width(testStr) > maxWidth {
			break
		}
		result.WriteRune(r)
	}

	return result.String()
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

// RenderButton renders a single button
func RenderButton(b Button, isSelected bool) string {
	hStyle := b.HintStyle
	tStyle := b.TextStyle

	if isSelected {
		colors := DefaultColors()
		fgColor := colors.Black
		// White text is clearer on a red background
		if b.SelectedBg == colors.Red {
			fgColor = colors.White
		}
		hStyle = hStyle.Background(b.SelectedBg).Foreground(fgColor)
		tStyle = tStyle.Background(b.SelectedBg).Foreground(fgColor)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top,
		hStyle.Padding(0, 1).Render(b.Hint),
		tStyle.Padding(0, 1).Render(b.Text),
	)
}

// RenderProgressBar renders a progress bar
func RenderProgressBar(current, total int, width int, color lipgloss.Color) string {
	if total <= 0 || width <= 10 {
		return ""
	}

	percentage := float64(current) / float64(total)
	filledWidth := int(percentage * float64(width-2))

	style := lipgloss.NewStyle().Foreground(color)
	filled := strings.Repeat("█", filledWidth)
	empty := strings.Repeat("─", width-2-filledWidth)

	return style.Render("[" + filled + empty + "]")
}

// RenderStatusLine renders a status line
func RenderStatusLine(icon, text string, style lipgloss.Style) string {
	return icon + " " + style.Render(text)
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

// RenderBorder renders a border element
func RenderBorder(element string, style lipgloss.Style) string {
	return style.Render(element)
}

// CenterText centers text horizontally
func CenterText(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}

	padding := (width - textWidth) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-textWidth-padding)
}
