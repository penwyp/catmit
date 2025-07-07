package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/collector"
)

// Stage 表示进度阶段
type Stage int

const (
	StageCollect Stage = iota
	StagePreprocess // 新增：智能数据预处理阶段
	StagePrompt
	StageQuery
	StageDone
)

// Interfaces duplicated to decouple from cmd package
type collectorInterface interface {
	RecentCommits(ctx context.Context, n int) ([]string, error)
	Diff(ctx context.Context) (string, error)
	BranchName(ctx context.Context) (string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
	// 新增：支持文件状态摘要
	FileStatusSummary(ctx context.Context) (*collector.FileStatusSummary, error)
}

type promptInterface interface {
	Build(seed, diff string, commits []string, branch string, files []string) string
	BuildSystemPrompt() string
	BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string
	// 新增：支持token预算控制的智能prompt构建
	BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error)
}

type clientInterface interface {
	GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	GetCommitMessageLegacy(ctx context.Context, prompt string) (string, error)
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

	seed string
	lang string

	message string
	err     error
}

func NewLoadingModel(ctx context.Context, col collectorInterface, pb promptInterface, cli clientInterface, seed, lang string) *LoadingModel {
	sp := spinner.New()
	sp.Spinner = spinner.Line
	return &LoadingModel{
		stage:       StageCollect,
		spinner:     sp,
		ctx:         ctx,
		collector:   col,
		promptBuild: pb,
		client:      cli,
		seed:        seed,
		lang:        lang,
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
		// 进入预处理阶段，尝试使用新的智能处理流程
		m.stage = StagePreprocess
		return m, preprocessCmd(m.collector, m.ctx)
	case preprocessDoneMsg:
		// 预处理完成，进入智能Prompt构建阶段
		m.stage = StagePrompt
		return m, buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)
	case smartPromptBuiltMsg:
		// 智能prompt构建完成，进入Query阶段
		m.stage = StageQuery
		return m, queryCmd(m.client, m.ctx, msg.systemPrompt, msg.userPrompt)
	case promptBuiltMsg:
		// 传统prompt构建完成，进入Query阶段（fallback路径）
		m.stage = StageQuery
		return m, queryCmd(m.client, m.ctx, msg.systemPrompt, msg.userPrompt)
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

type queryDoneMsg struct{ message string }

type errorMsg struct{ err error }

// ---------------- Cmd 实现 --------------------

func collectCmd(col collectorInterface, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		diff, err := col.Diff(ctx)
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

// 新增：预处理命令，获取文件状态摘要
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

// 新增：智能prompt构建命令，使用token预算控制
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


func queryCmd(cli clientInterface, ctx context.Context, systemPrompt, userPrompt string) tea.Cmd {
	return func() tea.Msg {
		msg, err := cli.GetCommitMessage(ctx, systemPrompt, userPrompt)
		if err != nil {
			return errorMsg{err}
		}
		return queryDoneMsg{message: msg}
	}
}
