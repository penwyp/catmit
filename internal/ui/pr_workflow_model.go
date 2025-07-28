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
)

// PRWorkflowModel handles the PR-only workflow (no commit generation)
type PRWorkflowModel struct {
	*BaseWorkflowModel

	// PR-specific data
	promptBuild promptInterface

	// Configurations
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

// NewPRWorkflowModel creates a new PR-only workflow model
func NewPRWorkflowModel(
	ctx context.Context,
	col collectorInterface,
	pb promptInterface,
	cli clientInterface,
	com commitInterface,
	lang string,
	apiTimeout time.Duration,
	prConfig PRConfig,
) *PRWorkflowModel {
	base := NewBaseWorkflowModel(
		"Analyzing Commits",
		ctx,
		col,
		cli,
		com,
		lang,
		apiTimeout,
	)

	m := &PRWorkflowModel{
		BaseWorkflowModel: base,
		promptBuild:       pb,
		prRemote:          prConfig.Remote,
		prBase:            prConfig.Base,
		prDraft:           prConfig.Draft,
		prProvider:        prConfig.Provider,
		useTemplate:       prConfig.UseTemplate,
	}

	// Override content renderer to use PR-specific rendering
	base.SetContentRenderer(m.renderContent)

	// Adjust textarea for PR descriptions
	base.textArea.Placeholder = "Edit PR description..."
	base.textArea.CharLimit = 2000

	return m
}

// Init starts the first phase
func (m *PRWorkflowModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.collectPRDataCmd())
}

// Update handles messages
func (m *PRWorkflowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		// For PR-only workflow, skip preprocessing and go directly to PR prompt building
		m.loadingStage = StagePrompt
		return m, m.buildPRPromptCmd()

	case smartPromptBuiltMsg:
		m.loadingStage = StageQuery
		return m, queryCmd(m.client, m.ctx, msg.systemPrompt, msg.userPrompt, m.apiTimeout)

	case queryDoneMsg:
		m.message = strings.TrimSpace(strings.ReplaceAll(msg.message, "\r", ""))
		m.phase = WorkflowPhaseReview
		m.textArea.SetValue(m.message)
		return m, nil

	// Push and PR creation handling
	case pushDoneMsg:
		if msg.err != nil {
			m.commitStage = CommitStagePushFailed
			m.err = msg.err
			m.finalStartTime = time.Now()
			return m, tea.Tick(m.showDuration*2, func(time.Time) tea.Msg {
				return finalTimeoutMsg{}
			})
		}
		// Push succeeded, create PR
		m.commitStage = CommitStagePushed
		return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
			return delayedCreatePRMsg{}
		})

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
		
		// For PR-only workflow, skip commit and go directly to PR creation
		// Check if we need to push first
		needsPush, err := m.committer.NeedsPush(m.ctx)
		if err != nil {
			// Log error but continue
			needsPush = false
		}
		
		if needsPush {
			m.commitStage = CommitStagePushing
			return m, m.startPush()
		} else {
			// Go directly to PR creation
			m.commitStage = CommitStageCreatingPR
			return m, m.startCreatePR()
		}

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

// updateActionsForPhase updates available actions based on current phase
func (m *PRWorkflowModel) updateActionsForPhase() {
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
		// No actions during PR creation
		m.SetActions(nil)
	}
}

// Action handlers

func (m *PRWorkflowModel) handleAccept() tea.Cmd {
	m.reviewDecision = DecisionAccept
	// Go to PR preview phase
	return m.preparePRPreview()
}

func (m *PRWorkflowModel) handleRegenerate() tea.Cmd {
	// Reset state to loading phase
	m.phase = WorkflowPhaseLoading
	m.loadingStage = StagePrompt
	// Rebuild PR prompt and query again
	return m.buildPRPromptCmd()
}

// Phase-specific rendering

func (m *PRWorkflowModel) renderContent() string {
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

func (m *PRWorkflowModel) renderPRPreviewContent() string {
	if m.prPreview == nil {
		colors := DefaultColors()
		progressStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
		return m.spinner.View() + " " + progressStyle.Render("Preparing PR preview...")
	}
	return m.prPreview.View()
}

func (m *PRWorkflowModel) renderCommitContent() string {
	var content strings.Builder

	// Show PR title preview
	colors := DefaultColors()
	titleStyle := lipgloss.NewStyle().Foreground(colors.White).Bold(true)
	messagePreview := m.message
	maxWidth := CalculateContentWidth(m.width) - 4
	// For PR workflow, show first line as title
	lines := strings.Split(messagePreview, "\n")
	if len(lines) > 0 {
		titlePreview := lines[0]
		if len(titlePreview) > maxWidth {
			titlePreview = titlePreview[:maxWidth-3] + "..."
		}
		content.WriteString(titleStyle.Render("Title: ") + titlePreview + "\n")
		// Show truncated body
		if len(lines) > 1 {
			bodyPreview := strings.Join(lines[1:], " ")
			bodyPreview = strings.TrimSpace(bodyPreview)
			if len(bodyPreview) > maxWidth {
				bodyPreview = bodyPreview[:maxWidth-3] + "..."
			}
			if bodyPreview != "" {
				content.WriteString("\n" + titleStyle.Render("Body: ") + bodyPreview[:min(len(bodyPreview), 50)] + "...\n")
			}
		}
		content.WriteString("\n")
	}

	successStyle := lipgloss.NewStyle().Foreground(colors.Green)
	errorStyle := lipgloss.NewStyle().Foreground(colors.Red)
	progressStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	descStyle := lipgloss.NewStyle().Foreground(colors.White)

	// Show status based on commit stage
	switch m.commitStage {
	case CommitStagePushing:
		content.WriteString(m.spinner.View() + " " + progressStyle.Render("Pushing to remote..."))
	case CommitStagePushFailed:
		errorText := "Push failed"
		if m.err != nil {
			errorText = errors.FormatError(m.err)
			if len(errorText) > 120 {
				errorText = errorText[:120] + "..."
			}
		}
		content.WriteString("✗ " + errorStyle.Render(errorText))
	case CommitStagePushed:
		content.WriteString("✓ " + successStyle.Render("Pushed successfully"))
		content.WriteString("\n" + m.spinner.View() + " " + progressStyle.Render("Preparing to create PR..."))
	case CommitStageCreatingPR:
		content.WriteString(m.spinner.View() + " " + progressStyle.Render("Creating pull request..."))
	case CommitStagePRFailed:
		errorText := "Pull request creation failed"
		if m.err != nil {
			errorText = errors.FormatError(m.err)
			if len(errorText) > 120 {
				errorText = errorText[:120] + "..."
			}
		}
		content.WriteString("✗ " + errorStyle.Render(errorText))
	case CommitStagePRCreated, CommitStageDone:
		content.WriteString("✓ " + successStyle.Render("Pull request created successfully"))
		if m.prURL != "" {
			content.WriteString("\n\n" + descStyle.Render("Pull Request URL:"))
			content.WriteString("\n" + descStyle.Render(m.prURL))
		}
	}

	return content.String()
}

// Override phase title for PR workflow
func (m *PRWorkflowModel) getPhaseTitle() string {
	switch m.phase {
	case WorkflowPhaseLoading:
		return "Analyzing Commits"
	case WorkflowPhaseReview:
		if m.editing {
			return "Edit PR Description"
		}
		return "PR Preview"
	case WorkflowPhasePRPreview:
		return "Pull Request Preview"
	case WorkflowPhaseCommit:
		return "Creating PR"
	default:
		return "Catmit PR"
	}
}

// PR preview handling

func (m *PRWorkflowModel) updatePRPreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "d", "D":
		if m.prPreview != nil {
			m.prPreview.ToggleDetails()
		}
		return m, nil
	case "enter", " ":
		// Continue to PR creation phase
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

func (m *PRWorkflowModel) preparePRPreview() tea.Cmd {
	return func() tea.Msg {
		// Collect data needed for PR preview
		branchName, _ := m.collector.BranchName(m.ctx)
		
		// For PR workflow, we don't have changed files since everything is committed
		// We could potentially analyze commits to get file changes
		
		// Parse PR content as title and body
		lines := strings.Split(m.message, "\n")
		title := lines[0]
		body := ""
		if len(lines) > 1 {
			body = strings.Join(lines[1:], "\n")
			body = strings.TrimSpace(body)
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
			HasChanges:  true, // Assume there are changes if creating PR
			FileChanges: []FileChange{}, // Empty for PR-only workflow
		}

		return prPreviewReadyMsg{data: prData}
	}
}

// Data collection for PR

func (m *PRWorkflowModel) collectPRDataCmd() tea.Cmd {
	return func() tea.Msg {
		// Collect recent commits for PR generation
		commits, err := m.collector.RecentCommits(m.ctx, 20)
		if err != nil {
			return errorMsg{err}
		}
		
		// Get branch name
		branch, _ := m.collector.BranchName(m.ctx)
		
		// For PR-only, we simulate a "diff collected" message with commit info
		// but no actual diff since all changes are committed
		return diffCollectedMsg{
			diff:    "", // No uncommitted changes
			commits: commits,
			branch:  branch,
			files:   []string{}, // No changed files
		}
	}
}

func (m *PRWorkflowModel) buildPRPromptCmd() tea.Cmd {
	return func() tea.Msg {
		// For PR-only workflow, we use PR-specific prompts
		systemPrompt := m.promptBuild.BuildPRSystemPrompt()
		
		// Get recent commits for PR content generation
		commits, err := m.collector.RecentCommits(m.ctx, 20)
		if err != nil {
			return errorMsg{err}
		}
		
		userPrompt := m.promptBuild.BuildPRUserPrompt(commits)
		
		return smartPromptBuiltMsg{
			systemPrompt: systemPrompt,
			userPrompt:   userPrompt,
		}
	}
}

// PR operations

func (m *PRWorkflowModel) startPush() tea.Cmd {
	return func() tea.Msg {
		err := m.committer.Push(m.ctx)
		return pushDoneMsg{err: err}
	}
}

func (m *PRWorkflowModel) startCreatePR() tea.Cmd {
	return func() tea.Msg {
		prURL, err := m.committer.CreatePullRequest(m.ctx)
		return createPRDoneMsg{err: err, prURL: prURL}
	}
}

// min function is already defined in framework.go, no need to redefine