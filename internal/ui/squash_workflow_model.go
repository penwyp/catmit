package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/internal/squash"
)

// SquashWorkflowModel handles the squash-draft workflow
type SquashWorkflowModel struct {
	*BaseWorkflowModel

	// Squash-specific fields
	messages    []string
	squash      *squash.Squash
	copySuccess bool
	accepted    bool
}

// NewSquashWorkflowModel creates a new squash workflow model
func NewSquashWorkflowModel(
	ctx context.Context,
	squash *squash.Squash,
	messages []string,
) *SquashWorkflowModel {
	base := NewBaseWorkflowModel(
		"Squashing Commit Messages",
		ctx,
		nil, // no collector needed
		nil, // no client needed (squash has its own)
		nil, // no committer needed
		"",  // no language preference
		60*time.Second,
	)

	m := &SquashWorkflowModel{
		BaseWorkflowModel: base,
		messages:          messages,
		squash:            squash,
	}

	// Override content renderer
	base.SetContentRenderer(m.renderContent)

	// Adjust for squash workflow
	base.textArea.Placeholder = "Edit squashed message..."
	base.textArea.CharLimit = 1000

	return m
}

// Init starts the first phase
func (m *SquashWorkflowModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.generateSquashMessage())
}

// Update handles messages
func (m *SquashWorkflowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.HandleWindowSize(msg)
		return m, nil

	case tea.KeyMsg:
		// Handle global shortcuts
		if handled, cmd := m.HandleGlobalKeys(msg); handled {
			return m, cmd
		}

		// Handle phase-specific keyboard input
		if m.phase == WorkflowPhaseReview {
			if cmd := m.BaseWorkflowModel.updateReview(msg); cmd != nil {
				return m, cmd
			}
		}

	case spinner.TickMsg:
		return m, m.UpdateSpinner(msg)

	case squashGeneratedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = WorkflowPhaseReview
			return m, nil
		}
		m.message = msg.result
		m.phase = WorkflowPhaseReview
		m.textArea.SetValue(m.message)
		
		// Try to copy to clipboard
		if err := clipboard.WriteAll(m.message); err == nil {
			m.copySuccess = true
		}
		return m, nil

	case errorMsg:
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	}

	// Handle textarea updates in editing mode
	if m.editing && m.phase == WorkflowPhaseReview {
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the UI
func (m *SquashWorkflowModel) View() string {
	// Update title based on phase
	title := m.getPhaseTitle()
	m.SetTitle(title)

	// Update actions based on phase
	m.updateActionsForPhase()

	return m.BaseModel.View()
}

// updateActionsForPhase updates available actions
func (m *SquashWorkflowModel) updateActionsForPhase() {
	switch m.phase {
	case WorkflowPhaseLoading:
		m.SetActions(nil)
	case WorkflowPhaseReview:
		if m.editing {
			m.SetActions(nil)
		} else {
			if m.err != nil {
				// Error state actions
				m.SetActions([]Action{
					{Key: "R", Label: "etry", Handler: m.handleRegenerate},
					{Key: "Q", Label: "uit", Handler: m.handleQuit},
				})
			} else {
				// Normal review actions
				m.SetActions([]Action{
					{Key: "A", Label: "ccept", Handler: m.handleAccept},
					{Key: "E", Label: "dit", Handler: m.handleEdit},
					{Key: "R", Label: "egenerate", Handler: m.handleRegenerate},
					{Key: "Q", Label: "uit", Handler: m.handleQuit},
				})
			}
		}
	default:
		m.SetActions(nil)
	}
}

// Action handlers
func (m *SquashWorkflowModel) handleAccept() tea.Cmd {
	m.accepted = true
	m.reviewDecision = DecisionAccept
	m.done = true
	return tea.Quit
}

func (m *SquashWorkflowModel) handleRegenerate() tea.Cmd {
	m.phase = WorkflowPhaseLoading
	m.loadingStage = StageQuery
	m.copySuccess = false
	m.err = nil
	return m.generateSquashMessage()
}

func (m *SquashWorkflowModel) handleQuit() tea.Cmd {
	m.reviewDecision = DecisionCancel
	m.done = true
	return tea.Quit
}

// Phase-specific rendering
func (m *SquashWorkflowModel) renderContent() string {
	switch m.phase {
	case WorkflowPhaseLoading:
		return m.renderLoadingContent()
	case WorkflowPhaseReview:
		if m.editing {
			return m.renderEditingContent()
		}
		return m.renderReviewContent()
	default:
		return ""
	}
}

// Override renderLoadingContent for squash-specific messages
func (m *SquashWorkflowModel) renderLoadingContent() string {
	colors := DefaultColors()
	progressStyle := lipgloss.NewStyle().Foreground(colors.Green)
	
	var content strings.Builder
	content.WriteString(m.spinner.View() + " " + progressStyle.Render("Generating consolidated commit message..."))
	content.WriteString("\n\n")
	content.WriteString(fmt.Sprintf("Processing %d commit messages", len(m.messages)))
	
	return content.String()
}

// Override renderReviewContent to show copy success
func (m *SquashWorkflowModel) renderReviewContent() string {
	// First render the base review content
	baseContent := m.BaseWorkflowModel.renderReviewContent()
	
	if m.copySuccess {
		colors := DefaultColors()
		successStyle := lipgloss.NewStyle().Foreground(colors.BrightGreen)
		return baseContent + "\n\n" + successStyle.Render("✅ Copied to clipboard!")
	}
	
	return baseContent
}

// getPhaseTitle returns the title for the current phase
func (m *SquashWorkflowModel) getPhaseTitle() string {
	switch m.phase {
	case WorkflowPhaseLoading:
		return "Squashing Commit Messages"
	case WorkflowPhaseReview:
		if m.editing {
			return "Edit Message"
		}
		if m.err != nil {
			return "Error"
		}
		return "Generated commit message:"
	default:
		return "Squash Draft"
	}
}

// generateSquashMessage generates the consolidated commit message
func (m *SquashWorkflowModel) generateSquashMessage() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, m.apiTimeout)
		defer cancel()

		result, err := m.squash.Generate(ctx, m.messages)
		return squashGeneratedMsg{result: result, err: err}
	}
}

// Message types
type squashGeneratedMsg struct {
	result string
	err    error
}

// Public getters for external use

// IsAccepted returns whether the user accepted the result
func (m *SquashWorkflowModel) IsAccepted() bool {
	return m.accepted
}

// GetResult returns the generated commit message
func (m *SquashWorkflowModel) GetResult() string {
	return m.message
}

// IsCopySuccess returns whether the message was copied to clipboard
func (m *SquashWorkflowModel) IsCopySuccess() bool {
	return m.copySuccess
}