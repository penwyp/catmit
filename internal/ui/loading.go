package ui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/pkg/gitinfo"
)

// Stage represents the progress stage
type Stage int

const (
	StageCollect    Stage = iota
	StagePreprocess       // New: intelligent data preprocessing stage
	StagePrompt
	StageQuery
	StageDone
)

// Interfaces duplicated to decouple from cmd package
type collectorInterface interface {
	RecentCommits(ctx context.Context, n int) ([]string, error)
	BranchName(ctx context.Context) (string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
	FileStatusSummary(ctx context.Context) (*gitinfo.FileStatusSummary, error)
	ComprehensiveDiff(ctx context.Context) (string, error)
	AnalyzeChanges(ctx context.Context) (*gitinfo.ChangesSummary, error)
}

type promptInterface interface {
	Build(seed, diff string, commits []string, branch string, files []string) string
	BuildSystemPrompt() string
	BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string
	BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error)
}

type clientInterface interface {
	GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// LoadingModel displays a spinner during time-consuming steps.
// After completion, it exits via tea.Quit, and writes message or err back to its own fields.
// Dependencies are injected for easier testing.

type LoadingModel struct {
	stage   Stage
	spinner spinner.Model
	// injected dependencies
	ctx         context.Context
	collector   collectorInterface
	promptBuild promptInterface
	client      clientInterface

	seed       string
	lang       string
	apiTimeout time.Duration

	// timing control for minimum display duration
	stageStartTime time.Time
	minStageDelay  time.Duration

	message string
	err     error
}

// NewLoadingModel creates a new LoadingModel with injected dependencies and initial settings.
func NewLoadingModel(ctx context.Context, col collectorInterface, pb promptInterface, cli clientInterface, seed, lang string, apiTimeout time.Duration) *LoadingModel {
	sp := spinner.New()
	sp.Spinner = spinner.Line
	return &LoadingModel{
		stage:          StageCollect,
		spinner:        sp,
		ctx:            ctx,
		collector:      col,
		promptBuild:    pb,
		client:         cli,
		seed:           seed,
		lang:           lang,
		apiTimeout:     apiTimeout,
		stageStartTime: time.Now(),
		minStageDelay:  500 * time.Millisecond,
	}
}

// Init starts the first stage.
func (m *LoadingModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, collectCmd(m.collector, m.ctx))
}

// Update handles messages.
func (m *LoadingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.err = context.Canceled
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case diffCollectedMsg:
		// Check if we need to delay transition to preprocessing stage
		elapsed := time.Since(m.stageStartTime)
		if elapsed < m.minStageDelay {
			// Need to delay, use tea.Tick to delay the remaining time
			remaining := m.minStageDelay - elapsed
			return m, tea.Tick(remaining, func(time.Time) tea.Msg {
				return delayedPreprocessMsg{originalMsg: msg}
			})
		}
		// Minimum display time reached, transition directly
		m.stage = StagePreprocess
		m.stageStartTime = time.Now() // Reset timer
		return m, preprocessCmd(m.collector, m.ctx)
	case preprocessDoneMsg:
		// Check if we need to delay transition to prompt building stage
		elapsed := time.Since(m.stageStartTime)
		if elapsed < m.minStageDelay {
			// Need to delay, use tea.Tick to delay the remaining time
			remaining := m.minStageDelay - elapsed
			return m, tea.Tick(remaining, func(time.Time) tea.Msg {
				return delayedPromptMsg{originalMsg: msg}
			})
		}
		// Minimum display time reached, transition directly
		m.stage = StagePrompt
		return m, buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)
	case delayedPreprocessMsg:
		// Delay is over, now transition to preprocessing stage
		m.stage = StagePreprocess
		m.stageStartTime = time.Now() // Reset timer
		return m, preprocessCmd(m.collector, m.ctx)
	case delayedPromptMsg:
		// Delay is over, now transition to prompt building stage
		m.stage = StagePrompt
		return m, buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)
	case smartPromptBuiltMsg:
		// Smart prompt built, enter Query stage
		m.stage = StageQuery
		return m, queryCmd(m.client, m.ctx, msg.systemPrompt, msg.userPrompt, m.apiTimeout)
	case promptBuiltMsg:
		// Traditional prompt built, enter Query stage (fallback path)
		m.stage = StageQuery
		return m, queryCmd(m.client, m.ctx, msg.systemPrompt, msg.userPrompt, m.apiTimeout)
	case queryDoneMsg:
		m.stage = StageDone
		m.message = msg.message
		return m, tea.Quit
	case errorMsg:
		m.stage = StageDone
		m.err = msg.err
		return m, tea.Quit
	}
	// By default, let spinner handle other messages
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// View displays text according to the current stage.
func (m *LoadingModel) View() string {
	// Define colors for different stages
	colors := DefaultColors()
	var statusStyle lipgloss.Style
	var status string

	switch m.stage {
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
		statusStyle = lipgloss.NewStyle().Foreground(colors.SecondaryGray)
	}

	return m.spinner.View() + " " + statusStyle.Render(status)
}

// IsDone returns the result.
func (m *LoadingModel) IsDone() (string, error) {
	return m.message, m.err
}

// ---------------- tea.Msg definitions ----------------

type diffCollectedMsg struct {
	diff    string
	commits []string
	branch  string
	files   []string
}

// New: message for preprocessing done
type preprocessDoneMsg struct {
	summary *gitinfo.FileStatusSummary
}

type promptBuiltMsg struct {
	systemPrompt string
	userPrompt   string
}

// New: message for smart prompt built
type smartPromptBuiltMsg struct {
	systemPrompt string
	userPrompt   string
}

// New: message type for delayed transition
type delayedPreprocessMsg struct {
	originalMsg diffCollectedMsg
}

type delayedPromptMsg struct {
	originalMsg preprocessDoneMsg
}

type queryDoneMsg struct{ message string }

type errorMsg struct{ err error }

// ---------------- Cmd implementations --------------------

func collectCmd(col collectorInterface, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Use ComprehensiveDiff to include untracked files
		diff, err := col.ComprehensiveDiff(ctx)
		if err != nil {
			return errorMsg{err}
		}
		commits, err := col.RecentCommits(ctx, 10)
		if err != nil {
			return errorMsg{err}
		}
		branch, _ := col.BranchName(ctx)
		files, _ := col.ChangedFiles(ctx)
		return diffCollectedMsg{diff: diff, commits: commits, branch: branch, files: files}
	}
}

// Preprocessing command, get file status summary
func preprocessCmd(col collectorInterface, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Try to use the new FileStatusSummary method
		summary, err := col.FileStatusSummary(ctx)
		if err != nil {
			// If the new method fails, maybe collector does not implement the new interface, return error
			return errorMsg{err}
		}
		return preprocessDoneMsg{summary: summary}
	}
}

// Smart prompt building command, using token budget control
func buildSmartPromptCmd(pb promptInterface, col collectorInterface, ctx context.Context, seed string) tea.Cmd {
	return func() tea.Msg {
		// Try to use the new BuildUserPromptWithBudget method
		systemPrompt := pb.BuildSystemPrompt()
		userPrompt, err := pb.BuildUserPromptWithBudget(ctx, col, seed)
		if err != nil {
			// If the new method fails, fallback to traditional method
			return errorMsg{err}
		}
		return smartPromptBuiltMsg{systemPrompt: systemPrompt, userPrompt: userPrompt}
	}
}

func queryCmd(cli clientInterface, ctx context.Context, systemPrompt, userPrompt string, apiTimeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		// Create timeout context only for API call
		apiCtx, cancel := context.WithTimeout(ctx, apiTimeout)
		defer cancel()
		msg, err := cli.GetCommitMessage(apiCtx, systemPrompt, userPrompt)
		if err != nil {
			return errorMsg{err}
		}
		return queryDoneMsg{message: msg}
	}
}
