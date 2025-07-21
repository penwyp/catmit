package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

// SimpleReviewModel is a simplified review model using the BaseModel framework
type SimpleReviewModel struct {
	BaseModel
	message   string
	lang      string
	editing   bool
	textArea  textarea.Model
	decision  Decision
}

// NewSimpleReviewModel creates a new simplified review model
func NewSimpleReviewModel(message, lang string, appendMode bool) *SimpleReviewModel {
	// Clean the message
	cleanMsg := strings.TrimSpace(strings.ReplaceAll(message, "\r", ""))

	// Create text area for editing
	ta := textarea.New()
	ta.Placeholder = "Edit commit message..."
	ta.SetValue(cleanMsg)
	ta.CharLimit = 1000
	ta.ShowLineNumbers = false

	model := &SimpleReviewModel{
		BaseModel: NewBaseModel("Commit Preview", nil, appendMode),
		message:   cleanMsg,
		lang:      lang,
		textArea:  ta,
		decision:  DecisionNone,
	}

	// Set initial actions
	model.updateActions()
	
	// Set content renderer
	model.SetContentRenderer(model.renderContent)

	return model
}

// Init initializes the model
func (m *SimpleReviewModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *SimpleReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.HandleWindowSize(msg)
		m.textArea.SetWidth(CalculateContentWidth(m.width) - 4)
		m.textArea.SetHeight(8)
		return m, nil

	case tea.KeyMsg:
		if m.editing {
			// Handle editing mode
			switch msg.String() {
			case "esc":
				m.editing = false
				m.textArea.Blur()
				m.updateActions()
				return m, nil
			case "ctrl+s":
				m.message = strings.TrimSpace(m.textArea.Value())
				m.editing = false
				m.textArea.Blur()
				m.updateActions()
				return m, nil
			default:
				var cmd tea.Cmd
				m.textArea, cmd = m.textArea.Update(msg)
				return m, cmd
			}
		}

		// Normal mode - let BaseModel handle navigation
		cmd := m.HandleKeyboard(msg)
		if cmd != nil {
			return m, cmd
		}
	}

	return m, nil
}

// View renders the model
func (m *SimpleReviewModel) View() string {
	// Update title with language
	m.SetTitle("Commit Preview (" + m.lang + ")")
	return m.BaseModel.View()
}

// Action handlers
func (m *SimpleReviewModel) accept() tea.Cmd {
	m.decision = DecisionAccept
	m.done = true
	return tea.Quit
}

func (m *SimpleReviewModel) edit() tea.Cmd {
	m.editing = true
	m.textArea.Focus()
	m.updateActions()
	return textarea.Blink
}

func (m *SimpleReviewModel) cancel() tea.Cmd {
	m.decision = DecisionCancel
	m.done = true
	return tea.Quit
}

// updateActions updates available actions based on current state
func (m *SimpleReviewModel) updateActions() {
	if m.editing {
		// No actions shown during editing
		m.SetActions(nil)
	} else {
		m.SetActions([]Action{
			{Key: "A", Label: "ccept", Handler: m.accept},
			{Key: "E", Label: "dit", Handler: m.edit},
			{Key: "C", Label: "ancel", Handler: m.cancel},
		})
	}
}

// renderContent renders the main content
func (m *SimpleReviewModel) renderContent() string {
	if m.editing {
		return m.renderEditingContent()
	}
	return m.renderReviewContent()
}

// renderReviewContent renders the review mode content
func (m *SimpleReviewModel) renderReviewContent() string {
	var content strings.Builder

	// Parse and style the commit message
	lines := strings.Split(m.message, "\n")
	if len(lines) > 0 {
		// Style the subject line
		parts := strings.SplitN(lines[0], ":", 2)
		var subject string
		if len(parts) == 2 {
			subject = m.styles.CommitType.Render(parts[0]+":") + m.styles.CommitDesc.Render(parts[1])
		} else {
			subject = m.styles.CommitDesc.Render(lines[0])
		}
		content.WriteString(subject + "\n")
	}

	// Style the body
	if len(lines) > 1 {
		content.WriteString("\n")
		bodyText := strings.Join(lines[1:], "\n")
		wrappedBody := wordWrap(bodyText, CalculateContentWidth(m.width)-4)
		for _, line := range strings.Split(wrappedBody, "\n") {
			content.WriteString(m.styles.CommitBody.Render(line) + "\n")
		}
	}

	return strings.TrimRight(content.String(), "\n")
}

// renderEditingContent renders the editing mode content
func (m *SimpleReviewModel) renderEditingContent() string {
	promptStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Yellow)
	hintStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Italic(true)

	var content strings.Builder
	content.WriteString(promptStyle.Render("Edit Commit Message:") + "\n\n")
	content.WriteString(m.textArea.View() + "\n\n")
	content.WriteString(hintStyle.Render("[Ctrl+S] Save  [Esc] Cancel"))

	return content.String()
}

// IsDone returns whether the model is done and the decision
func (m *SimpleReviewModel) IsDone() (bool, Decision, string) {
	return m.done, m.decision, m.message
}