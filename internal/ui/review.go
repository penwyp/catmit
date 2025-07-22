package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Decision represents the user's choice in the Review UI
type Decision int

// buttonState defines the index of the buttons
type buttonState int

const (
	DecisionNone Decision = iota
	DecisionAccept
	DecisionCancel
)

const (
	buttonAccept buttonState = iota
	buttonEdit
	buttonRegenerate
	buttonCancel
)

// ReviewModel is used to display the commit message for user confirmation/editing.
// When the user presses a/e/c, the program ends and returns the decision and final message.
// For user-friendliness, supports up/down key to switch buttons (simplified implementation).

type ReviewModel struct {
	message        string // current commit message
	lang           string // language
	editing        bool   // whether in editing mode
	textInput      textinput.Model
	decision       Decision
	done           bool
	selectedButton buttonState
	// Responsive terminal size support
	terminalWidth  int // terminal width
	terminalHeight int // terminal height
}

// NewReviewModel creates the initial model.
func NewReviewModel(msg, lang string) *ReviewModel {
	// Remove \r and trim whitespace to avoid TUI rendering issues caused by carriage returns
	cleanMsg := strings.TrimSpace(strings.ReplaceAll(msg, "\r", ""))

	ti := textinput.New()
	ti.Placeholder = "Edit commit message"
	ti.SetValue(cleanMsg)
	ti.CharLimit = 256
	ti.Focus()
	return &ReviewModel{
		message:        cleanMsg,
		lang:           lang,
		editing:        false,
		textInput:      ti,
		selectedButton: buttonAccept, // Default to Accept selected
		terminalWidth:  80,           // Default width, will be updated by WindowSizeMsg
		terminalHeight: 24,           // Default height, will be updated by WindowSizeMsg
	}
}

// Init implements the tea.Model interface
func (m *ReviewModel) Init() tea.Cmd { return nil }

// Update handles key events
func (m *ReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Update terminal size
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height
		return m, nil
	case tea.KeyMsg:
		// Handle Ctrl+C in all modes: always cancel and exit
		if msg.String() == "ctrl+c" {
			m.decision = DecisionCancel
			m.done = true
			return m, tea.Quit
		}

		if m.editing {
			// In editing mode, delegate to textinput
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			switch msg.String() {
			case "enter":
				m.message = strings.TrimSpace(m.textInput.Value())
				m.editing = false
			case "esc":
				m.editing = false
			}
			return m, cmd
		}

		// Navigation and selection logic
		switch msg.String() {
		// Switch buttons
		case "left", "h", "up", "k":
			m.selectedButton--
			if m.selectedButton < buttonAccept {
				m.selectedButton = buttonCancel
			}
		case "right", "l", "down", "j":
			m.selectedButton++
			if m.selectedButton > buttonCancel {
				m.selectedButton = buttonAccept
			}
		// Shortcuts
		case "a", "A":
			m.decision = DecisionAccept
			m.done = true
			return m, tea.Quit
		case "e", "E":
			m.editing = true
			return m, nil
		case "c", "C", "q", "Q", "esc":
			m.decision = DecisionCancel
			m.done = true
			return m, tea.Quit
		// Confirm selection
		case "enter":
			switch m.selectedButton {
			case buttonAccept:
				m.decision = DecisionAccept
				m.done = true
				return m, tea.Quit
			case buttonEdit:
				m.editing = true
				return m, nil
			case buttonCancel:
				m.decision = DecisionCancel
				m.done = true
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

// calculateContentWidth calculates the dynamic content width based on terminal width
func (m *ReviewModel) calculateContentWidth() int {
	const (
		minWidth = 60  // minimum width
		maxWidth = 120 // maximum width
		margin   = 4   // left and right margin
	)

	// Calculate available width (excluding margin)
	availableWidth := m.terminalWidth - margin

	// Apply min and max width constraints
	if availableWidth < minWidth {
		return minWidth
	}
	if availableWidth > maxWidth {
		return maxWidth
	}

	return availableWidth
}

// View renders the UI
func (m *ReviewModel) View() string {
	// --- Palette ---
	const (
		cGray   = lipgloss.Color("245")
		cBlue   = lipgloss.Color("39")
		cGreen  = lipgloss.Color("42")
		cYellow = lipgloss.Color("220")
		cRed    = lipgloss.Color("196")
		cWhite  = lipgloss.Color("255")
		cBlack  = lipgloss.Color("0")
		padding = 1
	)

	// Dynamically calculate content width
	contentWidth := m.calculateContentWidth()

	// --- Editing mode ---
	if m.editing {
		promptStyle := lipgloss.NewStyle().Foreground(cYellow).Bold(true)
		prompt := promptStyle.Render("Editing commit message (enter to save, esc to cancel):")
		return fmt.Sprintf("\n%s\n%s\n", prompt, m.textInput.View())
	}

	// --- Style definitions ---
	borderStyle := lipgloss.NewStyle().Foreground(cBlue)
	titleStyle := lipgloss.NewStyle().Foreground(cWhite).Bold(true)
	langStyle := lipgloss.NewStyle().Foreground(cGray)
	commitTypeStyle := lipgloss.NewStyle().Foreground(cYellow)
	commitDescStyle := lipgloss.NewStyle().Foreground(cWhite)
	commitBodyStyle := lipgloss.NewStyle().Foreground(cGray)

	// --- Helper function: line renderer ---
	renderLine := func(content string) string {
		contentDisplayWidth := lipgloss.Width(content)
		// Handle overflow: truncate if content is too long
		if contentDisplayWidth > contentWidth {
			// Use smart truncation to preserve important info
			truncated := truncateContent(content, contentWidth-3) + "..."
			content = truncated
			contentDisplayWidth = lipgloss.Width(content)
		}

		linePadding := contentWidth - contentDisplayWidth
		if linePadding < 0 {
			linePadding = 0
		}
		return borderStyle.Render("│") + content + strings.Repeat(" ", linePadding) + borderStyle.Render("│")
	}

	// --- Helper function: button renderer ---
	renderButton := func(hint, text string, isSelected bool, hintStyle, textStyle, selectedBg lipgloss.Color) string {
		hStyle := lipgloss.NewStyle().Foreground(hintStyle)
		tStyle := lipgloss.NewStyle().Foreground(textStyle)

		if isSelected {
			// When button is selected, set high-contrast foreground for readability
			fgColor := cBlack
			// On red background, white text is clearer
			if selectedBg == cRed {
				fgColor = cWhite
			}
			hStyle = hStyle.Background(selectedBg).Foreground(fgColor)
			tStyle = tStyle.Background(selectedBg).Foreground(fgColor)
		}

		return lipgloss.JoinHorizontal(lipgloss.Top,
			hStyle.Padding(0, 1).Render(hint),
			tStyle.Padding(0, 1).Render(text),
		)
	}

	// --- Build title ---
	titleText := titleStyle.Render("Commit Preview") + langStyle.Render(fmt.Sprintf(" (%s)", m.lang))
	titlePadding := contentWidth - lipgloss.Width(titleText)
	if titlePadding < 0 {
		titlePadding = 0
	}
	header := borderStyle.Render("┌") + strings.Repeat(borderStyle.Render("─"), titlePadding/2) +
		titleText + strings.Repeat(borderStyle.Render("─"), titlePadding-titlePadding/2) +
		borderStyle.Render("┐")

	// --- Build message body ---
	var bodyBuilder strings.Builder
	lines := strings.Split(m.message, "\n")

	// Render first line (Subject)
	if len(lines) > 0 {
		parts := strings.SplitN(lines[0], ":", 2)
		var subject string
		if len(parts) == 2 {
			subject = commitTypeStyle.Render(parts[0]+":") + commitDescStyle.Render(parts[1])
		} else {
			subject = commitDescStyle.Render(lines[0])
		}
		bodyBuilder.WriteString(renderLine(" "+subject) + "\n")
	}

	// Render subsequent lines (Body)
	if len(lines) > 1 {
		bodyBuilder.WriteString(renderLine("") + "\n") // blank line
		bodyText := strings.Join(lines[1:], "\n")
		// Auto-wrap body, -2 for left/right padding
		wrappedBody := wordWrap(bodyText, contentWidth-2)
		for _, l := range strings.Split(wrappedBody, "\n") {
			lineContent := " " + commitBodyStyle.Render(l)
			bodyBuilder.WriteString(renderLine(lineContent) + "\n")
		}
	}

	// --- Build interactive buttons ---
	btnAccept := renderButton("[A]", "Accept", m.selectedButton == buttonAccept, cGray, cGreen, cGreen)
	btnEdit := renderButton("[E]", "Edit", m.selectedButton == buttonEdit, cGray, cYellow, cYellow)
	btnCancel := renderButton("[C]", "Cancel", m.selectedButton == buttonCancel, cGray, cRed, cRed)
	buttons := lipgloss.JoinHorizontal(lipgloss.Top, btnAccept, "  ", btnEdit, "  ", btnCancel)

	// Check if buttons overflow content width, adjust layout if needed
	buttonsWidth := lipgloss.Width(buttons)
	if buttonsWidth > contentWidth-2 { // -2 for padding
		// Use compact layout
		buttons = lipgloss.JoinHorizontal(lipgloss.Top, btnAccept, " ", btnEdit, " ", btnCancel)
		buttonsWidth = lipgloss.Width(buttons)
		if buttonsWidth > contentWidth-2 {
			// If still too long, use the most compact layout
			buttons = btnAccept + " " + btnEdit + " " + btnCancel
		}
	}

	// --- Assemble footer ---
	blankLine := renderLine("")
	buttonRow := renderLine(" " + buttons)
	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", contentWidth) + "┘")
	footer := strings.Join([]string{blankLine, buttonRow, bottomBorder}, "\n")

	// Remove any trailing newlines from body to avoid breaking layout
	finalBody := strings.TrimRight(bodyBuilder.String(), "\n")

	return strings.Join([]string{header, finalBody, footer}, "\n")
}

// IsDone returns whether the model is finished, along with the decision and final message.
func (m *ReviewModel) IsDone() (bool, Decision, string) {
	return m.done, m.decision, m.message
}
