package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CommitStage 表示commit/push操作的阶段
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

// CommitModel 用于显示commit和push操作的进度，保持与ReviewModel一致的视觉风格
type CommitModel struct {
	stage        CommitStage
	message      string  // commit message
	lang         string  // 语言设置
	enablePush   bool    // 是否启用push
	stageAll     bool    // 是否stage all
	spinner      spinner.Model
	
	// 操作接口
	committer    commitInterface
	ctx          context.Context
	
	// 状态管理
	err          error
	done         bool
	
	// 响应式终端尺寸支持  
	terminalWidth  int
	terminalHeight int
	
	// 显示控制
	showDuration   time.Duration // 最终状态显示时长
	finalStartTime time.Time     // 最终状态开始时间
}

// commitInterface 定义commit和push操作接口
type commitInterface interface {
	Commit(ctx context.Context, message string) error
	Push(ctx context.Context) error
	StageAll(ctx context.Context) error
	HasStagedChanges(ctx context.Context) bool
	CreatePullRequest(ctx context.Context) error
}

// NewCommitModel 创建新的CommitModel
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
		terminalWidth:  80,  // 默认宽度
		terminalHeight: 24,  // 默认高度
		showDuration:   1500 * time.Millisecond, // 最终状态显示1.5秒
	}
}

// Init 实现 tea.Model 接口
func (m *CommitModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.startCommit())
}

// Update 处理消息
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
			// 没有push，直接进入完成状态
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

// calculateContentWidth 计算基于终端宽度的动态内容宽度（复用ReviewModel的逻辑）
func (m *CommitModel) calculateContentWidth() int {
	const (
		minWidth = 60  // 最小宽度
		maxWidth = 120 // 最大宽度
		margin   = 4   // 左右边距
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

// View 渲染界面
func (m *CommitModel) View() string {
	// 调色板（与ReviewModel保持一致）
	const (
		cGray   = lipgloss.Color("245")
		cBlue   = lipgloss.Color("39")
		cGreen  = lipgloss.Color("42")
		cYellow = lipgloss.Color("220")
		cWhite  = lipgloss.Color("255")
	)
	
	contentWidth := m.calculateContentWidth()
	
	// 样式定义
	borderStyle := lipgloss.NewStyle().Foreground(cBlue)
	titleStyle := lipgloss.NewStyle().Foreground(cWhite).Bold(true)
	langStyle := lipgloss.NewStyle().Foreground(cGray)
	
	// 状态样式
	progressStyle := lipgloss.NewStyle().Foreground(cYellow)
	successStyle := lipgloss.NewStyle().Foreground(cGreen)
	
	// 辅助函数：行渲染器（与ReviewModel保持一致）
	renderLine := func(content string) string {
		contentDisplayWidth := lipgloss.Width(content)
		if contentDisplayWidth > contentWidth {
			content = content[:contentWidth-3] + "..."
			contentDisplayWidth = lipgloss.Width(content)
		}
		
		linePadding := contentWidth - contentDisplayWidth
		if linePadding < 0 {
			linePadding = 0
		}
		return borderStyle.Render("│") + content + strings.Repeat(" ", linePadding) + borderStyle.Render("│")
	}
	
	// 构建标题
	titleText := titleStyle.Render("Commit Progress") + langStyle.Render(fmt.Sprintf(" (%s)", m.lang))
	titlePadding := contentWidth - lipgloss.Width(titleText)
	if titlePadding < 0 {
		titlePadding = 0
	}
	header := borderStyle.Render("┌") + strings.Repeat(borderStyle.Render("─"), titlePadding/2) +
		titleText + strings.Repeat(borderStyle.Render("─"), titlePadding-titlePadding/2) +
		borderStyle.Render("┐")
	
	// 构建内容
	var contentLines []string
	
	// 显示commit message（截断显示）
	messagePreview := m.message
	if len(messagePreview) > contentWidth-4 {
		messagePreview = messagePreview[:contentWidth-7] + "..."
	}
	contentLines = append(contentLines, renderLine(" "+titleStyle.Render("Message: ")+messagePreview))
	contentLines = append(contentLines, renderLine("")) // 空行
	
	// 根据阶段显示状态
	switch m.stage {
	case CommitStageInit, CommitStageCommitting:
		statusLine := " " + m.spinner.View() + " " + progressStyle.Render("Committing changes...")
		contentLines = append(contentLines, renderLine(statusLine))
	case CommitStageCommitted:
		statusLine := " ✓ " + successStyle.Render("Committed successfully")
		contentLines = append(contentLines, renderLine(statusLine))
		if m.enablePush {
			statusLine = " " + m.spinner.View() + " " + progressStyle.Render("Preparing to push...")
			contentLines = append(contentLines, renderLine(statusLine))
		}
	case CommitStagePushing:
		statusLine := " ✓ " + successStyle.Render("Committed successfully")
		contentLines = append(contentLines, renderLine(statusLine))
		statusLine = " " + m.spinner.View() + " " + progressStyle.Render("Pushing to remote...")
		contentLines = append(contentLines, renderLine(statusLine))
	case CommitStagePushed, CommitStageDone:
		statusLine := " ✓ " + successStyle.Render("Committed successfully")
		contentLines = append(contentLines, renderLine(statusLine))
		if m.enablePush {
			statusLine = " ✓ " + successStyle.Render("Pushed successfully")
			contentLines = append(contentLines, renderLine(statusLine))
		}
	}
	
	// 构建底部边框
	bottomBorder := borderStyle.Render("└" + strings.Repeat("─", contentWidth) + "┘")
	
	// 组装最终输出
	result := []string{header}
	result = append(result, contentLines...)
	result = append(result, bottomBorder)
	
	return strings.Join(result, "\n")
}

// IsDone 返回操作是否完成
func (m *CommitModel) IsDone() (bool, error) {
	return m.done, m.err
}

// --- 命令和消息类型 ---

type commitDoneMsg struct {
	err error
}

type pushDoneMsg struct {
	err error
}

type finalTimeoutMsg struct{}

// startCommit 开始commit操作
func (m *CommitModel) startCommit() tea.Cmd {
	return func() tea.Msg {
		m.stage = CommitStageCommitting
		err := m.committer.Commit(m.ctx, m.message)
		return commitDoneMsg{err: err}
	}
}

// startPush 开始push操作
func (m *CommitModel) startPush() tea.Cmd {
	return func() tea.Msg {
		err := m.committer.Push(m.ctx)
		return pushDoneMsg{err: err}
	}
}