package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// UIColors 定义统一的颜色主题
type UIColors struct {
	Gray   lipgloss.Color
	Blue   lipgloss.Color
	Green  lipgloss.Color
	Yellow lipgloss.Color
	Red    lipgloss.Color
	White  lipgloss.Color
	Black  lipgloss.Color
	Orange lipgloss.Color
}

// DefaultColors 返回默认的颜色主题
func DefaultColors() UIColors {
	return UIColors{
		Gray:   lipgloss.Color("245"),
		Blue:   lipgloss.Color("39"),
		Green:  lipgloss.Color("42"),
		Yellow: lipgloss.Color("220"),
		Red:    lipgloss.Color("196"),
		White:  lipgloss.Color("255"),
		Black:  lipgloss.Color("0"),
		Orange: lipgloss.Color("208"),
	}
}

// UIStyles 定义统一的样式
type UIStyles struct {
	Colors       UIColors
	Border       lipgloss.Style
	Title        lipgloss.Style
	Lang         lipgloss.Style
	Success      lipgloss.Style
	Warning      lipgloss.Style
	Error        lipgloss.Style
	Progress     lipgloss.Style
	CommitType   lipgloss.Style
	CommitDesc   lipgloss.Style
	CommitBody   lipgloss.Style
}

// DefaultStyles 返回默认的样式集
func DefaultStyles() UIStyles {
	colors := DefaultColors()
	return UIStyles{
		Colors:       colors,
		Border:       lipgloss.NewStyle().Foreground(colors.Blue),
		Title:        lipgloss.NewStyle().Foreground(colors.White).Bold(true),
		Lang:         lipgloss.NewStyle().Foreground(colors.Gray),
		Success:      lipgloss.NewStyle().Foreground(colors.Green),
		Warning:      lipgloss.NewStyle().Foreground(colors.Yellow),
		Error:        lipgloss.NewStyle().Foreground(colors.Red),
		Progress:     lipgloss.NewStyle().Foreground(colors.Yellow),
		CommitType:   lipgloss.NewStyle().Foreground(colors.Yellow),
		CommitDesc:   lipgloss.NewStyle().Foreground(colors.White),
		CommitBody:   lipgloss.NewStyle().Foreground(colors.Gray),
	}
}

// truncateContent 智能截断内容，保留重要信息
func truncateContent(content string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	// 如果内容长度在限制内，直接返回
	if lipgloss.Width(content) <= maxWidth {
		return content
	}

	// 逐个字符检查，确保截断后的宽度不超过限制
	var result strings.Builder
	for _, r := range content {
		testStr := result.String() + string(r)
		if lipgloss.Width(testStr) > maxWidth {
			break
		}
		result.WriteRune(r)
	}

	return result.String()
}

// wordWrap 包装文本，支持CJK字符
func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}

	if s == "" {
		return ""
	}

	var finalResult strings.Builder
	paragraphs := strings.Split(s, "\n")

	for i, paragraph := range paragraphs {
		if strings.TrimSpace(paragraph) == "" {
			if i > 0 {
				finalResult.WriteString("\n")
			}
			continue
		}

		// 使用 Lipgloss 的文本包装能力，支持 CJK 字符
		wrappedParagraph := wrapParagraph(paragraph, width)
		finalResult.WriteString(wrappedParagraph)

		if i < len(paragraphs)-1 {
			finalResult.WriteString("\n")
		}
	}
	return finalResult.String()
}

// wrapParagraph 包装单个段落，支持 CJK 字符和智能换行
func wrapParagraph(paragraph string, width int) string {
	var result strings.Builder
	var line strings.Builder
	words := strings.Fields(paragraph)

	for _, word := range words {
		// 检查当前行是否为空
		if line.Len() == 0 {
			line.WriteString(word)
		} else {
			// 计算添加空格和新词后的宽度
			testLine := line.String() + " " + word
			testWidth := lipgloss.Width(testLine)

			if testWidth <= width {
				line.WriteString(" ")
				line.WriteString(word)
			} else {
				// 当前行满了，换行
				result.WriteString(line.String() + "\n")
				line.Reset()
				line.WriteString(word)
			}
		}

		// 如果单个词太长，需要强制换行
		if lipgloss.Width(line.String()) > width {
			result.WriteString(line.String() + "\n")
			line.Reset()
		}
	}

	// 添加最后一行
	if line.Len() > 0 {
		result.WriteString(line.String())
	}

	return result.String()
}

// Button 表示一个可交互的按钮
type Button struct {
	Hint       string
	Text       string
	HintStyle  lipgloss.Style
	TextStyle  lipgloss.Style
	SelectedBg lipgloss.Color
}

// RenderButton 渲染单个按钮
func RenderButton(b Button, isSelected bool) string {
	hStyle := b.HintStyle
	tStyle := b.TextStyle

	if isSelected {
		colors := DefaultColors()
		fgColor := colors.Black
		// 红色背景上白色文字更清晰
		if b.SelectedBg == colors.Red {
			fgColor = colors.White
		}
		hStyle = hStyle.Copy().Background(b.SelectedBg).Foreground(fgColor)
		tStyle = tStyle.Copy().Background(b.SelectedBg).Foreground(fgColor)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top,
		hStyle.Padding(0, 1).Render(b.Hint),
		tStyle.Padding(0, 1).Render(b.Text),
	)
}

// RenderProgressBar 渲染进度条
func RenderProgressBar(current, total int, width int, color lipgloss.Color) string {
	if total <= 0 || width <= 10 {
		return ""
	}

	percentage := float64(current) / float64(total)
	filledWidth := int(percentage * float64(width-2))

	style := lipgloss.NewStyle().Foreground(color)
	filled := strings.Repeat("█", filledWidth)
	empty := strings.Repeat("─", width-2-filledWidth)

	return style.Render("[" + filled + empty + "]")
}

// RenderStatusLine 渲染状态行
func RenderStatusLine(icon, text string, style lipgloss.Style) string {
	return icon + " " + style.Render(text)
}

// CalculateContentWidth 计算响应式内容宽度
func CalculateContentWidth(terminalWidth int) int {
	const (
		minWidth = 60
		maxWidth = 120
		margin   = 4
	)

	availableWidth := terminalWidth - margin

	if availableWidth < minWidth {
		return minWidth
	}
	if availableWidth > maxWidth {
		return maxWidth
	}

	return availableWidth
}

// RenderBorder 渲染边框元素
func RenderBorder(element string, style lipgloss.Style) string {
	return style.Render(element)
}

// CenterText 居中文本
func CenterText(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}

	padding := (width - textWidth) / 2
	return strings.Repeat(" ", padding) + text + strings.Repeat(" ", width-textWidth-padding)
}