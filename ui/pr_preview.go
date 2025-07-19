package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PRPreviewData 包含PR预览所需的数据
type PRPreviewData struct {
	Title       string
	Body        string
	Base        string
	Head        string
	Remote      string
	Provider    string
	IsDraft     bool
	HasChanges  bool
	FileChanges []FileChange
	
	// 模板相关
	UsingTemplate bool   // 是否使用了模板
	TemplateName  string // 模板名称
}

// FileChange 表示文件变更信息
type FileChange struct {
	Path       string
	Additions  int
	Deletions  int
	ChangeType string // "added", "modified", "deleted"
}

// PRPreviewModel PR预览组件
type PRPreviewModel struct {
	data        PRPreviewData
	styles      UIStyles
	showDetails bool
	width       int
}

// NewPRPreviewModel 创建新的PR预览模型
func NewPRPreviewModel(data PRPreviewData, styles UIStyles, width int) *PRPreviewModel {
	return &PRPreviewModel{
		data:        data,
		styles:      styles,
		showDetails: false,
		width:       width,
	}
}

// ToggleDetails 切换详细信息显示
func (m *PRPreviewModel) ToggleDetails() {
	m.showDetails = !m.showDetails
}

// View 渲染PR预览界面
func (m *PRPreviewModel) View() string {
	var content strings.Builder

	// PR标题部分
	titleStyle := m.styles.Title
	content.WriteString(titleStyle.Render("Pull Request Preview") + "\n\n")

	// 基本信息
	infoStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray)
	content.WriteString(m.renderInfoLine("Provider", m.data.Provider, infoStyle))
	content.WriteString(m.renderInfoLine("Remote", m.data.Remote, infoStyle))
	content.WriteString(m.renderInfoLine("From", m.data.Head, infoStyle))
	content.WriteString(m.renderInfoLine("To", m.data.Base, infoStyle))
	
	if m.data.IsDraft {
		draftStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Yellow)
		content.WriteString(m.renderInfoLine("Status", "Draft", draftStyle))
	}
	
	// 显示是否使用了模板
	if m.data.UsingTemplate {
		templateStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Blue)
		templateName := m.data.TemplateName
		if templateName == "" {
			templateName = "Default"
		}
		content.WriteString(m.renderInfoLine("Template", templateName, templateStyle))
	}
	
	content.WriteString("\n")

	// PR标题
	content.WriteString(m.styles.CommitType.Render("Title: "))
	content.WriteString(m.styles.CommitDesc.Render(m.data.Title) + "\n\n")

	// PR内容预览
	if m.data.Body != "" {
		content.WriteString(m.styles.CommitType.Render("Description:") + "\n")
		bodyLines := strings.Split(m.data.Body, "\n")
		maxLines := 5
		if m.showDetails {
			maxLines = len(bodyLines)
		}
		
		for i, line := range bodyLines {
			if i >= maxLines {
				remainingLines := len(bodyLines) - maxLines
				hintStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Italic(true)
				content.WriteString(hintStyle.Render(fmt.Sprintf("  ... %d more lines ...", remainingLines)) + "\n")
				break
			}
			content.WriteString("  " + m.styles.CommitBody.Render(line) + "\n")
		}
		content.WriteString("\n")
	}

	// 文件变更摘要
	if len(m.data.FileChanges) > 0 {
		content.WriteString(m.renderFileChanges())
	}

	// 操作提示
	hintStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Italic(true)
	if !m.showDetails && m.data.Body != "" && len(strings.Split(m.data.Body, "\n")) > 5 {
		content.WriteString(hintStyle.Render("[D] Show details") + "  ")
	} else if m.showDetails {
		content.WriteString(hintStyle.Render("[D] Hide details") + "  ")
	}
	content.WriteString(hintStyle.Render("[Enter] Continue  [Esc] Cancel") + "\n")

	return content.String()
}

// renderInfoLine 渲染信息行
func (m *PRPreviewModel) renderInfoLine(label, value string, style lipgloss.Style) string {
	labelStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Width(10)
	return labelStyle.Render(label+":") + " " + style.Render(value) + "\n"
}

// renderFileChanges 渲染文件变更摘要
func (m *PRPreviewModel) renderFileChanges() string {
	var content strings.Builder
	
	content.WriteString(m.styles.CommitType.Render("Changes:") + "\n")
	
	// 统计变更
	totalAdditions := 0
	totalDeletions := 0
	for _, fc := range m.data.FileChanges {
		totalAdditions += fc.Additions
		totalDeletions += fc.Deletions
	}
	
	// 显示摘要
	summaryStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray)
	addStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Green)
	delStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Red)
	
	summary := fmt.Sprintf("  %d files changed, ", len(m.data.FileChanges))
	content.WriteString(summaryStyle.Render(summary))
	content.WriteString(addStyle.Render(fmt.Sprintf("+%d", totalAdditions)))
	content.WriteString(summaryStyle.Render(", "))
	content.WriteString(delStyle.Render(fmt.Sprintf("-%d", totalDeletions)))
	content.WriteString("\n")
	
	// 显示前几个文件
	maxFiles := 3
	if m.showDetails {
		maxFiles = len(m.data.FileChanges)
	}
	
	for i, fc := range m.data.FileChanges {
		if i >= maxFiles {
			remainingFiles := len(m.data.FileChanges) - maxFiles
			hintStyle := lipgloss.NewStyle().Foreground(m.styles.Colors.Gray).Italic(true)
			content.WriteString(hintStyle.Render(fmt.Sprintf("  ... %d more files ...", remainingFiles)) + "\n")
			break
		}
		
		// 文件图标
		icon := m.getChangeIcon(fc.ChangeType)
		iconStyle := m.getChangeStyle(fc.ChangeType)
		
		// 文件路径（截断过长路径）
		path := fc.Path
		maxPathLen := m.width - 20 // 留出空间给变更统计
		if len(path) > maxPathLen {
			path = "..." + path[len(path)-maxPathLen+3:]
		}
		
		content.WriteString("  ")
		content.WriteString(iconStyle.Render(icon))
		content.WriteString(" ")
		content.WriteString(path)
		
		// 变更统计
		if fc.Additions > 0 || fc.Deletions > 0 {
			content.WriteString(" ")
			if fc.Additions > 0 {
				content.WriteString(addStyle.Render(fmt.Sprintf("+%d", fc.Additions)))
			}
			if fc.Additions > 0 && fc.Deletions > 0 {
				content.WriteString(" ")
			}
			if fc.Deletions > 0 {
				content.WriteString(delStyle.Render(fmt.Sprintf("-%d", fc.Deletions)))
			}
		}
		content.WriteString("\n")
	}
	content.WriteString("\n")
	
	return content.String()
}

// getChangeIcon 获取变更类型图标
func (m *PRPreviewModel) getChangeIcon(changeType string) string {
	switch changeType {
	case "added":
		return "+"
	case "deleted":
		return "-"
	case "modified":
		return "●"
	default:
		return "○"
	}
}

// getChangeStyle 获取变更类型样式
func (m *PRPreviewModel) getChangeStyle(changeType string) lipgloss.Style {
	switch changeType {
	case "added":
		return lipgloss.NewStyle().Foreground(m.styles.Colors.Green)
	case "deleted":
		return lipgloss.NewStyle().Foreground(m.styles.Colors.Red)
	case "modified":
		return lipgloss.NewStyle().Foreground(m.styles.Colors.Yellow)
	default:
		return lipgloss.NewStyle().Foreground(m.styles.Colors.Gray)
	}
}