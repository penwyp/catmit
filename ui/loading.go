package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Stage 表示进度阶段
type Stage int

const (
	StageCollect Stage = iota
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
}

type promptInterface interface {
	Build(seed, diff string, commits []string, branch string, files []string) string
	BuildSystemPrompt() string
	BuildUserPrompt(seed, diff string, commits []string, branch string, files []string) string
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
		// 进入 Prompt 阶段
		m.stage = StagePrompt
		return m, buildPromptCmd(m.promptBuild, msg.seed, msg.diff, msg.commits, msg.branch, msg.files)
	case promptBuiltMsg:
		// 进入 Query 阶段
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
	seed    string
	diff    string
	commits []string
	branch  string
	files   []string
}

type promptBuiltMsg struct {
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

func buildPromptCmd(pb promptInterface, seed, diff string, commits []string, branch string, files []string) tea.Cmd {
	return func() tea.Msg {
		systemPrompt := pb.BuildSystemPrompt()
		userPrompt := pb.BuildUserPrompt(seed, diff, commits, branch, files)
		return promptBuiltMsg{systemPrompt: systemPrompt, userPrompt: userPrompt}
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
