package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommitStage represents the stage of commit/push operation
type CommitStage int

const (
	CommitStageInit CommitStage = iota
	CommitStageCommitting
	CommitStageCommitted
	CommitStagePushing
	CommitStagePushed
	CommitStagePushFailed
	CommitStageCreatingPR
	CommitStagePRCreated
	CommitStagePRFailed
	CommitStageDone
)

// CommitModel displays the progress of commit and push operations, keeping the same visual style as ReviewModel
type CommitModel struct {
	stage      CommitStage
	message    string // commit message
	lang       string // language setting
	enablePush bool   // whether to enable push
	stageAll   bool   // whether to stage all
	spinner    spinner.Model

	// operation interface
	committer commitInterface
	ctx       context.Context

	// state management
	err  error
	done bool

	// responsive terminal size support
	terminalWidth  int
	terminalHeight int

	// display control
	showDuration   time.Duration // duration to show final state
	finalStartTime time.Time     // start time of final state
}

// commitInterface defines the interface for commit and push operations
type commitInterface interface {
	Commit(ctx context.Context, message string) error
	Push(ctx context.Context) error
	StageAll(ctx context.Context) error
	HasStagedChanges(ctx context.Context) bool
	CreatePullRequest(ctx context.Context) (string, error)
	NeedsPush(ctx context.Context) (bool, error)
}

// NewCommitModel creates a new CommitModel
func NewCommitModel(ctx context.Context, committer commitInterface, message, lang string, enablePush, stageAll bool) *CommitModel {
	sp := spinner.New()
	sp.Spinner = spinner.Line

	return &CommitModel{
		stage:          CommitStageInit,
		message:        message,
		lang:           lang,
		enablePush:     enablePush,
		stageAll:       stageAll,
		spinner:        sp,
		committer:      committer,
		ctx:            ctx,
		terminalWidth:  80,                      // default width
		terminalHeight: 24,                      // default height
		showDuration:   1500 * time.Millisecond, // show final state for 1.5 seconds
	}
}

// Init implements tea.Model interface
func (m *CommitModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.startCommit())
}

// Update handles messages
func (m *CommitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.err = context.Canceled
			m.done = true
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case commitDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}
		m.stage = CommitStageCommitted
		if m.enablePush {
			m.stage = CommitStagePushing
			return m, m.startPush()
		} else {
			// No push, go directly to done state
			m.stage = CommitStageDone
			m.finalStartTime = time.Now()
			return m, tea.Tick(m.showDuration, func(time.Time) tea.Msg {
				return finalTimeoutMsg{}
			})
		}
	case pushDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}
		m.stage = CommitStagePushed
		m.stage = CommitStageDone
		m.finalStartTime = time.Now()
		return m, tea.Tick(m.showDuration, func(time.Time) tea.Msg {
			return finalTimeoutMsg{}
		})
	case finalTimeoutMsg:
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// calculateContentWidth calculates the dynamic content width based on terminal width (reuse logic from ReviewModel)
func (m *CommitModel) calculateContentWidth() int {
	const (
		minWidth = 60  // minimum width
		maxWidth = 120 // maximum width
		margin   = 4   // left and right margin
	)

	availableWidth := m.terminalWidth - margin

	if availableWidth < minWidth {
		return minWidth
	}
	if availableWidth > maxWidth {
		return maxWidth
	}

	return availableWidth
}

// View renders the UI (returns only the content, border is handled by MainModel)
func (m *CommitModel) View() string {
	// Palette (consistent with ReviewModel)
	const (
		cGray   = lipgloss.Color("245")
		cBlue   = lipgloss.Color("39")
		cGreen  = lipgloss.Color("42")
		cYellow = lipgloss.Color("220")
		cWhite  = lipgloss.Color("255")
	)

	contentWidth := m.calculateContentWidth()

	// Style definitions
	titleStyle := lipgloss.NewStyle().Foreground(cWhite).Bold(true)

	// Status styles
	progressStyle := lipgloss.NewStyle().Foreground(cYellow)
	successStyle := lipgloss.NewStyle().Foreground(cGreen)

	// Build content
	var content strings.Builder

	// Show commit message (truncate if needed)
	messagePreview := m.message
	if len(messagePreview) > contentWidth-4 {
		messagePreview = messagePreview[:contentWidth-7] + "..."
	}
	content.WriteString(" " + titleStyle.Render("Message: ") + messagePreview + "\n")
	content.WriteString("\n") // blank line

	// Show status based on stage
	switch m.stage {
	case CommitStageInit, CommitStageCommitting:
		statusLine := " " + m.spinner.View() + " " + progressStyle.Render("Committing changes...")
		content.WriteString(statusLine)
	case CommitStageCommitted:
		statusLine := " ✓ " + successStyle.Render("Committed successfully")
		content.WriteString(statusLine)
		if m.enablePush {
			statusLine = "\n " + m.spinner.View() + " " + progressStyle.Render("Preparing to push...")
			content.WriteString(statusLine)
		}
	case CommitStagePushing:
		statusLine := " ✓ " + successStyle.Render("Committed successfully")
		content.WriteString(statusLine)
		statusLine = "\n " + m.spinner.View() + " " + progressStyle.Render("Pushing to remote...")
		content.WriteString(statusLine)
	case CommitStagePushed, CommitStageDone:
		statusLine := " ✓ " + successStyle.Render("Committed successfully")
		content.WriteString(statusLine)
		if m.enablePush {
			statusLine = "\n ✓ " + successStyle.Render("Pushed successfully")
			content.WriteString(statusLine)
		}
	}

	return content.String()
}

// IsDone returns whether the operation is finished
func (m *CommitModel) IsDone() (bool, error) {
	return m.done, m.err
}

// --- Command and message types ---

type commitDoneMsg struct {
	err error
}

type pushDoneMsg struct {
	err error
}

type finalTimeoutMsg struct{}

// startCommit starts the commit operation
func (m *CommitModel) startCommit() tea.Cmd {
	return func() tea.Msg {
		m.stage = CommitStageCommitting
		err := m.committer.Commit(m.ctx, m.message)
		return commitDoneMsg{err: err}
	}
}

// startPush starts the push operation
func (m *CommitModel) startPush() tea.Cmd {
	return func() tea.Msg {
		err := m.committer.Push(m.ctx)
		return pushDoneMsg{err: err}
	}
}
