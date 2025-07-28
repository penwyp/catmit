package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/pkg/gitinfo"
)

// Phase represents the current phase of the main model
type Phase int

const (
	PhaseLoading Phase = iota
	PhaseReview
	PhasePRPreview
	PhaseCommit
	PhaseDone
)

// MainModel is a unified single-view model managing the entire lifecycle
type MainModel struct {
	BaseModel

	// State management
	phase          Phase
	loadingStage   Stage
	reviewDecision Decision
	commitStage    CommitStage

	// UI components
	spinner  spinner.Model
	textArea textarea.Model
	editing  bool

	// Data
	message string
	seed    string
	lang    string

	// Dependency injection
	ctx         context.Context
	collector   collectorInterface
	promptBuild promptInterface
	client      clientInterface
	committer   commitInterface

	// Configurations
	enablePush bool
	stageAll   bool
	apiTimeout time.Duration
	createPR   bool
	prRemote   string
	prBase     string
	prDraft    bool
	prProvider string

	// Internal state
	finalStartTime time.Time
	showDuration   time.Duration

	// PR preview related
	prPreview     *PRPreviewModel
	prPreviewData PRPreviewData
	prURL         string

	// Template related
	useTemplate bool // Whether to try using template
	
	// Workflow mode
	isPROnly bool // Whether this is a PR-only workflow (skip commit generation)
}

// PRConfig holds PR configuration
type PRConfig struct {
	CreatePR    bool
	Remote      string
	Base        string
	Draft       bool
	Provider    string
	UseTemplate bool // Whether to use template
}

// NewMainModel creates a new unified model
func NewMainModel(
	ctx context.Context,
	col collectorInterface,
	pb promptInterface,
	cli clientInterface,
	com commitInterface,
	seed, lang string,
	apiTimeout time.Duration,
	enablePush, stageAll, createPR bool,
) *MainModel {
	// Use default PR config
	prConfig := PRConfig{
		CreatePR: createPR,
		Remote:   "origin",
		Base:     "",
		Draft:    false,
		Provider: "",
	}
	return NewMainModelWithPRConfig(ctx, col, pb, cli, com, seed, lang, apiTimeout, enablePush, stageAll, prConfig)
}

// NewMainModelWithPRConfig creates a new unified model with PR config
func NewMainModelWithPRConfig(
	ctx context.Context,
	col collectorInterface,
	pb promptInterface,
	cli clientInterface,
	com commitInterface,
	seed, lang string,
	apiTimeout time.Duration,
	enablePush, stageAll bool,
	prConfig PRConfig,
) *MainModel {
	sp := spinner.New()
	sp.Spinner = spinner.Line

	ta := textarea.New()
	ta.Placeholder = "Edit commit message..."
	ta.CharLimit = 1000
	ta.ShowLineNumbers = false

	m := &MainModel{
		BaseModel:    NewBaseModel("Generating Message", nil),
		phase:        PhaseLoading,
		loadingStage: StageCollect,
		spinner:      sp,
		textArea:     ta,
		ctx:          ctx,
		collector:    col,
		promptBuild:  pb,
		client:       cli,
		committer:    com,
		seed:         seed,
		lang:         lang,
		apiTimeout:   apiTimeout,
		enablePush:   enablePush,
		stageAll:     stageAll,
		createPR:     prConfig.CreatePR,
		prRemote:     prConfig.Remote,
		prBase:       prConfig.Base,
		prDraft:      prConfig.Draft,
		prProvider:   prConfig.Provider,
		useTemplate:  prConfig.UseTemplate,
		showDuration: 1500 * time.Millisecond,
		isPROnly:     false, // Regular workflow includes commit generation
	}

	// Set content renderer
	m.SetContentRenderer(m.renderContent)

	return m
}

// Init starts the first phase
func (m *MainModel) Init() tea.Cmd {
	// For PR-only workflow, skip change collection and go directly to PR generation
	if m.isPROnly {
		return tea.Batch(m.spinner.Tick, m.collectPRDataCmd())
	}
	
	// For regular workflow, start with collection
	return tea.Batch(m.spinner.Tick, collectCmd(m.collector, m.ctx))
}

// Update handles messages
func (m *MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.HandleWindowSize(msg)
		m.textArea.SetWidth(CalculateContentWidth(m.width) - 4)
		m.textArea.SetHeight(8)
		return m, nil

	case tea.KeyMsg:
		// Global shortcut handling
		if msg.String() == "ctrl+c" {
			m.err = context.Canceled
			m.done = true
			return m, tea.Quit
		}

		// Handle keyboard input based on phase
		switch m.phase {
		case PhaseReview:
			return m.updateReview(msg)
		case PhasePRPreview:
			return m.updatePRPreview(msg)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	// Loading phase message handling
	case diffCollectedMsg:
		if m.isPROnly {
			// For PR-only workflow, skip preprocessing and go directly to PR prompt building
			m.loadingStage = StagePrompt
			return m, m.buildPRPromptCmd()
		}
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
		m.phase = PhaseReview
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
			// Add delay to ensure CommitStageCommitted state is fully rendered
			return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
				return delayedPushMsg{}
			})
		} else {
			if m.createPR {
				// Add delay before creating PR
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
			// Show push error for a longer duration before exit
			return m, tea.Tick(m.showDuration*2, func(time.Time) tea.Msg {
				return finalTimeoutMsg{}
			})
		}
		m.commitStage = CommitStageDone
		m.finalStartTime = time.Now()
		return m, tea.Tick(m.showDuration, func(time.Time) tea.Msg {
			return finalTimeoutMsg{}
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
			// Show PR creation error for a longer duration before exit
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
		m.phase = PhaseCommit
		
		// For PR-only workflow, skip commit and go directly to PR creation
		if m.isPROnly {
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
		}
		
		// Normal workflow: commit first
		m.commitStage = CommitStageCommitting
		return m, m.startCommit()

	case prPreviewReadyMsg:
		m.prPreviewData = msg.data
		m.prPreview = NewPRPreviewModel(msg.data, DefaultStyles(), CalculateContentWidth(m.width))
		m.phase = PhasePRPreview
		return m, nil

	case errorMsg:
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	}

	// Handle textarea updates in editing mode during Review phase
	if m.editing && m.phase == PhaseReview {
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// updateReview handles keyboard input during the Review phase
func (m *MainModel) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editing {
		switch msg.String() {
		case "esc":
			m.editing = false
			m.textArea.Blur()
			m.updateActionsForPhase()
			return m, nil
		case "ctrl+s":
			m.message = strings.TrimSpace(m.textArea.Value())
			m.editing = false
			m.textArea.Blur()
			m.updateActionsForPhase()
			return m, nil
		default:
			var cmd tea.Cmd
			m.textArea, cmd = m.textArea.Update(msg)
			return m, cmd
		}
	}

	// Let BaseModel handle navigation and action execution
	cmd := m.HandleKeyboard(msg)
	if cmd != nil {
		return m, cmd
	}

	return m, nil
}

// regenerateCommitMessage triggers regeneration of the commit message
func (m *MainModel) regenerateCommitMessage() tea.Cmd {
	// Reset state to loading phase
	m.phase = PhaseLoading
	m.loadingStage = StagePrompt
	
	// For PR-only workflow, use PR-specific prompt building
	if m.isPROnly {
		return m.buildPRPromptCmd()
	}
	
	// For regular workflow, rebuild prompt and query again
	return buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)
}

// View renders the unified interface
func (m *MainModel) View() string {
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
func (m *MainModel) updateActionsForPhase() {
	switch m.phase {
	case PhaseLoading:
		// No actions during loading
		m.SetActions(nil)
	case PhaseReview:
		if m.editing {
			// No button actions during editing
			m.SetActions(nil)
		} else {
			m.SetActions([]Action{
				{Key: "A", Label: "ccept", Handler: m.handleAccept},
				{Key: "E", Label: "dit", Handler: m.handleEdit},
				{Key: "R", Label: "egenerate", Handler: m.handleRegenerate},
				{Key: "C", Label: "ancel", Handler: m.handleCancel},
			})
		}
	case PhasePRPreview:
		// PR preview actions handled by PRPreviewModel
		m.SetActions(nil)
	case PhaseCommit:
		// No actions during commit
		m.SetActions(nil)
	}
}

// renderContent renders the main content based on current phase
func (m *MainModel) renderContent() string {
	switch m.phase {
	case PhaseLoading:
		return m.renderLoadingContent()
	case PhaseReview:
		if m.editing {
			return m.renderEditingContent()
		}
		return m.renderReviewContent()
	case PhasePRPreview:
		return m.renderPRPreviewContent()
	case PhaseCommit:
		return m.renderCommitContent()
	default:
		return ""
	}
}

// Action handlers
func (m *MainModel) handleAccept() tea.Cmd {
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

func (m *MainModel) handleEdit() tea.Cmd {
	m.editing = true
	m.textArea.Focus()
	m.updateActionsForPhase()
	return textarea.Blink
}

func (m *MainModel) handleRegenerate() tea.Cmd {
	return m.regenerateCommitMessage()
}

func (m *MainModel) handleCancel() tea.Cmd {
	m.reviewDecision = DecisionCancel
	m.done = true
	return tea.Quit
}

// renderLoadingContent renders the content during the loading phase
func (m *MainModel) renderLoadingContent() string {
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
		status = "Generating commit message…"
		statusStyle = lipgloss.NewStyle().Foreground(colors.Green)
	default:
		status = "Processing…"
		statusStyle = lipgloss.NewStyle().Foreground(colors.Gray)
	}

	return m.spinner.View() + " " + statusStyle.Render(status)
}

// renderReviewContent renders the content during the review phase
func (m *MainModel) renderReviewContent() string {
	colors := DefaultColors()
	commitTypeStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	commitDescStyle := lipgloss.NewStyle().Foreground(colors.White)
	commitBodyStyle := lipgloss.NewStyle().Foreground(colors.Gray)

	var content strings.Builder

	// Render commit message
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
func (m *MainModel) renderEditingContent() string {
	colors := DefaultColors()
	promptStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	hintStyle := lipgloss.NewStyle().Foreground(colors.Gray).Italic(true)

	var content strings.Builder
	content.WriteString(promptStyle.Render("Edit Commit Message:") + "\n\n")

	// Render each line of the textarea
	lines := strings.Split(m.textArea.View(), "\n")
	for _, line := range lines {
		content.WriteString(line + "\n")
	}

	content.WriteString("\n" + hintStyle.Render("[Ctrl+S] Save  [Esc] Cancel"))

	return strings.TrimRight(content.String(), "\n")
}

// renderCommitContent renders the content during the commit phase
func (m *MainModel) renderCommitContent() string {

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
				// Use error framework's formatted output
				errorText = errors.FormatError(m.err)
				// Limit length for display if needed
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
		if !m.isPROnly {
			content.WriteString("✓ " + successStyle.Render("Committed successfully"))
			if m.enablePush {
				content.WriteString("\n✓ " + successStyle.Render("Pushed successfully"))
			}
		}
		content.WriteString("\n" + m.spinner.View() + " " + progressStyle.Render("Creating pull request..."))
	case CommitStagePRFailed:
		if !m.isPROnly {
			content.WriteString("✓ " + successStyle.Render("Committed successfully"))
			if m.enablePush {
				content.WriteString("\n✓ " + successStyle.Render("Pushed successfully"))
			}
		}
		if m.createPR {
			errorText := "Pull request creation failed"
			if m.err != nil {
				// Use error framework's formatted output
				errorText = errors.FormatError(m.err)
				// Limit length for display if needed
				if len(errorText) > 120 {
					errorText = errorText[:120] + "..."
				}
			}
			content.WriteString("\n✗ " + errorStyle.Render(errorText))
		}
	case CommitStagePRCreated, CommitStageDone:
		if !m.isPROnly {
			content.WriteString("✓ " + successStyle.Render("Committed successfully"))
			if m.enablePush {
				content.WriteString("\n✓ " + successStyle.Render("Pushed successfully"))
			}
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

// getPhaseTitle gets the title for the current phase
func (m *MainModel) getPhaseTitle() string {
	switch m.phase {
	case PhaseLoading:
		return "Generating Message"
	case PhaseReview:
		if m.editing {
			return "Edit Message"
		}
		return "Commit Preview"
	case PhasePRPreview:
		return "Pull Request Preview"
	case PhaseCommit:
		return "Commit Progress"
	default:
		return "Catmit"
	}
}

// startCommit starts the commit process
func (m *MainModel) startCommit() tea.Cmd {
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

// startPush starts the push process
func (m *MainModel) startPush() tea.Cmd {
	return func() tea.Msg {
		err := m.committer.Push(m.ctx)
		return pushDoneMsg{err: err}
	}
}

// startCreatePR starts the pull request creation process
func (m *MainModel) startCreatePR() tea.Cmd {
	return func() tea.Msg {
		prURL, err := m.committer.CreatePullRequest(m.ctx)
		return createPRDoneMsg{err: err, prURL: prURL}
	}
}

// IsDone returns whether the operation is done and related info
func (m *MainModel) IsDone() (bool, Decision, string, error) {
	return m.done, m.reviewDecision, m.message, m.err
}

// GetError returns the error info
func (m *MainModel) GetError() error {
	if m.err == gitinfo.ErrNoDiff {
		return m.err
	}
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

// Message type definitions
type delayedPushMsg struct{}
type delayedCreatePRMsg struct{}
type startCommitPhaseMsg struct{}

type prPreviewReadyMsg struct {
	data PRPreviewData
}

type createPRDoneMsg struct {
	err   error
	prURL string
}

// preparePRPreview prepares the PR preview
func (m *MainModel) preparePRPreview() tea.Cmd {
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
			// Simplified: in practice, should get added/removed lines from git
			fileChanges = append(fileChanges, FileChange{
				Path:       file,
				ChangeType: "modified",
			})
		}

		// Get provider and base branch info if not already set
		providerName := m.prProvider
		baseBranch := m.prBase
		
		// If provider not specified, try to detect it from remote URL pattern
		if providerName == "" {
			// Simple provider detection based on common patterns
			// This is a simplified version - the actual detection happens in PR creator
			providerName = "github" // Default assumption for preview
		}
		
		// If base branch not specified, use a reasonable default
		if baseBranch == "" {
			// Common default branches
			baseBranch = "main" // Most common default
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

// renderPRPreviewContent renders the PR preview content
func (m *MainModel) renderPRPreviewContent() string {
	if m.prPreview == nil {
		colors := DefaultColors()
		progressStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
		return m.spinner.View() + " " + progressStyle.Render("Preparing PR preview...")
	}

	return m.prPreview.View()
}

// updatePRPreview handles keyboard input during the PR preview phase
func (m *MainModel) updatePRPreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

// NewPROnlyModel creates a new model specifically for PR-only workflow
func NewPROnlyModel(
	ctx context.Context,
	col collectorInterface,
	pb promptInterface,
	cli clientInterface,
	com commitInterface,
	lang string,
	apiTimeout time.Duration,
	prConfig PRConfig,
) *MainModel {
	sp := spinner.New()
	sp.Spinner = spinner.Line

	ta := textarea.New()
	ta.Placeholder = "Edit PR description..."
	ta.CharLimit = 2000
	ta.ShowLineNumbers = false

	m := &MainModel{
		BaseModel:    NewBaseModel("Analyzing Commits", nil),
		phase:        PhaseLoading,
		loadingStage: StageCollect,
		spinner:      sp,
		textArea:     ta,
		ctx:          ctx,
		collector:    col,
		promptBuild:  pb,
		client:       cli,
		committer:    com,
		seed:         "", // No seed text for PR
		lang:         lang,
		apiTimeout:   apiTimeout,
		enablePush:   true,  // Always push before PR
		stageAll:     false, // No staging needed for PR-only
		createPR:     true,  // Always create PR in PR-only mode
		prRemote:     prConfig.Remote,
		prBase:       prConfig.Base,
		prDraft:      prConfig.Draft,
		prProvider:   prConfig.Provider,
		useTemplate:  prConfig.UseTemplate,
		showDuration: 1500 * time.Millisecond,
		isPROnly:     true, // PR-only workflow skips commit generation
	}

	// Set content renderer
	m.SetContentRenderer(m.renderContent)

	// For PR-only workflow, we'll generate PR content directly
	// without going through commit generation
	return m
}

// collectPRDataCmd collects commit data for PR generation
func (m *MainModel) collectPRDataCmd() tea.Cmd {
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

// buildPRPromptCmd builds prompts specifically for PR generation
func (m *MainModel) buildPRPromptCmd() tea.Cmd {
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
