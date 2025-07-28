package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WorkflowPhase represents the current phase of the workflow
type WorkflowPhase int

const (
	WorkflowPhaseLoading WorkflowPhase = iota
	WorkflowPhaseReview
	WorkflowPhasePRPreview
	WorkflowPhaseCommit
	WorkflowPhaseDone
)

// BaseWorkflowModel contains common functionality for all workflow models
type BaseWorkflowModel struct {
	BaseModel

	// State management
	phase          WorkflowPhase
	loadingStage   Stage
	reviewDecision Decision
	commitStage    CommitStage

	// UI components
	spinner  spinner.Model
	textArea textarea.Model
	editing  bool

	// Data
	message string
	lang    string

	// Common dependencies
	ctx        context.Context
	collector  collectorInterface
	client     clientInterface
	committer  commitInterface
	apiTimeout time.Duration

	// Internal state
	finalStartTime time.Time
	showDuration   time.Duration

	// Error handling
	err  error
	done bool
}

// NewBaseWorkflowModel creates a new base workflow model
func NewBaseWorkflowModel(
	title string,
	ctx context.Context,
	collector collectorInterface,
	client clientInterface,
	committer commitInterface,
	lang string,
	apiTimeout time.Duration,
) *BaseWorkflowModel {
	sp := spinner.New()
	sp.Spinner = spinner.Line

	ta := textarea.New()
	ta.Placeholder = "Edit message..."
	ta.CharLimit = 1000
	ta.ShowLineNumbers = false

	m := &BaseWorkflowModel{
		BaseModel:    NewBaseModel(title, nil),
		phase:        WorkflowPhaseLoading,
		loadingStage: StageCollect,
		spinner:      sp,
		textArea:     ta,
		ctx:          ctx,
		collector:    collector,
		client:       client,
		committer:    committer,
		lang:         lang,
		apiTimeout:   apiTimeout,
		showDuration: 1500 * time.Millisecond,
	}

	// Set content renderer
	m.SetContentRenderer(m.renderContent)

	return m
}

// Common message handlers

// HandleWindowSize handles terminal resize
func (m *BaseWorkflowModel) HandleWindowSize(msg tea.WindowSizeMsg) {
	m.BaseModel.HandleWindowSize(msg)
	m.textArea.SetWidth(CalculateContentWidth(m.width) - 4)
	m.textArea.SetHeight(8)
}

// HandleGlobalKeys handles global keyboard shortcuts
func (m *BaseWorkflowModel) HandleGlobalKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.err = context.Canceled
		m.done = true
		return true, tea.Quit
	}
	return false, nil
}

// UpdateSpinner updates the spinner animation
func (m *BaseWorkflowModel) UpdateSpinner(msg spinner.TickMsg) tea.Cmd {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return cmd
}

// View renders the unified interface
func (m *BaseWorkflowModel) View() string {
	// Update title based on phase and lang
	title := m.getPhaseTitle()
	if m.lang != "" {
		title += " (" + m.lang + ")"
	}
	m.SetTitle(title)

	// Update actions based on phase
	m.updateActionsForPhase()

	return m.BaseModel.View()
}

// Common rendering methods

// renderContent renders the main content based on current phase
func (m *BaseWorkflowModel) renderContent() string {
	switch m.phase {
	case WorkflowPhaseLoading:
		return m.renderLoadingContent()
	case WorkflowPhaseReview:
		if m.editing {
			return m.renderEditingContent()
		}
		return m.renderReviewContent()
	case WorkflowPhasePRPreview:
		return m.renderPRPreviewContent()
	case WorkflowPhaseCommit:
		return m.renderCommitContent()
	default:
		return ""
	}
}

// renderLoadingContent renders the content during the loading phase
func (m *BaseWorkflowModel) renderLoadingContent() string {
	colors := DefaultColors()
	var statusStyle lipgloss.Style
	var status string

	switch m.loadingStage {
	case StagePRCheck:
		status = "Checking if PR already exists…"
		statusStyle = lipgloss.NewStyle().Foreground(colors.Cyan)
	case StageCollect:
		status = "Collecting diff…"
		statusStyle = lipgloss.NewStyle().Foreground(colors.DarkOrange)
	case StagePreprocess:
		status = "Preprocessing files…"
		statusStyle = lipgloss.NewStyle().Foreground(colors.Orange)
	case StagePrompt:
		status = "Crafting prompt…"
		statusStyle = lipgloss.NewStyle().Foreground(colors.Blue)
	case StageQuery:
		status = "Generating message…"
		statusStyle = lipgloss.NewStyle().Foreground(colors.Green)
	default:
		status = "Processing…"
		statusStyle = lipgloss.NewStyle().Foreground(colors.Gray)
	}

	return m.spinner.View() + " " + statusStyle.Render(status)
}

// renderReviewContent renders the content during the review phase
func (m *BaseWorkflowModel) renderReviewContent() string {
	colors := DefaultColors()
	commitTypeStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	commitDescStyle := lipgloss.NewStyle().Foreground(colors.White)
	commitBodyStyle := lipgloss.NewStyle().Foreground(colors.Gray)

	var content strings.Builder

	// Render message
	lines := strings.Split(m.message, "\n")
	if len(lines) > 0 {
		parts := strings.SplitN(lines[0], ":", 2)
		var subject string
		if len(parts) == 2 {
			subject = commitTypeStyle.Render(parts[0]+":") + commitDescStyle.Render(parts[1])
		} else {
			subject = commitDescStyle.Render(lines[0])
		}
		content.WriteString(subject + "\n")
	}

	if len(lines) > 1 {
		content.WriteString("\n")
		bodyText := strings.Join(lines[1:], "\n")
		wrappedBody := wordWrap(bodyText, CalculateContentWidth(m.width)-2)
		for _, l := range strings.Split(wrappedBody, "\n") {
			content.WriteString(commitBodyStyle.Render(l) + "\n")
		}
	}

	return strings.TrimRight(content.String(), "\n")
}

// renderEditingContent renders the content in editing mode
func (m *BaseWorkflowModel) renderEditingContent() string {
	colors := DefaultColors()
	promptStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	hintStyle := lipgloss.NewStyle().Foreground(colors.Gray).Italic(true)

	var content strings.Builder
	content.WriteString(promptStyle.Render("Edit Message:") + "\n\n")

	// Render each line of the textarea
	lines := strings.Split(m.textArea.View(), "\n")
	for _, line := range lines {
		content.WriteString(line + "\n")
	}

	content.WriteString("\n" + hintStyle.Render("[Ctrl+S] Save  [Esc] Cancel"))

	return strings.TrimRight(content.String(), "\n")
}

// Common action handlers

// handleEdit handles the edit action
func (m *BaseWorkflowModel) handleEdit() tea.Cmd {
	m.editing = true
	m.textArea.Focus()
	m.updateActionsForPhase()
	return textarea.Blink
}

// handleCancel handles the cancel action
func (m *BaseWorkflowModel) handleCancel() tea.Cmd {
	m.reviewDecision = DecisionCancel
	m.done = true
	return tea.Quit
}

// updateReview handles keyboard input during the Review phase
// This is a helper method that derived models can use in their Update method
func (m *BaseWorkflowModel) updateReview(msg tea.KeyMsg) tea.Cmd {
	if m.editing {
		switch msg.String() {
		case "esc":
			m.editing = false
			m.textArea.Blur()
			m.updateActionsForPhase()
			return nil
		case "ctrl+s":
			m.message = strings.TrimSpace(m.textArea.Value())
			m.editing = false
			m.textArea.Blur()
			m.updateActionsForPhase()
			return nil
		default:
			var cmd tea.Cmd
			m.textArea, cmd = m.textArea.Update(msg)
			return cmd
		}
	}

	// Let BaseModel handle navigation and action execution
	return m.HandleKeyboard(msg)
}

// State getters

// IsDone returns whether the operation is done and related info
func (m *BaseWorkflowModel) IsDone() (bool, Decision, string, error) {
	return m.done, m.reviewDecision, m.message, m.err
}

// GetError returns the error info
func (m *BaseWorkflowModel) GetError() error {
	if m.err == context.Canceled {
		return nil
	}
	// If commit succeeded but push failed, do not return error (main operation succeeded)
	// Push failure has already been shown in TUI, no need to output to terminal again
	if m.commitStage == CommitStagePushFailed {
		return nil
	}
	return m.err
}

// These methods must be implemented by derived models

// Init and Update must be implemented by derived models to satisfy tea.Model interface
// Init() tea.Cmd
// Update(tea.Msg) (tea.Model, tea.Cmd)

// getPhaseTitle gets the title for the current phase
func (m *BaseWorkflowModel) getPhaseTitle() string {
	// Default implementation, can be overridden
	switch m.phase {
	case WorkflowPhaseLoading:
		return "Generating Message"
	case WorkflowPhaseReview:
		if m.editing {
			return "Edit Message"
		}
		return "Review Message"
	case WorkflowPhasePRPreview:
		return "Pull Request Preview"
	case WorkflowPhaseCommit:
		return "Progress"
	default:
		return "Catmit"
	}
}

// updateActionsForPhase updates available actions based on current phase
func (m *BaseWorkflowModel) updateActionsForPhase() {
	// Default implementation, should be overridden by derived models
	switch m.phase {
	case WorkflowPhaseLoading:
		m.SetActions(nil)
	case WorkflowPhaseReview:
		if m.editing {
			m.SetActions(nil)
		} else {
			// Default actions, should be customized by derived models
			m.SetActions([]Action{
				{Key: "E", Label: "dit", Handler: m.handleEdit},
				{Key: "C", Label: "ancel", Handler: m.handleCancel},
			})
		}
	default:
		m.SetActions(nil)
	}
}

// Placeholder methods for phases that may not be used by all workflows

// renderPRPreviewContent renders PR preview (may be overridden)
func (m *BaseWorkflowModel) renderPRPreviewContent() string {
	return "PR Preview not implemented"
}

// renderCommitContent renders commit progress (may be overridden)
func (m *BaseWorkflowModel) renderCommitContent() string {
	return "Commit progress not implemented"
}