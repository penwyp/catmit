package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/internal/squash"
)

// SquashPhase represents the phase of the squash workflow
type SquashPhase int

const (
	SquashPhaseGenerating SquashPhase = iota
	SquashPhaseReviewing
	SquashPhaseDone
)

// SquashModel is the TUI model for the squash command
type SquashModel struct {
	BaseModel
	squash      *squash.Squash
	messages    []string
	result      string
	phase       SquashPhase
	spinner     spinner.Model
	copySuccess bool
	accepted    bool // Track if user accepted the result
}

// squashMsg is used to pass the generated result
type squashMsg struct {
	result string
	err    error
}

// NewSquashModel creates a new SquashModel
func NewSquashModel(s *squash.Squash, messages []string) *SquashModel {
	colors := DefaultColors()
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colors.HotPink)

	model := &SquashModel{
		BaseModel: NewBaseModel("Squashing Commit Messages", nil),
		squash:    s,
		messages:  messages,
		phase:     SquashPhaseGenerating,
		spinner:   sp,
	}

	// Set content renderer
	model.SetContentRenderer(model.renderContent)

	return model
}

// Run runs the TUI
func (m *SquashModel) Run() error {
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}

// Init initializes the model
func (m *SquashModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.generateCommitMessage(),
	)
}

// generateCommitMessage generates the consolidated commit message
func (m *SquashModel) generateCommitMessage() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := m.squash.Generate(ctx, m.messages)
		return squashMsg{result: result, err: err}
	}
}

// Update handles messages
func (m *SquashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.HandleWindowSize(msg)
		return m, nil

	case tea.KeyMsg:
		// Let BaseModel handle navigation in review phase
		if m.phase == SquashPhaseReviewing {
			cmd := m.HandleKeyboard(msg)
			if cmd != nil {
				return m, cmd
			}
		}
		return m, nil

	case squashMsg:
		if msg.err != nil {
			m.SetError(msg.err)
			m.phase = SquashPhaseReviewing
			// Update actions for error state
			m.updateActionsForPhase()
			return m, nil
		}
		m.result = msg.result
		m.phase = SquashPhaseReviewing
		// Try to copy to clipboard
		if err := clipboard.WriteAll(m.result); err == nil {
			m.copySuccess = true
		}
		// Update actions for review state
		m.updateActionsForPhase()
		return m, nil

	case spinner.TickMsg:
		if m.phase == SquashPhaseGenerating {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// updateActionsForPhase updates the available actions based on the current phase
func (m *SquashModel) updateActionsForPhase() {
	switch m.phase {
	case SquashPhaseReviewing:
		if m.GetError() != nil {
			// Error state actions
			m.SetActions([]Action{
				{Key: "R", Label: "etry", Handler: m.regenerate},
				{Key: "Q", Label: "uit", Handler: m.quit},
			})
		} else {
			// Normal review actions
			m.SetActions([]Action{
				{Key: "A", Label: "ccept", Handler: m.accept},
				{Key: "R", Label: "egenerate", Handler: m.regenerate},
				{Key: "E", Label: "dit", Handler: m.edit},
				{Key: "Q", Label: "uit", Handler: m.quit},
			})
		}
	default:
		// No actions during generation
		m.SetActions(nil)
	}
}

// Action handlers
func (m *SquashModel) accept() tea.Cmd {
	m.phase = SquashPhaseDone
	m.accepted = true
	return tea.Quit
}

func (m *SquashModel) regenerate() tea.Cmd {
	m.phase = SquashPhaseGenerating
	m.copySuccess = false
	m.SetError(nil)
	return tea.Batch(
		m.spinner.Tick,
		m.generateCommitMessage(),
	)
}

func (m *SquashModel) edit() tea.Cmd {
	// TODO: Implement edit functionality
	return func() tea.Msg {
		fmt.Fprintln(os.Stderr, "Edit feature not yet implemented")
		return nil
	}
}

func (m *SquashModel) quit() tea.Cmd {
	return tea.Quit
}

// renderContent renders the main content based on the current phase
func (m *SquashModel) renderContent() string {
	switch m.phase {
	case SquashPhaseGenerating:
		return m.renderGenerating()
	case SquashPhaseReviewing:
		return m.renderReviewing()
	default:
		return ""
	}
}

// View renders the view
func (m *SquashModel) View() string {
	if m.phase == SquashPhaseDone {
		return ""
	}
	// Update title based on phase
	switch m.phase {
	case SquashPhaseGenerating:
		m.SetTitle("Squashing Commit Messages")
	case SquashPhaseReviewing:
		if m.GetError() != nil {
			m.SetTitle("Error")
		} else {
			m.SetTitle("Generated commit message:")
		}
	}
	return m.BaseModel.View()
}

// renderGenerating renders the content during generation
func (m *SquashModel) renderGenerating() string {
	var content []string
	content = append(content,
		m.spinner.View()+" Generating consolidated commit message...",
		"",
		fmt.Sprintf("Processing %d commit messages", len(m.messages)),
	)
	return strings.Join(content, "\n")
}

// renderReviewing renders the review content
func (m *SquashModel) renderReviewing() string {
	colors := DefaultColors()
	resultStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.DarkGray).
		Padding(1, 2).
		Width(min(80, m.width-4))

	successStyle := lipgloss.NewStyle().
		Foreground(colors.BrightGreen)

	var content []string

	// Show the result
	content = append(content, resultStyle.Render(m.result), "")

	// Show copy success message
	if m.copySuccess {
		content = append(content, successStyle.Render("✅ Copied to clipboard!"), "")
	}

	return strings.Join(content, "\n")
}

// IsAccepted returns whether the user accepted the result
func (m *SquashModel) IsAccepted() bool {
	return m.accepted
}

// GetResult returns the generated commit message
func (m *SquashModel) GetResult() string {
	return m.result
}

// IsCopySuccess returns whether the message was copied to clipboard
func (m *SquashModel) IsCopySuccess() bool {
	return m.copySuccess
}
