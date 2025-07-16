package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penwyp/catmit/collector"
)

// Phase 表示主模型所处的阶段
type Phase int

const (
	PhaseLoading Phase = iota
	PhaseReview
	PhaseCommit
	PhaseDone
)

// MainModel 统一的单视图模型，管理整个生命周期
type MainModel struct {
	// 状态管理
	phase          Phase
	loadingStage   Stage
	reviewDecision Decision
	commitStage    CommitStage

	// UI组件
	spinner        spinner.Model
	textArea       textarea.Model
	selectedButton buttonState
	editing        bool

	// 数据
	message        string
	seed           string
	lang           string

	// 依赖注入
	ctx         context.Context
	collector   collectorInterface
	promptBuild promptInterface
	client      clientInterface
	committer   commitInterface

	// 配置
	enablePush bool
	stageAll   bool
	apiTimeout time.Duration
	createPR   bool

	// 响应式设计
	terminalWidth  int
	terminalHeight int

	// 错误和结果
	err   error
	done  bool
	prURL string

	// 内部状态
	finalStartTime time.Time
	showDuration   time.Duration

	// UI样式
	styles UIStyles
}

// NewMainModel 创建新的统一模型
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
	sp := spinner.New()
	sp.Spinner = spinner.Line

	ta := textarea.New()
	ta.Placeholder = "Edit commit message..."
	ta.CharLimit = 1000
	ta.ShowLineNumbers = false

	return &MainModel{
		phase:          PhaseLoading,
		loadingStage:   StageCollect,
		spinner:        sp,
		textArea:       ta,
		selectedButton: buttonAccept,
		ctx:            ctx,
		collector:      col,
		promptBuild:    pb,
		client:         cli,
		committer:      com,
		seed:           seed,
		lang:           lang,
		apiTimeout:     apiTimeout,
		enablePush:     enablePush,
		stageAll:       stageAll,
		createPR:       createPR,
		terminalWidth:  80,
		terminalHeight: 24,
		showDuration:   1500 * time.Millisecond,
		styles:         DefaultStyles(),
	}
}

// Init 启动第一个阶段
func (m *MainModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, collectCmd(m.collector, m.ctx))
}

// Update 处理消息
func (m *MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.terminalWidth = msg.Width
		m.terminalHeight = msg.Height
		m.textArea.SetWidth(CalculateContentWidth(m.terminalWidth) - 4)
		m.textArea.SetHeight(8)
		return m, nil

	case tea.KeyMsg:
		// 全局快捷键处理
		if msg.String() == "ctrl+c" {
			m.err = context.Canceled
			m.done = true
			return m, tea.Quit
		}

		// 根据phase处理不同的键盘输入
		switch m.phase {
		case PhaseReview:
			return m.updateReview(msg)
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	// Loading阶段的消息处理
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
		m.phase = PhaseReview
		m.textArea.SetValue(m.message)
		return m, nil

	// Commit阶段的消息处理
	case commitDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}
		m.commitStage = CommitStageCommitted
		if m.enablePush {
			// 添加延迟以确保CommitStageCommitted状态有时间完整渲染
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
			var prExists *ErrPRAlreadyExists
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
		m.commitStage = CommitStageCommitting
		return m, m.startCommit()

	case errorMsg:
		m.err = msg.err
		m.done = true
		return m, tea.Quit
	}

	// 处理editing模式下的textarea更新
	if m.editing && m.phase == PhaseReview {
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// updateReview 处理Review阶段的键盘输入
func (m *MainModel) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editing {
		switch msg.String() {
		case "esc":
			m.editing = false
			m.textArea.Blur()
			return m, nil
		case "ctrl+s":
			m.message = strings.TrimSpace(m.textArea.Value())
			m.editing = false
			m.textArea.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.textArea, cmd = m.textArea.Update(msg)
			return m, cmd
		}
	}

	// 非编辑模式的键盘处理
	switch msg.String() {
	case "left", "h", "up", "k":
		m.selectedButton--
		if m.selectedButton < buttonAccept {
			m.selectedButton = buttonCancel
		}
	case "right", "l", "down", "j":
		m.selectedButton++
		if m.selectedButton > buttonCancel {
			m.selectedButton = buttonAccept
		}
	case "a", "A":
		m.reviewDecision = DecisionAccept
		// 添加延迟来平滑过渡到commit阶段
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return startCommitPhaseMsg{}
		})
	case "e", "E":
		m.editing = true
		m.textArea.Focus()
		return m, textarea.Blink
	case "c", "C", "q", "Q", "esc":
		m.reviewDecision = DecisionCancel
		m.done = true
		return m, tea.Quit
	case "enter", " ":
		switch m.selectedButton {
		case buttonAccept:
			m.reviewDecision = DecisionAccept
			// 添加延迟来平滑过渡到commit阶段
			return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
				return startCommitPhaseMsg{}
			})
		case buttonEdit:
			m.editing = true
			m.textArea.Focus()
			return m, textarea.Blink
		case buttonCancel:
			m.reviewDecision = DecisionCancel
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View 渲染统一的界面
func (m *MainModel) View() string {
	contentWidth := CalculateContentWidth(m.terminalWidth)

	// 统一的边框容器
	content := m.renderContainer(func() string {
		switch m.phase {
		case PhaseLoading:
			return m.renderLoadingContent()
		case PhaseReview:
			return m.renderReviewContent()
		case PhaseCommit:
			return m.renderCommitContent()
		default:
			return ""
		}
	}, contentWidth)

	return content
}

// renderContainer 渲染统一的边框容器
func (m *MainModel) renderContainer(contentFunc func() string, width int) string {
	// 动态标题
	title := m.getPhaseTitle()
	titleText := m.styles.Title.Render(title) + m.styles.Lang.Render(fmt.Sprintf(" (%s)", m.lang))
	titlePadding := width - lipgloss.Width(titleText)
	if titlePadding < 0 {
		titlePadding = 0
	}

	header := RenderBorder("┌", m.styles.Border) + 
		strings.Repeat(RenderBorder("─", m.styles.Border), titlePadding/2) +
		titleText + 
		strings.Repeat(RenderBorder("─", m.styles.Border), titlePadding-titlePadding/2) +
		RenderBorder("┐", m.styles.Border)

	// 获取内容
	content := contentFunc()

	// 渲染内容行
	var bodyBuilder strings.Builder
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		bodyBuilder.WriteString(m.renderLine(line, width) + "\n")
	}

	// 移除末尾多余的换行符
	body := strings.TrimRight(bodyBuilder.String(), "\n")

	// 底部边框
	bottomBorder := RenderBorder("└", m.styles.Border) + 
		strings.Repeat(RenderBorder("─", m.styles.Border), width) + 
		RenderBorder("┘", m.styles.Border)

	return strings.Join([]string{header, body, bottomBorder}, "\n") + "\n"
}

// renderLine 渲染单行内容
func (m *MainModel) renderLine(content string, width int) string {
	contentDisplayWidth := lipgloss.Width(content)
	if contentDisplayWidth > width {
		content = truncateContent(content, width-3) + "..."
		contentDisplayWidth = lipgloss.Width(content)
	}

	linePadding := width - contentDisplayWidth
	if linePadding < 0 {
		linePadding = 0
	}

	return RenderBorder("│", m.styles.Border) + content + strings.Repeat(" ", linePadding) + RenderBorder("│", m.styles.Border)
}

// renderLoadingContent 渲染加载阶段的内容
func (m *MainModel) renderLoadingContent() string {
	var statusStyle lipgloss.Style
	var status string

	switch m.loadingStage {
	case StageCollect:
		status = "Collecting diff…"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	case StagePreprocess:
		status = "Preprocessing files…"
		statusStyle = lipgloss.NewStyle().Foreground(m.styles.Colors.Orange)
	case StagePrompt:
		status = "Crafting prompt…"
		statusStyle = lipgloss.NewStyle().Foreground(m.styles.Colors.Blue)
	case StageQuery:
		status = "Generating commit message…"
		statusStyle = lipgloss.NewStyle().Foreground(m.styles.Colors.Green)
	default:
		status = "Processing…"
		statusStyle = lipgloss.NewStyle().Foreground(m.styles.Colors.Gray)
	}

	return " " + m.spinner.View() + " " + statusStyle.Render(status)
}

// renderReviewContent 渲染审查阶段的内容
func (m *MainModel) renderReviewContent() string {
	if m.editing {
		return m.renderEditingContent()
	}

	var content strings.Builder

	// 渲染commit message
	lines := strings.Split(m.message, "\n")
	if len(lines) > 0 {
		parts := strings.SplitN(lines[0], ":", 2)
		var subject string
		if len(parts) == 2 {
			subject = m.styles.CommitType.Render(parts[0]+":") + m.styles.CommitDesc.Render(parts[1])
		} else {
			subject = m.styles.CommitDesc.Render(lines[0])
		}
		content.WriteString(" " + subject + "\n")
	}

	if len(lines) > 1 {
		content.WriteString("\n")
		bodyText := strings.Join(lines[1:], "\n")
		wrappedBody := wordWrap(bodyText, CalculateContentWidth(m.terminalWidth)-2)
		for _, l := range strings.Split(wrappedBody, "\n") {
			content.WriteString(" " + m.styles.CommitBody.Render(l) + "\n")
		}
	}

	content.WriteString("\n")

	// 渲染按钮
	buttons := m.renderButtons()
	content.WriteString(" " + buttons)

	return content.String()
}

// renderEditingContent 渲染编辑模式的内容
func (m *MainModel) renderEditingContent() string {
	promptStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Yellow)
	hintStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Italic(true)

	var content strings.Builder
	content.WriteString(" " + promptStyle.Render("Edit Commit Message:") + "\n\n")
	
	// 渲染textarea的每一行
	lines := strings.Split(m.textArea.View(), "\n")
	for _, line := range lines {
		content.WriteString(" " + line + "\n")
	}
	
	content.WriteString("\n " + hintStyle.Render("[Ctrl+S] Save  [Esc] Cancel"))

	return content.String()
}

// renderCommitContent 渲染提交阶段的内容
func (m *MainModel) renderCommitContent() string {

	var content strings.Builder

	// 显示commit message预览
	messagePreview := m.message
	maxWidth := CalculateContentWidth(m.terminalWidth) - 4
	if len(messagePreview) > maxWidth {
		messagePreview = messagePreview[:maxWidth-3] + "..."
	}
	content.WriteString(" " + m.styles.Title.Render("Message: ") + messagePreview + "\n\n")

	// 根据阶段显示状态
	switch m.commitStage {
	case CommitStageInit, CommitStageCommitting:
		content.WriteString(" " + m.spinner.View() + " " + m.styles.Progress.Render("Committing changes..."))
	case CommitStageCommitted:
		content.WriteString(" ✓ " + m.styles.Success.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n " + m.spinner.View() + " " + m.styles.Progress.Render("Preparing to push..."))
		}
	case CommitStagePushing:
		content.WriteString(" ✓ " + m.styles.Success.Render("Committed successfully"))
		content.WriteString("\n " + m.spinner.View() + " " + m.styles.Progress.Render("Pushing to remote..."))
	case CommitStagePushFailed:
		content.WriteString(" ✓ " + m.styles.Success.Render("Committed successfully"))
		if m.enablePush {
			errorText := "Push failed"
			if m.err != nil {
				// Extract meaningful error message, limit length for display
				errStr := m.err.Error()
				if len(errStr) > 80 {
					errStr = errStr[:80] + "..."
				}
				errorText = fmt.Sprintf("Push failed: %s", errStr)
			}
			content.WriteString("\n ✗ " + m.styles.Error.Render(errorText))
		}
	case CommitStagePushed:
		content.WriteString(" ✓ " + m.styles.Success.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n ✓ " + m.styles.Success.Render("Pushed successfully"))
		}
		if m.createPR {
			content.WriteString("\n " + m.spinner.View() + " " + m.styles.Progress.Render("Preparing to create PR..."))
		}
	case CommitStageCreatingPR:
		content.WriteString(" ✓ " + m.styles.Success.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n ✓ " + m.styles.Success.Render("Pushed successfully"))
		}
		content.WriteString("\n " + m.spinner.View() + " " + m.styles.Progress.Render("Creating pull request..."))
	case CommitStagePRFailed:
		content.WriteString(" ✓ " + m.styles.Success.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n ✓ " + m.styles.Success.Render("Pushed successfully"))
		}
		if m.createPR {
			errorText := "Pull request creation failed"
			if m.err != nil {
				// Extract meaningful error message, limit length for display
				errStr := m.err.Error()
				if len(errStr) > 80 {
					errStr = errStr[:80] + "..."
				}
				errorText = fmt.Sprintf("Pull request creation failed: %s", errStr)
			}
			content.WriteString("\n ✗ " + m.styles.Error.Render(errorText))
		}
	case CommitStagePRCreated, CommitStageDone:
		content.WriteString(" ✓ " + m.styles.Success.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n ✓ " + m.styles.Success.Render("Pushed successfully"))
		}
		if m.createPR {
			content.WriteString("\n ✓ " + m.styles.Success.Render("Pull request created successfully"))
			if m.prURL != "" {
				content.WriteString("\n   " + m.styles.CommitDesc.Render(m.prURL))
			}
		}
	}

	return content.String()
}

// renderButtons 渲染按钮组
func (m *MainModel) renderButtons() string {
	colors := m.styles.Colors
	
	buttons := []Button{
		{
			Hint:       "[A]",
			Text:       "Accept",
			HintStyle:  lipgloss.NewStyle().Foreground(colors.Gray),
			TextStyle:  lipgloss.NewStyle().Foreground(colors.Green),
			SelectedBg: colors.Green,
		},
		{
			Hint:       "[E]",
			Text:       "Edit",
			HintStyle:  lipgloss.NewStyle().Foreground(colors.Gray),
			TextStyle:  lipgloss.NewStyle().Foreground(colors.Yellow),
			SelectedBg: colors.Yellow,
		},
		{
			Hint:       "[C]",
			Text:       "Cancel",
			HintStyle:  lipgloss.NewStyle().Foreground(colors.Gray),
			TextStyle:  lipgloss.NewStyle().Foreground(colors.Red),
			SelectedBg: colors.Red,
		},
	}

	var rendered []string
	for i, btn := range buttons {
		isSelected := int(m.selectedButton) == i
		rendered = append(rendered, RenderButton(btn, isSelected))
	}

	return strings.Join(rendered, "  ")
}

// getPhaseTitle 获取当前阶段的标题
func (m *MainModel) getPhaseTitle() string {
	switch m.phase {
	case PhaseLoading:
		return "Generating Message"
	case PhaseReview:
		if m.editing {
			return "Edit Message"
		}
		return "Commit Preview"
	case PhaseCommit:
		return "Commit Progress"
	default:
		return "Catmit"
	}
}


// startCommit 开始提交
func (m *MainModel) startCommit() tea.Cmd {
	return func() tea.Msg {
		// 在commit之前，检查是否需要staging并执行
		if m.stageAll && !m.committer.HasStagedChanges(m.ctx) {
			if err := m.committer.StageAll(m.ctx); err != nil {
				return commitDoneMsg{err: fmt.Errorf("staging failed: %w", err)}
			}
		}
		err := m.committer.Commit(m.ctx, m.message)
		return commitDoneMsg{err: err}
	}
}

// startPush 开始推送
func (m *MainModel) startPush() tea.Cmd {
	return func() tea.Msg {
		err := m.committer.Push(m.ctx)
		return pushDoneMsg{err: err}
	}
}

// startCreatePR 开始创建PR
func (m *MainModel) startCreatePR() tea.Cmd {
	return func() tea.Msg {
		prURL, err := m.committer.CreatePullRequest(m.ctx)
		return createPRDoneMsg{err: err, prURL: prURL}
	}
}

// IsDone 返回操作是否完成及相关信息
func (m *MainModel) IsDone() (bool, Decision, string, error) {
	return m.done, m.reviewDecision, m.message, m.err
}

// GetError 返回错误信息
func (m *MainModel) GetError() error {
	if m.err == collector.ErrNoDiff {
		return m.err
	}
	if m.err == context.Canceled {
		return nil
	}
	// 如果commit成功但push失败，不返回错误（因为主要操作已成功）
	// push失败已经在TUI中显示给用户，不需要再次输出到终端
	if m.commitStage == CommitStagePushFailed {
		return nil
	}
	return m.err
}

// 消息类型定义
type delayedPushMsg struct{}
type delayedCreatePRMsg struct{}
type startCommitPhaseMsg struct{}

type createPRDoneMsg struct {
	err   error
	prURL string
}

// ErrPRAlreadyExists is returned when a PR already exists for the branch
type ErrPRAlreadyExists struct {
	URL string
}

func (e *ErrPRAlreadyExists) Error() string {
	return fmt.Sprintf("pull request already exists: %s", e.URL)
}