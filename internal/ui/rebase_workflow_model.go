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
	"github.com/penwyp/catmit/internal/rebase"
	"github.com/penwyp/catmit/pkg/githistory"
)

// rebaseWorkflowInterface defines the interface for rebase workflow operations
type rebaseWorkflowInterface interface {
	Analyze(ctx context.Context) (*rebase.AnalysisResult, error)
	GenerateCommitMessage(ctx context.Context, commits []githistory.Commit) (string, error)
	ExecuteRebase(ctx context.Context, analysis *rebase.AnalysisResult, message string) error
}

// RebaseWorkflowModel handles the squash-history workflow
type RebaseWorkflowModel struct {
	*BaseWorkflowModel

	// Rebase-specific fields
	workflow     rebaseWorkflowInterface
	analysis     *rebase.AnalysisResult
	backupBranch string
	
	// State tracking
	accepted     bool
	copySuccess  bool
	
	// Custom phase for analysis confirmation
	needsAnalysisConfirmation bool
}

// NewRebaseWorkflowModel creates a new rebase workflow model
func NewRebaseWorkflowModel(
	ctx context.Context,
	workflow rebaseWorkflowInterface,
) *RebaseWorkflowModel {
	base := NewBaseWorkflowModel(
		"Analyzing Repository",
		ctx,
		nil, // no collector needed
		nil, // no client needed (workflow has its own)
		nil, // no committer needed
		"",  // no language preference
		60*time.Second,
	)

	m := &RebaseWorkflowModel{
		BaseWorkflowModel: base,
		workflow:          workflow,
	}

	// Override content renderer
	base.SetContentRenderer(m.renderContent)

	// Adjust for rebase workflow
	base.textArea.Placeholder = "Edit squashed commit message..."
	base.textArea.CharLimit = 1000

	return m
}

// Init starts the first phase
func (m *RebaseWorkflowModel) Init() tea.Cmd {
	m.loadingStage = StageCollect // Analyzing repository
	return tea.Batch(m.spinner.Tick, m.analyzeRepository())
}

// Update handles messages
func (m *RebaseWorkflowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.HandleWindowSize(msg)
		return m, nil

	case tea.KeyMsg:
		// Handle global shortcuts
		if handled, cmd := m.HandleGlobalKeys(msg); handled {
			return m, cmd
		}

		// Handle analysis confirmation
		if m.needsAnalysisConfirmation {
			switch msg.String() {
			case "y", "Y":
				m.needsAnalysisConfirmation = false
				m.phase = WorkflowPhaseLoading
				m.loadingStage = StageQuery
				return m, m.generateRebaseMessage()
			case "n", "N", "q", "ctrl+c":
				m.done = true
				return m, tea.Quit
			}
			return m, nil
		}

		// Handle phase-specific keyboard input
		if m.phase == WorkflowPhaseReview {
			if cmd := m.BaseWorkflowModel.updateReview(msg); cmd != nil {
				return m, cmd
			}
		}

	case spinner.TickMsg:
		return m, m.UpdateSpinner(msg)

	case rebaseAnalysisMsg:
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}
		m.analysis = msg.result
		if !m.analysis.CanRebase {
			// Display the message to the user before exiting
			fmt.Println(m.analysis.Message)
			m.done = true
			return m, tea.Quit
		}
		// Need confirmation before proceeding
		m.needsAnalysisConfirmation = true
		m.phase = WorkflowPhaseReview // Reuse review phase for confirmation
		return m, nil

	case rebaseGeneratedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = WorkflowPhaseReview
			return m, nil
		}
		m.message = msg.message
		m.phase = WorkflowPhaseReview
		m.textArea.SetValue(m.message)
		
		// Try to copy to clipboard
		if err := clipboard.WriteAll(m.message); err == nil {
			m.copySuccess = true
		}
		return m, nil

	case rebaseExecutedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.commitStage = CommitStagePushFailed // Reuse for error display
		} else {
			m.backupBranch = msg.backupBranch
			m.commitStage = CommitStageDone
		}
		m.finalStartTime = time.Now()
		return m, tea.Tick(m.showDuration*2, func(time.Time) tea.Msg {
			return finalTimeoutMsg{}
		})

	case finalTimeoutMsg:
		m.done = true
		return m, tea.Quit

	case errorMsg:
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	}

	// Handle textarea updates in editing mode
	if m.editing && m.phase == WorkflowPhaseReview && !m.needsAnalysisConfirmation {
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the UI
func (m *RebaseWorkflowModel) View() string {
	// Update title based on phase
	title := m.getPhaseTitle()
	m.SetTitle(title)

	// Update actions based on phase
	m.updateActionsForPhase()

	return m.BaseModel.View()
}

// updateActionsForPhase updates available actions
func (m *RebaseWorkflowModel) updateActionsForPhase() {
	if m.needsAnalysisConfirmation {
		m.SetActions(nil) // No button actions during confirmation
		return
	}

	switch m.phase {
	case WorkflowPhaseLoading:
		m.SetActions(nil)
	case WorkflowPhaseReview:
		if m.editing {
			m.SetActions(nil)
		} else {
			m.SetActions([]Action{
				{Key: "A", Label: "ccept", Handler: m.handleAccept},
				{Key: "E", Label: "dit", Handler: m.handleEdit},
				{Key: "R", Label: "egenerate", Handler: m.handleRegenerate},
				{Key: "C", Label: "ancel", Handler: m.handleCancel},
			})
		}
	case WorkflowPhaseCommit:
		m.SetActions(nil)
	default:
		m.SetActions(nil)
	}
}

// Action handlers
func (m *RebaseWorkflowModel) handleAccept() tea.Cmd {
	m.accepted = true
	m.reviewDecision = DecisionAccept
	m.phase = WorkflowPhaseCommit
	m.commitStage = CommitStageCommitting
	return m.executeRebase()
}

func (m *RebaseWorkflowModel) handleRegenerate() tea.Cmd {
	m.phase = WorkflowPhaseLoading
	m.loadingStage = StageQuery
	m.copySuccess = false
	return m.generateRebaseMessage()
}

// Phase-specific rendering
func (m *RebaseWorkflowModel) renderContent() string {
	if m.needsAnalysisConfirmation {
		return m.renderAnalysisConfirmation()
	}

	switch m.phase {
	case WorkflowPhaseLoading:
		return m.renderLoadingContent()
	case WorkflowPhaseReview:
		if m.editing {
			return m.renderEditingContent()
		}
		return m.renderReviewContent()
	case WorkflowPhaseCommit:
		return m.renderExecutionContent()
	default:
		return ""
	}
}

// renderAnalysisConfirmation renders the analysis confirmation screen
func (m *RebaseWorkflowModel) renderAnalysisConfirmation() string {
	colors := DefaultColors()
	infoStyle := lipgloss.NewStyle().Foreground(colors.Cyan)
	normalStyle := lipgloss.NewStyle()
	dimStyle := lipgloss.NewStyle().Foreground(colors.DarkGray)
	helpStyle := lipgloss.NewStyle().Foreground(colors.LightGray)

	var content strings.Builder
	
	content.WriteString(infoStyle.Render(fmt.Sprintf("Branch: %s → %s", m.analysis.CurrentBranch, m.analysis.BaseBranch)) + "\n")
	content.WriteString(infoStyle.Render(fmt.Sprintf("Commits to squash: %d", len(m.analysis.UnpushedCommits))) + "\n\n")
	
	content.WriteString(normalStyle.Render("The following commits will be squashed:") + "\n")
	content.WriteString(dimStyle.Render(rebase.FormatCommitList(m.analysis.UnpushedCommits)) + "\n\n")
	
	content.WriteString(helpStyle.Render("Continue? (y/n): "))
	
	return content.String()
}

// renderExecutionContent renders the rebase execution progress
func (m *RebaseWorkflowModel) renderExecutionContent() string {
	colors := DefaultColors()
	successStyle := lipgloss.NewStyle().Foreground(colors.Green)
	errorStyle := lipgloss.NewStyle().Foreground(colors.Red)
	progressStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	infoStyle := lipgloss.NewStyle().Foreground(colors.Cyan)
	normalStyle := lipgloss.NewStyle()

	var content strings.Builder

	switch m.commitStage {
	case CommitStageCommitting:
		content.WriteString(m.spinner.View() + " " + progressStyle.Render("Executing rebase..."))
		content.WriteString("\n" + m.spinner.View() + " Creating backup branch")
		content.WriteString("\n" + m.spinner.View() + " Performing interactive rebase")
		
	case CommitStageDone:
		content.WriteString(successStyle.Render("✅ Rebase completed successfully!") + "\n\n")
		content.WriteString(infoStyle.Render(fmt.Sprintf("Backup branch: %s", m.backupBranch)) + "\n\n")
		content.WriteString(normalStyle.Render(rebase.GetRecoveryInstructions(m.backupBranch)))
		
	case CommitStagePushFailed: // Reusing for error state
		content.WriteString(errorStyle.Render("❌ Error: "+m.err.Error()) + "\n\n")
		if m.backupBranch != "" {
			content.WriteString(normalStyle.Render(rebase.GetRecoveryInstructions(m.backupBranch)) + "\n")
		}
	}

	return content.String()
}

// Override renderReviewContent to show copy success
func (m *RebaseWorkflowModel) renderReviewContent() string {
	// First render the base review content
	baseContent := m.BaseWorkflowModel.renderReviewContent()
	
	if m.copySuccess {
		colors := DefaultColors()
		successStyle := lipgloss.NewStyle().Foreground(colors.BrightGreen)
		return baseContent + "\n\n" + successStyle.Render("✓ Copied to clipboard")
	}
	
	return baseContent
}

// getPhaseTitle returns the title for the current phase
func (m *RebaseWorkflowModel) getPhaseTitle() string {
	if m.needsAnalysisConfirmation {
		return "📋 Commits to Squash"
	}

	switch m.phase {
	case WorkflowPhaseLoading:
		switch m.loadingStage {
		case StageCollect:
			return "🔍 Analyzing repository state..."
		case StageQuery:
			return "🤖 Generating commit message..."
		default:
			return "Processing..."
		}
	case WorkflowPhaseReview:
		if m.editing {
			return "📝 Edit Message"
		}
		return "📝 Generated Commit Message"
	case WorkflowPhaseCommit:
		if m.commitStage == CommitStageDone {
			return "✅ Rebase Complete"
		}
		return "🔄 Executing rebase..."
	default:
		return "Squash History"
	}
}

// Commands for async operations
func (m *RebaseWorkflowModel) analyzeRepository() tea.Cmd {
	return func() tea.Msg {
		result, err := m.workflow.Analyze(m.ctx)
		return rebaseAnalysisMsg{result: result, err: err}
	}
}

func (m *RebaseWorkflowModel) generateRebaseMessage() tea.Cmd {
	return func() tea.Msg {
		message, err := m.workflow.GenerateCommitMessage(m.ctx, m.analysis.UnpushedCommits)
		return rebaseGeneratedMsg{message: message, err: err}
	}
}

func (m *RebaseWorkflowModel) executeRebase() tea.Cmd {
	return func() tea.Msg {
		err := m.workflow.ExecuteRebase(m.ctx, m.analysis, m.message)
		
		// Extract backup branch name
		backupBranch := ""
		if m.analysis != nil {
			backupBranch = fmt.Sprintf("%s_bak", m.analysis.CurrentBranch)
		}
		
		return rebaseExecutedMsg{backupBranch: backupBranch, err: err}
	}
}

// Message types
type rebaseAnalysisMsg struct {
	result *rebase.AnalysisResult
	err    error
}

type rebaseGeneratedMsg struct {
	message string
	err     error
}

type rebaseExecutedMsg struct {
	backupBranch string
	err          error
}

// Public getters

// IsAccepted returns whether the user accepted the result
func (m *RebaseWorkflowModel) IsAccepted() bool {
	return m.accepted
}

// GetResult returns the generated commit message
func (m *RebaseWorkflowModel) GetResult() string {
	return m.message
}

// IsCopySuccess returns whether the message was copied to clipboard
func (m *RebaseWorkflowModel) IsCopySuccess() bool {
	return m.copySuccess
}

// GetBackupBranch returns the backup branch name
func (m *RebaseWorkflowModel) GetBackupBranch() string {
	return m.backupBranch
}