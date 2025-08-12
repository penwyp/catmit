package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode represents different viewing modes for PR preview
type ViewMode int

const (
	ViewModeRendered ViewMode = iota
	ViewModeRawResponse
	ViewModeTemplateDebug
	ViewModeSplit
)

// String returns the string representation of ViewMode
func (v ViewMode) String() string {
	switch v {
	case ViewModeRendered:
		return "Rendered"
	case ViewModeRawResponse:
		return "Raw Response"
	case ViewModeTemplateDebug:
		return "Template Debug"
	case ViewModeSplit:
		return "Split View"
	default:
		return "Unknown"
	}
}

// EnhancedPRPreviewModel is the enhanced PR preview component with multiple view modes
type EnhancedPRPreviewModel struct {
	data              PRPreviewData
	styles            UIStyles
	width             int
	height            int
	viewMode          ViewMode
	scrollOffset      int
	showLineNumbers   bool
	syntaxHighlight   bool
	showFileDetails   bool
	maxScrollOffset   int
	contentLines      []string // Cached content lines for current view
}

// NewEnhancedPRPreviewModel creates a new enhanced PR preview model
func NewEnhancedPRPreviewModel(data PRPreviewData, styles UIStyles, width, height int) *EnhancedPRPreviewModel {
	m := &EnhancedPRPreviewModel{
		data:            data,
		styles:          styles,
		width:           width,
		height:          height,
		viewMode:        ViewModeRendered,
		scrollOffset:    0,
		showLineNumbers: true,
		syntaxHighlight: true,
		showFileDetails: false,
	}
	m.updateContentCache()
	return m
}

// Init implements tea.Model
func (m *EnhancedPRPreviewModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *EnhancedPRPreviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.cycleViewMode()
		case "1":
			m.viewMode = ViewModeRendered
			m.updateContentCache()
		case "2":
			m.viewMode = ViewModeRawResponse
			m.updateContentCache()
		case "3":
			m.viewMode = ViewModeTemplateDebug
			m.updateContentCache()
		case "4":
			m.viewMode = ViewModeSplit
			m.updateContentCache()
		case "up", "k":
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		case "down", "j":
			if m.scrollOffset < m.maxScrollOffset {
				m.scrollOffset++
			}
		case "pgup":
			m.scrollOffset -= m.height / 2
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		case "pgdown":
			m.scrollOffset += m.height / 2
			if m.scrollOffset > m.maxScrollOffset {
				m.scrollOffset = m.maxScrollOffset
			}
		case "home":
			m.scrollOffset = 0
		case "end":
			m.scrollOffset = m.maxScrollOffset
		case "l":
			m.showLineNumbers = !m.showLineNumbers
		case "s":
			m.syntaxHighlight = !m.syntaxHighlight
		case "d":
			m.showFileDetails = !m.showFileDetails
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateContentCache()
	}
	return m, nil
}

// View implements tea.Model
func (m *EnhancedPRPreviewModel) View() string {
	var content strings.Builder

	// Header
	content.WriteString(m.renderHeader())
	content.WriteString("\n")

	// View mode tabs
	content.WriteString(m.renderViewModeTabs())
	content.WriteString("\n\n")

	// Main content area
	switch m.viewMode {
	case ViewModeRendered:
		content.WriteString(m.renderRenderedView())
	case ViewModeRawResponse:
		content.WriteString(m.renderRawResponseView())
	case ViewModeTemplateDebug:
		content.WriteString(m.renderTemplateDebugView())
	case ViewModeSplit:
		content.WriteString(m.renderSplitView())
	}

	// Footer with controls
	content.WriteString("\n")
	content.WriteString(m.renderFooter())

	return content.String()
}

// cycleViewMode cycles through available view modes
func (m *EnhancedPRPreviewModel) cycleViewMode() {
	m.viewMode = (m.viewMode + 1) % 4
	m.scrollOffset = 0
	m.updateContentCache()
}

// updateContentCache updates the cached content lines for current view
func (m *EnhancedPRPreviewModel) updateContentCache() {
	switch m.viewMode {
	case ViewModeRendered:
		m.contentLines = m.getRenderedContent()
	case ViewModeRawResponse:
		m.contentLines = strings.Split(m.data.RawLLMResponse, "\n")
	case ViewModeTemplateDebug:
		m.contentLines = m.getTemplateDebugContent()
	case ViewModeSplit:
		// Split view handles its own rendering
		m.contentLines = []string{}
	}
	
	// Calculate max scroll offset
	visibleLines := m.height - 10 // Account for header, tabs, and footer
	if len(m.contentLines) > visibleLines {
		m.maxScrollOffset = len(m.contentLines) - visibleLines
	} else {
		m.maxScrollOffset = 0
	}
	
	// Ensure scroll offset is within bounds
	if m.scrollOffset > m.maxScrollOffset {
		m.scrollOffset = m.maxScrollOffset
	}
}

// renderHeader renders the header section
func (m *EnhancedPRPreviewModel) renderHeader() string {
	titleStyle := m.styles.Title
	return titleStyle.Render("Enhanced Pull Request Preview")
}

// renderViewModeTabs renders the view mode tabs
func (m *EnhancedPRPreviewModel) renderViewModeTabs() string {
	var tabs []string
	modes := []ViewMode{ViewModeRendered, ViewModeRawResponse, ViewModeTemplateDebug, ViewModeSplit}
	
	for i, mode := range modes {
		tabText := fmt.Sprintf("[%d] %s", i+1, mode.String())
		if mode == m.viewMode {
			style := lipgloss.NewStyle().
				Background(m.styles.Colors.Blue).
				Foreground(m.styles.Colors.White).
				Padding(0, 1)
			tabs = append(tabs, style.Render(tabText))
		} else {
			style := lipgloss.NewStyle().
				Foreground(m.styles.Colors.Gray).
				Padding(0, 1)
			tabs = append(tabs, style.Render(tabText))
		}
	}
	
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

// renderFooter renders the footer with controls
func (m *EnhancedPRPreviewModel) renderFooter() string {
	var controls []string
	hintStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Italic(true)
	
	// Navigation controls
	controls = append(controls, "[Tab] Switch view")
	controls = append(controls, "[↑/↓] Scroll")
	
	// Toggle controls
	if m.showLineNumbers {
		controls = append(controls, "[L] Hide line numbers")
	} else {
		controls = append(controls, "[L] Show line numbers")
	}
	
	if m.syntaxHighlight {
		controls = append(controls, "[S] Disable highlighting")
	} else {
		controls = append(controls, "[S] Enable highlighting")
	}
	
	// Scroll indicator
	if m.maxScrollOffset > 0 {
		scrollPercent := int(float64(m.scrollOffset) / float64(m.maxScrollOffset) * 100)
		scrollInfo := fmt.Sprintf("Line %d/%d (%d%%)", 
			m.scrollOffset+1, len(m.contentLines), scrollPercent)
		controls = append(controls, scrollInfo)
	}
	
	return hintStyle.Render(strings.Join(controls, " • "))
}

// renderRenderedView renders the processed PR content
func (m *EnhancedPRPreviewModel) renderRenderedView() string {
	var content strings.Builder
	
	// PR metadata
	content.WriteString(m.renderPRMetadata())
	content.WriteString("\n")
	
	// Rendered content with scrolling
	visibleLines := m.height - 12
	startLine := m.scrollOffset
	endLine := startLine + visibleLines
	if endLine > len(m.contentLines) {
		endLine = len(m.contentLines)
	}
	
	for i := startLine; i < endLine; i++ {
		line := m.contentLines[i]
		if m.showLineNumbers {
			lineNumStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Width(4)
			content.WriteString(lineNumStyle.Render(fmt.Sprintf("%3d ", i+1)))
		}
		
		if m.syntaxHighlight {
			content.WriteString(m.highlightMarkdown(line))
		} else {
			content.WriteString(line)
		}
		content.WriteString("\n")
	}
	
	return content.String()
}

// renderRawResponseView renders the raw LLM response
func (m *EnhancedPRPreviewModel) renderRawResponseView() string {
	var content strings.Builder
	
	// Info about raw response
	infoStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Blue)
	content.WriteString(infoStyle.Render("Original LLM Response:"))
	content.WriteString("\n\n")
	
	// Raw content with scrolling
	visibleLines := m.height - 12
	startLine := m.scrollOffset
	endLine := startLine + visibleLines
	if endLine > len(m.contentLines) {
		endLine = len(m.contentLines)
	}
	
	for i := startLine; i < endLine; i++ {
		line := m.contentLines[i]
		if m.showLineNumbers {
			lineNumStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Width(4)
			content.WriteString(lineNumStyle.Render(fmt.Sprintf("%3d ", i+1)))
		}
		content.WriteString(line)
		content.WriteString("\n")
	}
	
	return content.String()
}

// renderTemplateDebugView renders template processing details
func (m *EnhancedPRPreviewModel) renderTemplateDebugView() string {
	var content strings.Builder
	
	// Template info
	infoStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Blue)
	content.WriteString(infoStyle.Render("Template Processing Debug:"))
	content.WriteString("\n\n")
	
	// Show template name
	labelStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray)
	content.WriteString(labelStyle.Render("Template: "))
	content.WriteString(m.data.TemplateName)
	content.WriteString("\n\n")
	
	// Show template variables
	if len(m.data.TemplateVars) > 0 {
		content.WriteString(labelStyle.Render("Variables:"))
		content.WriteString("\n")
		for key, value := range m.data.TemplateVars {
			varStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Green)
			content.WriteString("  ")
			content.WriteString(varStyle.Render(key))
			content.WriteString(": ")
			content.WriteString(value)
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}
	
	// Show processing errors if any
	if len(m.data.ProcessingErrors) > 0 {
		errorStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Red)
		content.WriteString(errorStyle.Render("Processing Errors:"))
		content.WriteString("\n")
		for _, err := range m.data.ProcessingErrors {
			content.WriteString("  • ")
			content.WriteString(err)
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}
	
	// Show template content with variable substitutions highlighted
	content.WriteString(labelStyle.Render("Template Content:"))
	content.WriteString("\n")
	
	visibleLines := m.height - 20
	startLine := m.scrollOffset
	endLine := startLine + visibleLines
	if endLine > len(m.contentLines) {
		endLine = len(m.contentLines)
	}
	
	for i := startLine; i < endLine; i++ {
		line := m.contentLines[i]
		if m.showLineNumbers {
			lineNumStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Width(4)
			content.WriteString(lineNumStyle.Render(fmt.Sprintf("%3d ", i+1)))
		}
		content.WriteString(m.highlightTemplateVars(line))
		content.WriteString("\n")
	}
	
	return content.String()
}

// renderSplitView renders side-by-side comparison
func (m *EnhancedPRPreviewModel) renderSplitView() string {
	// Calculate widths for split view
	totalWidth := m.width
	dividerWidth := 3
	halfWidth := (totalWidth - dividerWidth) / 2
	
	// Get content for both sides
	rawLines := strings.Split(m.data.RawLLMResponse, "\n")
	renderedLines := m.getRenderedContent()
	
	// Calculate visible lines
	visibleLines := m.height - 12
	startLine := m.scrollOffset
	endLine := startLine + visibleLines
	
	var content strings.Builder
	
	// Headers
	leftHeader := lipgloss.NewStyle().
		Width(halfWidth).
		Align(lipgloss.Center).
		Background(m.styles.Colors.Blue).
		Foreground(m.styles.Colors.White).
		Render("Raw Response")
	
	rightHeader := lipgloss.NewStyle().
		Width(halfWidth).
		Align(lipgloss.Center).
		Background(m.styles.Colors.Green).
		Foreground(m.styles.Colors.White).
		Render("Rendered")
	
	divider := lipgloss.NewStyle().
		Width(dividerWidth).
		Align(lipgloss.Center).
		Render("│")
	
	content.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftHeader, divider, rightHeader))
	content.WriteString("\n")
	
	// Content lines
	for i := startLine; i < endLine; i++ {
		var leftLine, rightLine string
		
		// Get left side (raw)
		if i < len(rawLines) {
			leftLine = m.truncateLine(rawLines[i], halfWidth-4)
			if m.showLineNumbers {
				leftLine = fmt.Sprintf("%3d %s", i+1, leftLine)
			}
		}
		
		// Get right side (rendered)
		if i < len(renderedLines) {
			rightLine = m.truncateLine(renderedLines[i], halfWidth-4)
			if m.showLineNumbers {
				rightLine = fmt.Sprintf("%3d %s", i+1, rightLine)
			}
		}
		
		// Style and join
		leftStyle := lipgloss.NewStyle().Width(halfWidth)
		rightStyle := lipgloss.NewStyle().Width(halfWidth)
		
		content.WriteString(leftStyle.Render(leftLine))
		content.WriteString(divider)
		content.WriteString(rightStyle.Render(rightLine))
		content.WriteString("\n")
	}
	
	return content.String()
}

// Helper methods

func (m *EnhancedPRPreviewModel) renderPRMetadata() string {
	var content strings.Builder
	
	infoStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray)
	content.WriteString(m.renderInfoLine("Provider", m.data.Provider, infoStyle))
	content.WriteString(m.renderInfoLine("Remote", m.data.Remote, infoStyle))
	content.WriteString(m.renderInfoLine("From", m.data.Head, infoStyle))
	content.WriteString(m.renderInfoLine("To", m.data.Base, infoStyle))
	
	if m.data.IsDraft {
		draftStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Yellow)
		content.WriteString(m.renderInfoLine("Status", "Draft", draftStyle))
	}
	
	if m.data.UsingTemplate {
		templateStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Blue)
		templateName := m.data.TemplateName
		if templateName == "" {
			templateName = "Default"
		}
		content.WriteString(m.renderInfoLine("Template", templateName, templateStyle))
	}
	
	return content.String()
}

func (m *EnhancedPRPreviewModel) renderInfoLine(label, value string, style lipgloss.Style) string {
	labelStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Width(10)
	return labelStyle.Render(label+":") + " " + style.Render(value) + "\n"
}

func (m *EnhancedPRPreviewModel) getRenderedContent() []string {
	var lines []string
	
	// Title
	lines = append(lines, "Title: "+m.data.Title)
	lines = append(lines, "")
	
	// Body
	if m.data.Body != "" {
		lines = append(lines, "Description:")
		bodyLines := strings.Split(m.data.Body, "\n")
		lines = append(lines, bodyLines...)
	}
	
	// File changes if showing details
	if m.showFileDetails && len(m.data.FileChanges) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Changes:")
		for _, fc := range m.data.FileChanges {
			changeLine := fmt.Sprintf("  %s %s (+%d -%d)", 
				m.getChangeIcon(fc.ChangeType), fc.Path, fc.Additions, fc.Deletions)
			lines = append(lines, changeLine)
		}
	}
	
	return lines
}

func (m *EnhancedPRPreviewModel) getTemplateDebugContent() []string {
	if m.data.TemplateContent == "" {
		return []string{"No template content available"}
	}
	return strings.Split(m.data.TemplateContent, "\n")
}

func (m *EnhancedPRPreviewModel) highlightMarkdown(line string) string {
	// Simple markdown highlighting
	if strings.HasPrefix(line, "# ") {
		return m.styles.Title.Render(line)
	}
	if strings.HasPrefix(line, "## ") {
		return m.styles.CommitType.Render(line)
	}
	if strings.HasPrefix(line, "### ") {
		return m.styles.CommitDesc.Render(line)
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		bulletStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Blue)
		return bulletStyle.Render(line[:2]) + line[2:]
	}
	if strings.Contains(line, "`") {
		// Highlight code snippets
		codeStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Green)
		parts := strings.Split(line, "`")
		for i := 1; i < len(parts); i += 2 {
			if i < len(parts) {
				parts[i] = codeStyle.Render(parts[i])
			}
		}
		return strings.Join(parts, "")
	}
	return line
}

func (m *EnhancedPRPreviewModel) highlightTemplateVars(line string) string {
	// Highlight template variables like {{ .Variable }}
	if !strings.Contains(line, "{{") {
		return line
	}
	
	varStyle := lipgloss.NewStyle().
		Foreground(m.styles.Colors.Green).
		Background(m.styles.Colors.Black)
	
	result := line
	start := 0
	for {
		startIdx := strings.Index(result[start:], "{{")
		if startIdx == -1 {
			break
		}
		startIdx += start
		
		endIdx := strings.Index(result[startIdx:], "}}")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx + 2
		
		variable := result[startIdx:endIdx]
		highlighted := varStyle.Render(variable)
		result = result[:startIdx] + highlighted + result[endIdx:]
		start = startIdx + len(highlighted)
	}
	
	return result
}

func (m *EnhancedPRPreviewModel) truncateLine(line string, maxWidth int) string {
	if len(line) <= maxWidth {
		return line
	}
	if maxWidth <= 3 {
		return "..."
	}
	return line[:maxWidth-3] + "..."
}

func (m *EnhancedPRPreviewModel) getChangeIcon(changeType string) string {
	switch changeType {
	case "added":
		return "+"
	case "deleted":
		return "-"
	case "modified":
		return "●"
	default:
		return "○"
	}
}