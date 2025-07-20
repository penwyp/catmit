package ui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/collector"
)

// Stage 表示进度阶段
type Stage int

const (
	StageCollect    Stage = iota
	StagePreprocess       // 新增：智能数据预处理阶段
	StagePrompt
	StageQuery
	StageDone
)

// Interfaces duplicated to decouple from cmd package
type collectorInterface interface {
	RecentCommits(ctx context.Context, n int) ([]string, error)
	BranchName(ctx context.Context) (string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
	FileStatusSummary(ctx context.Context) (*collector.FileStatusSummary, error)
	ComprehensiveDiff(ctx context.Context) (string, error)
	AnalyzeChanges(ctx context.Context) (*collector.ChangesSummary, error)
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

// LoadingModel 在执行耗时步骤时展示 Spinner
// 完成后通过 tea.Quit 退出，将 message 或 err 写回自身字段
// 依赖注入接口，便于测试。

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

// Init 启动第一个阶段
func (m *LoadingModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, collectCmd(m.collector, m.ctx))
}

// Update 处理消息
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
		// 检查是否需要延迟过渡到预处理阶段
		elapsed := time.Since(m.stageStartTime)
		if elapsed < m.minStageDelay {
			// 需要延迟，使用 tea.Tick 延迟剩余时间
			remaining := m.minStageDelay - elapsed
			return m, tea.Tick(remaining, func(time.Time) tea.Msg {
				return delayedPreprocessMsg{originalMsg: msg}
			})
		}
		// 已经达到最小显示时间，直接过渡
		m.stage = StagePreprocess
		m.stageStartTime = time.Now() // 重置计时器
		return m, preprocessCmd(m.collector, m.ctx)
	case preprocessDoneMsg:
		// 检查是否需要延迟过渡到Prompt构建阶段
		elapsed := time.Since(m.stageStartTime)
		if elapsed < m.minStageDelay {
			// 需要延迟，使用 tea.Tick 延迟剩余时间
			remaining := m.minStageDelay - elapsed
			return m, tea.Tick(remaining, func(time.Time) tea.Msg {
				return delayedPromptMsg{originalMsg: msg}
			})
		}
		// 已经达到最小显示时间，直接过渡
		m.stage = StagePrompt
		return m, buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)
	case delayedPreprocessMsg:
		// 延迟时间已到，现在可以过渡到预处理阶段
		m.stage = StagePreprocess
		m.stageStartTime = time.Now() // 重置计时器
		return m, preprocessCmd(m.collector, m.ctx)
	case delayedPromptMsg:
		// 延迟时间已到，现在可以过渡到Prompt构建阶段
		m.stage = StagePrompt
		return m, buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)
	case smartPromptBuiltMsg:
		// 智能prompt构建完成，进入Query阶段
		m.stage = StageQuery
		return m, queryCmd(m.client, m.ctx, msg.systemPrompt, msg.userPrompt, m.apiTimeout)
	case promptBuiltMsg:
		// 传统prompt构建完成，进入Query阶段（fallback路径）
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
	// 默认交给 spinner 处理其他消息
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

// View 根据阶段显示文字
func (m *LoadingModel) View() string {
	// Define colors for different stages
	var statusStyle lipgloss.Style
	var status string

	switch m.stage {
	case StageCollect:
		status = "Collecting diff…"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("33")) // Orange
	case StagePreprocess:
		status = "Preprocessing files…"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Dark orange
	case StagePrompt:
		status = "Crafting prompt…"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")) // Blue
	case StageQuery:
		status = "Generating commit message…"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // Green
	default:
		status = "Processing…"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250")) // Gray
	}

	return m.spinner.View() + " " + statusStyle.Render(status)
}

// IsDone 返回结果
func (m *LoadingModel) IsDone() (string, error) {
	return m.message, m.err
}

// ---------------- tea.Msg 定义 ----------------

type diffCollectedMsg struct {
	diff    string
	commits []string
	branch  string
	files   []string
}

// 新增：预处理完成消息
type preprocessDoneMsg struct {
	summary *collector.FileStatusSummary
}

type promptBuiltMsg struct {
	systemPrompt string
	userPrompt   string
}

// 新增：智能prompt构建完成消息
type smartPromptBuiltMsg struct {
	systemPrompt string
	userPrompt   string
}

// 新增：延迟过渡消息类型
type delayedPreprocessMsg struct {
	originalMsg diffCollectedMsg
}

type delayedPromptMsg struct {
	originalMsg preprocessDoneMsg
}

type queryDoneMsg struct{ message string }

type errorMsg struct{ err error }

// ---------------- Cmd 实现 --------------------

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

// 预处理命令，获取文件状态摘要
func preprocessCmd(col collectorInterface, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// 尝试使用新的FileStatusSummary方法
		summary, err := col.FileStatusSummary(ctx)
		if err != nil {
			// 如果新方法失败，可能collector没有实现新接口，返回错误
			return errorMsg{err}
		}
		return preprocessDoneMsg{summary: summary}
	}
}

// 智能prompt构建命令，使用token预算控制
func buildSmartPromptCmd(pb promptInterface, col collectorInterface, ctx context.Context, seed string) tea.Cmd {
	return func() tea.Msg {
		// 尝试使用新的BuildUserPromptWithBudget方法
		systemPrompt := pb.BuildSystemPrompt()
		userPrompt, err := pb.BuildUserPromptWithBudget(ctx, col, seed)
		if err != nil {
			// 如果新方法失败，fallback到传统方法
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
