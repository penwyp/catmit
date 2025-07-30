package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/pkg/gitinfo"
)

// CommitWorkflowModel handles the commit generation and submission workflow
type CommitWorkflowModel struct {
	*BaseWorkflowModel

	// Commit-specific data
	seed        string
	promptBuild promptInterface

	// Configurations
	enablePush bool
	stageAll   bool
	createPR   bool
	prRemote   string
	prBase     string
	prDraft    bool
	prProvider string

	// PR preview related
	prPreview     *PRPreviewModel
	prPreviewData PRPreviewData
	prURL         string

	// Template related
	useTemplate bool
}

// NewCommitWorkflowModel creates a new commit workflow model
func NewCommitWorkflowModel(
	ctx context.Context,
	col collectorInterface,
	pb promptInterface,
	cli clientInterface,
	com commitInterface,
	seed, lang string,
	apiTimeout time.Duration,
	enablePush, stageAll bool,
	prConfig PRConfig,
) *CommitWorkflowModel {
	base := NewBaseWorkflowModel(
		"Generating Message",
		ctx,
		col,
		cli,
		com,
		lang,
		apiTimeout,
	)

	m := &CommitWorkflowModel{
		BaseWorkflowModel: base,
		seed:              seed,
		promptBuild:       pb,
		enablePush:        enablePush,
		stageAll:          stageAll,
		createPR:          prConfig.CreatePR,
		prRemote:          prConfig.Remote,
		prBase:            prConfig.Base,
		prDraft:           prConfig.Draft,
		prProvider:        prConfig.Provider,
		useTemplate:       prConfig.UseTemplate,
	}

	// Override content renderer to use commit-specific rendering
	base.SetContentRenderer(m.renderContent)

	return m
}

// Init starts the first phase
func (m *CommitWorkflowModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, collectCmd(m.collector, m.ctx))
}

// Update handles messages
func (m *CommitWorkflowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		switch m.phase {
		case WorkflowPhaseReview:
			if cmd := m.BaseWorkflowModel.updateReview(msg); cmd != nil {
				return m, cmd
			}
		case WorkflowPhasePRPreview:
			return m.updatePRPreview(msg)
		}

	case spinner.TickMsg:
		return m, m.UpdateSpinner(msg)

	// Loading phase message handling
	case diffCollectedMsg:
		m.loadingStage = StagePreprocess
		return m, preprocessCmd(m.collector, m.ctx)

	case preprocessDoneMsg:
		m.loadingStage = StagePrompt
		return m, buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)

	case smartPromptBuiltMsg:
		m.loadingStage = StageQuery
		return m, queryCmd(m.client, m.ctx, msg.systemPrompt, msg.userPrompt, m.apiTimeout)

	case queryDoneMsg:
		m.message = strings.TrimSpace(strings.ReplaceAll(msg.message, "\r", ""))
		m.phase = WorkflowPhaseReview
		m.textArea.SetValue(m.message)
		return m, nil

	// Commit phase message handling
	case commitDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}
		m.commitStage = CommitStageCommitted
		if m.enablePush {
			return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
				return delayedPushMsg{}
			})
		} else {
			if m.createPR {
				return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
					return delayedCreatePRMsg{}
				})
			} else {
				m.commitStage = CommitStageDone
				m.finalStartTime = time.Now()
				return m, tea.Tick(m.showDuration, func(time.Time) tea.Msg {
					return finalTimeoutMsg{}
				})
			}
		}

	case pushDoneMsg:
		if msg.err != nil {
			m.commitStage = CommitStagePushFailed
			m.err = msg.err
			m.finalStartTime = time.Now()
			return m, tea.Tick(m.showDuration*2, func(time.Time) tea.Msg {
				return finalTimeoutMsg{}
			})
		}
		// Push succeeded
		m.commitStage = CommitStagePushed
		if m.createPR {
			return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
				return delayedCreatePRMsg{}
			})
		} else {
			m.commitStage = CommitStageDone
			m.finalStartTime = time.Now()
			return m, tea.Tick(m.showDuration, func(time.Time) tea.Msg {
				return finalTimeoutMsg{}
			})
		}

	case finalTimeoutMsg:
		m.done = true
		return m, tea.Quit

	case delayedPushMsg:
		m.commitStage = CommitStagePushing
		return m, m.startPush()

	case delayedCreatePRMsg:
		m.commitStage = CommitStageCreatingPR
		return m, m.startCreatePR()

	case createPRDoneMsg:
		if msg.err != nil {
			// Check if PR already exists
			var prExists *pr.ErrPRAlreadyExists
			if errors.As(msg.err, &prExists) {
				// Treat existing PR as success
				m.commitStage = CommitStagePRCreated
				m.prURL = prExists.URL
				m.finalStartTime = time.Now()
				return m, tea.Tick(m.showDuration, func(time.Time) tea.Msg {
					return finalTimeoutMsg{}
				})
			}
			// Other errors
			m.commitStage = CommitStagePRFailed
			m.err = msg.err
			m.finalStartTime = time.Now()
			return m, tea.Tick(m.showDuration*2, func(time.Time) tea.Msg {
				return finalTimeoutMsg{}
			})
		}
		m.commitStage = CommitStagePRCreated
		m.prURL = msg.prURL
		m.finalStartTime = time.Now()
		return m, tea.Tick(m.showDuration, func(time.Time) tea.Msg {
			return finalTimeoutMsg{}
		})

	case startCommitPhaseMsg:
		m.phase = WorkflowPhaseCommit
		m.commitStage = CommitStageCommitting
		return m, m.startCommit()

	case prPreviewReadyMsg:
		m.prPreviewData = msg.data
		m.prPreview = NewPRPreviewModel(msg.data, DefaultStyles(), CalculateContentWidth(m.width))
		m.phase = WorkflowPhasePRPreview
		return m, nil

	case errorMsg:
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	}

	// Handle textarea updates in editing mode during Review phase
	if m.editing && m.phase == WorkflowPhaseReview {
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the unified interface
func (m *CommitWorkflowModel) View() string {
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

// updateActionsForPhase updates available actions based on current phase
func (m *CommitWorkflowModel) updateActionsForPhase() {
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
	case WorkflowPhasePRPreview:
		// PR preview actions handled by PRPreviewModel
		m.SetActions(nil)
	case WorkflowPhaseCommit:
		// No actions during commit
		m.SetActions(nil)
	}
}

// Action handlers

func (m *CommitWorkflowModel) handleAccept() tea.Cmd {
	m.reviewDecision = DecisionAccept
	// If PR creation is needed, go to PR preview phase first
	if m.createPR {
		return m.preparePRPreview()
	}
	// Otherwise, go directly to commit phase
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return startCommitPhaseMsg{}
	})
}

func (m *CommitWorkflowModel) handleRegenerate() tea.Cmd {
	// Reset state to loading phase
	m.phase = WorkflowPhaseLoading
	m.loadingStage = StagePrompt
	// Rebuild prompt and query again
	return buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)
}

// Phase-specific rendering

func (m *CommitWorkflowModel) renderContent() string {
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

func (m *CommitWorkflowModel) renderPRPreviewContent() string {
	if m.prPreview == nil {
		colors := DefaultColors()
		progressStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
		return m.spinner.View() + " " + progressStyle.Render("Preparing PR preview...")
	}
	return m.prPreview.View()
}

func (m *CommitWorkflowModel) renderCommitContent() string {
	var content strings.Builder

	// Show commit message preview
	colors := DefaultColors()
	titleStyle := lipgloss.NewStyle().Foreground(colors.White).Bold(true)
	messagePreview := m.message
	maxWidth := CalculateContentWidth(m.width) - 4
	if len(messagePreview) > maxWidth {
		messagePreview = messagePreview[:maxWidth-3] + "..."
	}
	content.WriteString(titleStyle.Render("Message: ") + messagePreview + "\n\n")

	successStyle := lipgloss.NewStyle().Foreground(colors.Green)
	errorStyle := lipgloss.NewStyle().Foreground(colors.Red)
	progressStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	descStyle := lipgloss.NewStyle().Foreground(colors.White)

	// Show status based on commit stage
	switch m.commitStage {
	case CommitStageInit, CommitStageCommitting:
		content.WriteString(m.spinner.View() + " " + progressStyle.Render("Committing changes..."))
	case CommitStageCommitted:
		content.WriteString("✓ " + successStyle.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n" + m.spinner.View() + " " + progressStyle.Render("Preparing to push..."))
		}
	case CommitStagePushing:
		content.WriteString("✓ " + successStyle.Render("Committed successfully"))
		content.WriteString("\n" + m.spinner.View() + " " + progressStyle.Render("Pushing to remote..."))
	case CommitStagePushFailed:
		content.WriteString("✓ " + successStyle.Render("Committed successfully"))
		if m.enablePush {
			errorText := "Push failed"
			if m.err != nil {
				errorText = errors.FormatError(m.err)
				if len(errorText) > 120 {
					errorText = errorText[:120] + "..."
				}
			}
			content.WriteString("\n✗ " + errorStyle.Render(errorText))
		}
	case CommitStagePushed:
		content.WriteString("✓ " + successStyle.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n✓ " + successStyle.Render("Pushed successfully"))
		}
		if m.createPR {
			content.WriteString("\n" + m.spinner.View() + " " + progressStyle.Render("Preparing to create PR..."))
		}
	case CommitStageCreatingPR:
		content.WriteString("✓ " + successStyle.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n✓ " + successStyle.Render("Pushed successfully"))
		}
		content.WriteString("\n" + m.spinner.View() + " " + progressStyle.Render("Creating pull request..."))
	case CommitStagePRFailed:
		content.WriteString("✓ " + successStyle.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n✓ " + successStyle.Render("Pushed successfully"))
		}
		if m.createPR {
			errorText := "Pull request creation failed"
			if m.err != nil {
				errorText = errors.FormatError(m.err)
				if len(errorText) > 120 {
					errorText = errorText[:120] + "..."
				}
			}
			content.WriteString("\n✗ " + errorStyle.Render(errorText))
		}
	case CommitStagePRCreated, CommitStageDone:
		content.WriteString("✓ " + successStyle.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n✓ " + successStyle.Render("Pushed successfully"))
		}
		if m.createPR {
			content.WriteString("\n✓ " + successStyle.Render("Pull request created successfully"))
			if m.prURL != "" {
				content.WriteString("\n\n" + descStyle.Render("Pull Request URL:"))
				content.WriteString("\n" + descStyle.Render(m.prURL))
			}
		}
	}

	return content.String()
}

// Override phase title for commit workflow
func (m *CommitWorkflowModel) getPhaseTitle() string {
	switch m.phase {
	case WorkflowPhaseLoading:
		return "Generating Message"
	case WorkflowPhaseReview:
		if m.editing {
			return "Edit Message"
		}
		return "Commit Preview"
	case WorkflowPhasePRPreview:
		return "Pull Request Preview"
	case WorkflowPhaseCommit:
		return "Commit Progress"
	default:
		return "Catmit"
	}
}

// PR preview handling

func (m *CommitWorkflowModel) updatePRPreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "d", "D":
		if m.prPreview != nil {
			m.prPreview.ToggleDetails()
		}
		return m, nil
	case "enter", " ":
		// Continue to commit phase
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return startCommitPhaseMsg{}
		})
	case "c", "C", "q", "Q", "esc":
		m.reviewDecision = DecisionCancel
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *CommitWorkflowModel) preparePRPreview() tea.Cmd {
	return func() tea.Msg {
		// Collect data needed for PR preview
		branchName, _ := m.collector.BranchName(m.ctx)
		changedFiles, _ := m.collector.ChangedFiles(m.ctx)

		// Parse commit message as PR title and body
		lines := strings.Split(m.message, "\n")
		title := lines[0]
		body := ""
		if len(lines) > 1 {
			body = strings.Join(lines[1:], "\n")
			body = strings.TrimSpace(body)
		}

		// Prepare file change info
		var fileChanges []FileChange
		for _, file := range changedFiles {
			fileChanges = append(fileChanges, FileChange{
				Path:       file,
				ChangeType: "modified",
			})
		}

		// Get provider and base branch info if not already set
		providerName := m.prProvider
		baseBranch := m.prBase
		
		// Simple defaults for preview
		if providerName == "" {
			providerName = "github"
		}
		if baseBranch == "" {
			baseBranch = "main"
		}
		
		prData := PRPreviewData{
			Title:       title,
			Body:        body,
			Base:        baseBranch,
			Head:        branchName,
			Remote:      m.prRemote,
			Provider:    providerName,
			IsDraft:     m.prDraft,
			HasChanges:  len(fileChanges) > 0,
			FileChanges: fileChanges,
		}

		return prPreviewReadyMsg{data: prData}
	}
}

// Commit operations

func (m *CommitWorkflowModel) startCommit() tea.Cmd {
	return func() tea.Msg {
		// Stage all changes if stageAll is enabled
		if m.stageAll {
			if err := m.committer.StageAll(m.ctx); err != nil {
				return commitDoneMsg{err: errors.Wrap(errors.ErrTypeGit, "staging failed", err)}
			}
		}
		err := m.committer.Commit(m.ctx, m.message)
		return commitDoneMsg{err: err}
	}
}

func (m *CommitWorkflowModel) startPush() tea.Cmd {
	return func() tea.Msg {
		err := m.committer.Push(m.ctx)
		return pushDoneMsg{err: err}
	}
}

func (m *CommitWorkflowModel) startCreatePR() tea.Cmd {
	return func() tea.Msg {
		prURL, err := m.committer.CreatePullRequest(m.ctx)
		return createPRDoneMsg{err: err, prURL: prURL}
	}
}

// GetError returns the error info
func (m *CommitWorkflowModel) GetError() error {
	if m.err == gitinfo.ErrNoDiff {
		return m.err
	}
	return m.BaseWorkflowModel.GetError()
}