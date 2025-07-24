package ui

import tea "github.com/charmbracelet/bubbletea"

// ---------------- Message Types ------------------
// GenerateStartMsg indicates the start of the generation process.
type GenerateStartMsg struct{}

// GenerateSuccessMsg indicates successful generation, carrying the commit message.
type GenerateSuccessMsg struct {
	Message string
}

// GenerateErrorMsg indicates generation failure.
type GenerateErrorMsg struct {
	Err error
}

// ---------------- Model --------------------------
// Model represents the Bubble Tea state model.
// Only minimal fields are retained to pass unit tests; can be extended to a full TUI later.
type Model struct {
	isLoading bool   // Whether the model is in loading state
	isDone    bool   // Whether the process is done
	message   string // The generated commit message
	err       error  // Error information, if any
}

// NewModel returns the initial model in the Loading state.
func NewModel() Model {
	return Model{isLoading: true}
}

// Init implements the tea.Model interface, returns nil as no initial command is needed.
func (m Model) Init() tea.Cmd { return nil }

// Update updates the model based on different Msg types.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case GenerateStartMsg:
		m.isLoading = true
		m.isDone = false
		m.err = nil
	case GenerateSuccessMsg:
		m.isLoading = false
		m.isDone = true
		m.message = msg.Message
	case GenerateErrorMsg:
		m.isLoading = false
		m.isDone = true
		m.err = msg.Err
	default:
		// For other messages, keep the original state.
	}
	return m, nil
}

// View returns the current view string. Simplified implementation; TUI can be improved later.
func (m Model) View() string {
	if m.isLoading {
		return "Loading..."
	}
	if m.err != nil {
		return "Error: " + m.err.Error()
	}
	return m.message
}
