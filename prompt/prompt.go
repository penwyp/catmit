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

// BuildSystemPrompt 构建系统提示词，包含角色定义、任务说明、格式规则和示例。
// 根据 docs/prompt-analyze.md 最佳实践，系统提示词应包含指令和规则。
func (b *Builder) BuildSystemPrompt() string {
	// 角色与身份设定
	rolePrompt := "You are an expert software engineer and a master of writing concise, high-quality Git commit messages. You adhere strictly to the Conventional Commits specification."
	
	// 任务描述
	taskPrompt := "Your task is to analyze the provided git diff and context information to generate a Git commit message."
	
	// 语言指令
	var langInst string
	switch strings.ToLower(b.lang) {
	case "zh":
		langInst = "The commit message MUST be in Chinese."
	default:
		langInst = "The commit message MUST be in English."
	}
	
	// 格式规则
	formatRules := `You MUST follow these rules:

1. **Format**: Adhere to the Conventional Commits specification (<type>[optional scope]: <description>).
2. **Type**: Choose the most appropriate type from: feat, fix, refactor, docs, style, perf, test, build, ci, chore.
3. **Scope**: If changes are limited to a specific component/module, include scope. Otherwise, omit it.
4. **Description (Subject)**:
   - Write a short, imperative summary (e.g., "add user login", not "adds user login").
   - Maximum 50 characters.
   - Lowercase first letter, no period at the end.
5. **Body (Optional)**:
   - If the change is non-trivial, explain the 'why' behind the change.
   - Separate from subject with a blank line.
   - Wrap lines at 72 characters.
6. **Breaking Changes**: Add footer starting with "BREAKING CHANGE: " if applicable.`

	// 示例
	examples := `# EXAMPLES
## Example 1: Simple fix
- **Context**: A typo fix in variable name
- **Commit**: fix(parser): correct typo in 'username' variable

## Example 2: New feature with body
- **Context**: Adds new API endpoint
- **Commit**:
feat(api): add endpoint to retrieve user list

This introduces a new GET /api/v1/users endpoint that allows
authenticated users to fetch a paginated list of users.

This is the first step towards the user management dashboard.`

	// 输出要求
	outputReq := "Generate ONE complete git commit message for the provided context and diff. Output only the raw commit message text and nothing else."
	
	return strings.Join([]string{rolePrompt, taskPrompt, langInst, formatRules, examples, outputReq}, "\n\n")
}

// BuildUserPrompt 构建用户提示词，包含上下文数据（分支、文件、提交历史、diff）。
// 根据 docs/prompt-analyze.md 最佳实践，用户提示词应包含实际数据。
func (b *Builder) BuildUserPrompt(seed string, diff string, commits []string, branch string, files []string) string {
	var parts []string
	
	// 种子文本
	if seed != "" {
		parts = append(parts, "Seed: "+seed)
	}
	
	// 上下文信息
	if branch != "" {
		parts = append(parts, "Branch: "+branch)
	}
	if len(files) > 0 {
		parts = append(parts, "Changed files: "+strings.Join(files, ", "))
	}
	if len(commits) > 0 {
		parts = append(parts, "Recent commits:\n"+strings.Join(commits, "\n"))
	}
	
	// 处理 diff 截断
	diffPart := diff
	if b.diffLimit > 0 && len(diff) > b.diffLimit {
		half := b.diffLimit / 2
		diffPart = diff[:half] + "\n" + b.truncMarker + "\n" + diff[len(diff)-half:]
	}
	
	// 添加 diff
	if diffPart != "" {
		parts = append(parts, "Git diff:\n```diff\n"+diffPart+"\n```")
	}
	
	if len(parts) == 0 {
		return "No changes detected."
	}
	
	return strings.Join(parts, "\n\n")
}

// Build 生成最终 Prompt（保持向后兼容）。
// 已废弃：建议使用 BuildSystemPrompt() 和 BuildUserPrompt() 分别构建。
func (b *Builder) Build(seed string, diff string, commits []string, branch string, files []string) string {
	system := b.BuildSystemPrompt()
	user := b.BuildUserPrompt(seed, diff, commits, branch, files)
	return system + "\n\n" + user
}
