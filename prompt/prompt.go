package prompt

import (
	"strings"
)

// Builder 负责构建发送给 LLM 的 Prompt 文本。
// 支持语言注入与 Diff 截断。
type Builder struct {
	lang        string // ISO 639-1 语言代码，例如 "en", "zh"
	diffLimit   int    // Diff 最大长度（字节），0 表示不限制
	truncMarker string // 截断标记，可自定义，便于测试
}

// NewBuilder 创建 Prompt Builder。
// diffLimit == 0 表示不对 diff 做截断。
func NewBuilder(lang string, diffLimit int) *Builder {
	return &Builder{
		lang:        lang,
		diffLimit:   diffLimit,
		truncMarker: "(diff truncated)",
	}
}

// Build 生成最终 Prompt。
// seed 可为空；commits 允许为空；diff 必须为完整 diff 文本（可能为空）。
func (b *Builder) Build(seed string, diff string, commits []string, branch string, files []string) string {
	// 语言指令部分
	var langInst string
	switch strings.ToLower(b.lang) {
	case "zh":
		langInst = "请用中文撰写一条符合 Conventional Commits 规范的 Git commit message。"
	default:
		langInst = "Generate a conventional commit message in English."
	}

	// branch & files & commits
	var contextPart strings.Builder
	if branch != "" {
		contextPart.WriteString("Branch: " + branch + "\n")
	}
	if len(files) > 0 {
		contextPart.WriteString("Changed files: " + strings.Join(files, ", ") + "\n")
	}
	if len(commits) > 0 {
		contextPart.WriteString("Recent commits:\n" + strings.Join(commits, "\n") + "\n")
	}
	if contextPart.Len() > 0 {
		contextPart.WriteString("\n")
	}

	// 处理 diff 截断
	diffPart := diff
	if b.diffLimit > 0 && len(diff) > b.diffLimit {
		half := b.diffLimit / 2
		diffPart = diff[:half] + "\n" + b.truncMarker + "\n" + diff[len(diff)-half:]
	}

	// 拼接 Prompt
	var sb strings.Builder
	sb.WriteString(langInst + "\n\n")

	if seed != "" {
		sb.WriteString("Seed: " + seed + "\n\n")
	}

	if contextPart.Len() > 0 {
		sb.WriteString(contextPart.String())
	}

	if diffPart != "" {
		sb.WriteString("Diff:\n```diff\n")
		sb.WriteString(diffPart)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}
