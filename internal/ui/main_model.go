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

// Phase 表示主模型所处的阶段
type Phase int

const (
	PhaseLoading Phase = iota
	PhaseReview
	PhasePRPreview
	PhaseCommit
	PhaseDone
)

// MainModel 统一的单视图模型，管理整个生命周期
type MainModel struct {
	BaseModel

	// 状态管理
	phase          Phase
	loadingStage   Stage
	reviewDecision Decision
	commitStage    CommitStage

	// UI组件
	spinner  spinner.Model
	textArea textarea.Model
	editing  bool

	// 数据
	message string
	seed    string
	lang    string

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
	prRemote   string
	prBase     string
	prDraft    bool
	prProvider string

	// 内部状态
	finalStartTime time.Time
	showDuration   time.Duration

	// PR预览相关
	prPreview     *PRPreviewModel
	prPreviewData PRPreviewData
	prURL         string

	// 模板相关
	useTemplate bool // 是否尝试使用模板
}

// PRConfig PR配置
type PRConfig struct {
	CreatePR    bool
	Remote      string
	Base        string
	Draft       bool
	Provider    string
	UseTemplate bool // 是否使用模板
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
	// 使用默认PR配置
	prConfig := PRConfig{
		CreatePR: createPR,
		Remote:   "origin",
		Base:     "",
		Draft:    false,
		Provider: "",
	}
	return NewMainModelWithPRConfig(ctx, col, pb, cli, com, seed, lang, apiTimeout, enablePush, stageAll, prConfig)
}

// NewMainModelWithPRConfig 创建带PR配置的统一模型
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
		BaseModel:    NewBaseModel("Generating Message", nil, false),
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
	}

	// Set content renderer
	m.SetContentRenderer(m.renderContent)

	return m
}

// Init 启动第一个阶段
func (m *MainModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, collectCmd(m.collector, m.ctx))
}

// Update 处理消息
func (m *MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.HandleWindowSize(msg)
		m.textArea.SetWidth(CalculateContentWidth(m.width) - 4)
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
		case PhasePRPreview:
			return m.updatePRPreview(msg)
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

// regenerateCommitMessage 触发重新生成commit消息
func (m *MainModel) regenerateCommitMessage() tea.Cmd {
	// 重置状态到加载阶段
	m.phase = PhaseLoading
	m.loadingStage = StagePrompt
	// 重新构建prompt并查询
	return buildSmartPromptCmd(m.promptBuild, m.collector, m.ctx, m.seed)
}

// View 渲染统一的界面
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
	// 如果需要创建PR，先进入PR预览阶段
	if m.createPR {
		return m.preparePRPreview()
	}
	// 否则直接进入commit阶段
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

// renderLoadingContent 渲染加载阶段的内容
func (m *MainModel) renderLoadingContent() string {
	colors := DefaultColors()
	var statusStyle lipgloss.Style
	var status string

	switch m.loadingStage {
	case StageCollect:
		status = "Collecting diff…"
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
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

// renderReviewContent 渲染审查阶段的内容
func (m *MainModel) renderReviewContent() string {
	colors := DefaultColors()
	commitTypeStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	commitDescStyle := lipgloss.NewStyle().Foreground(colors.White)
	commitBodyStyle := lipgloss.NewStyle().Foreground(colors.Gray)

	var content strings.Builder

	// 渲染commit message
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

// renderEditingContent 渲染编辑模式的内容
func (m *MainModel) renderEditingContent() string {
	colors := DefaultColors()
	promptStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
	hintStyle := lipgloss.NewStyle().Foreground(colors.Gray).Italic(true)

	var content strings.Builder
	content.WriteString(promptStyle.Render("Edit Commit Message:") + "\n\n")

	// 渲染textarea的每一行
	lines := strings.Split(m.textArea.View(), "\n")
	for _, line := range lines {
		content.WriteString(line + "\n")
	}

	content.WriteString("\n" + hintStyle.Render("[Ctrl+S] Save  [Esc] Cancel"))

	return strings.TrimRight(content.String(), "\n")
}

// renderCommitContent 渲染提交阶段的内容
func (m *MainModel) renderCommitContent() string {

	var content strings.Builder

	// 显示commit message预览
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

	// 根据阶段显示状态
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
		content.WriteString("✓ " + successStyle.Render("Committed successfully"))
		if m.enablePush {
			content.WriteString("\n✓ " + successStyle.Render("Pushed successfully"))
		}
		if m.createPR {
			content.WriteString("\n✓ " + successStyle.Render("Pull request created successfully"))
			if m.prURL != "" {
				content.WriteString("\n  " + descStyle.Render(m.prURL))
			}
		}
	}

	return content.String()
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
	case PhasePRPreview:
		return "Pull Request Preview"
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
				return commitDoneMsg{err: errors.Wrap(errors.ErrTypeGit, "staging failed", err)}
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
	if m.err == gitinfo.ErrNoDiff {
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

type prPreviewReadyMsg struct {
	data PRPreviewData
}

type createPRDoneMsg struct {
	err   error
	prURL string
}

// preparePRPreview 准备PR预览
func (m *MainModel) preparePRPreview() tea.Cmd {
	return func() tea.Msg {
		// 收集PR预览所需的数据
		branchName, _ := m.collector.BranchName(m.ctx)
		changedFiles, _ := m.collector.ChangedFiles(m.ctx)

		// 解析commit message作为PR标题和内容
		lines := strings.Split(m.message, "\n")
		title := lines[0]
		body := ""
		if len(lines) > 1 {
			body = strings.Join(lines[1:], "\n")
			body = strings.TrimSpace(body)
		}

		// 准备文件变更信息
		var fileChanges []FileChange
		for _, file := range changedFiles {
			// 简化处理，实际应该从git获取具体的增删行数
			fileChanges = append(fileChanges, FileChange{
				Path:       file,
				ChangeType: "modified",
			})
		}

		prData := PRPreviewData{
			Title:       title,
			Body:        body,
			Base:        m.prBase,
			Head:        branchName,
			Remote:      m.prRemote,
			Provider:    m.prProvider,
			IsDraft:     m.prDraft,
			HasChanges:  len(fileChanges) > 0,
			FileChanges: fileChanges,
		}

		return prPreviewReadyMsg{data: prData}
	}
}

// renderPRPreviewContent 渲染PR预览内容
func (m *MainModel) renderPRPreviewContent() string {
	if m.prPreview == nil {
		colors := DefaultColors()
		progressStyle := lipgloss.NewStyle().Foreground(colors.Yellow)
		return m.spinner.View() + " " + progressStyle.Render("Preparing PR preview...")
	}

	return m.prPreview.View()
}

// updatePRPreview 处理PR预览阶段的键盘输入
func (m *MainModel) updatePRPreview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "d", "D":
		if m.prPreview != nil {
			m.prPreview.ToggleDetails()
		}
		return m, nil
	case "enter", " ":
		// 继续到commit阶段
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
