package squash

import (
	"context"
	"fmt"
	"strings"
)

const (
	squashPromptEN = `You are a Git commit message expert who values absolute clarity.

Task: Squash the Git commit messages I provide into a single, comprehensive commit message that strictly follows the Conventional Commits specification.

Input commit messages:
{{.CommitMessages}}

Rules:
1. Identify the correct change type (feat, fix, refactor, docs, style, test, or chore) and, if appropriate, add an optional scope in parentheses.
2. Write the first line as <type>(scope?): <subject>:
   - Use the imperative mood.
   - Limit to ≤ 72 characters.
3. Insert **one blank line** after the first line.
4. Craft the body using **up to 3 bullet points**, each beginning with "- ":
   - Group related changes and summarize significant points.
   - Preserve the context and intent of the original commits.
   - Limit each bullet to ≤ 100 characters.
5. Preserve all issue/PR identifiers exactly as written (e.g., #123, JIRA-456).
6. Keep the tone professional and technical.

**IMPORTANT:** Output **only** the consolidated commit message itself—no prefixes, explanations, or meta-text. Start directly with the commit type and message.
`

	squashPromptZH = `你是一名注重绝对清晰度的 Git 提交信息专家。

**任务：** 将我提供的一组 Git 提交信息进行 squash，生成一条完整且严格符合 Conventional Commits 规范的提交信息。

**输入提交信息：**
{{.CommitMessages}}

**规则：**

1. 确定正确的变更类型（feat、fix、refactor、docs、style、test、chore），必要时可在括号中添加可选 scope。
2. 首行格式为 <type>(scope?): <subject>：

   * 使用祈使句。
   * 限制在 ≤ 72 个字符内。
3. 首行后插入**一个空行**。
4. 正文使用**最多 3 条**项目符号，每条以 "- "开头：

   * 将相关变更归类并概括关键点。
   * 保留原始提交的上下文与意图。
   * 每条 ≤ 100 个字符。
5. 按原样保留所有 issue/PR 标识（如 #123、JIRA-456）。
6. 保持专业且技术化的语气。

**重要：** 仅输出合并后的提交信息本身——不要包含任何前缀、解释或元文本。直接以提交类型和信息开头。
`
)

// ClientInterface 定义了 squash 需要的客户端接口
type ClientInterface interface {
	GenerateCommitMessage(ctx context.Context, prompt string) (string, error)
}

// Squash 提供合并多个 commit messages 的功能
type Squash struct {
	client ClientInterface
	lang   string
}

// New 创建一个新的 Squash 实例
func New(client ClientInterface, lang string) *Squash {
	return &Squash{
		client: client,
		lang:   lang,
	}
}

// Generate 生成合并后的 commit message
func (s *Squash) Generate(ctx context.Context, messages []string) (string, error) {
	// 验证输入
	if len(messages) < 2 {
		return "", fmt.Errorf("at least 2 commit messages are required")
	}

	// 准备 prompt
	prompt := s.buildPrompt(messages)

	// 调用 LLM
	response, err := s.client.GenerateCommitMessage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	// 清理输出，确保没有不需要的前缀
	cleaned := cleanCommitMessage(response)
	return cleaned, nil
}

// buildPrompt 构建 LLM prompt
func (s *Squash) buildPrompt(messages []string) string {
	// 选择语言模板
	template := squashPromptEN
	if s.lang == "zh" {
		template = squashPromptZH
	}

	// 格式化 commit messages
	formattedMessages := ""
	for i, msg := range messages {
		formattedMessages += fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(msg))
	}

	// 替换模板中的占位符
	prompt := strings.Replace(template, "{{.CommitMessages}}", formattedMessages, 1)

	return prompt
}

// ValidateMessages 验证输入的 commit messages
func (s *Squash) ValidateMessages(messages []string) error {
	if len(messages) < 2 {
		return fmt.Errorf("at least 2 commit messages are required")
	}

	// 检查每个 message 是否为空
	for i, msg := range messages {
		if strings.TrimSpace(msg) == "" {
			return fmt.Errorf("commit message %d is empty", i+1)
		}
	}

	// 检查总长度（粗略的 token 估算）
	totalLength := 0
	for _, msg := range messages {
		totalLength += len(msg)
	}

	// 假设平均每个字符约 0.25 个 token，留出足够空间给 prompt 和响应
	if totalLength > 12000 { // ~3000 tokens
		return fmt.Errorf("total input is too long, please reduce the number or length of messages")
	}

	return nil
}

// cleanCommitMessage 清理 commit message，移除可能的前缀
func cleanCommitMessage(message string) string {
	// 移除常见的 AI 前缀
	prefixes := []string{
		"Here's the consolidated commit message:",
		"Here is the consolidated commit message:",
		"The consolidated commit message is:",
		"Consolidated commit message:",
		"Here's the commit message:",
		"Here is the commit message:",
		"The commit message is:",
		// 中文版本
		"合并后的提交信息：",
		"这是合并后的提交信息：",
		"提交信息：",
		"以下是合并后的提交信息：",
		"生成的提交信息：",
	}
	
	cleaned := strings.TrimSpace(message)
	
	// 尝试移除各种前缀
	for _, prefix := range prefixes {
		// 不区分大小写的检查
		if strings.HasPrefix(strings.ToLower(cleaned), strings.ToLower(prefix)) {
			cleaned = strings.TrimSpace(cleaned[len(prefix):])
			break
		}
	}
	
	// 如果消息被引号包围，去掉引号
	if len(cleaned) > 2 {
		firstChar := cleaned[0]
		lastChar := cleaned[len(cleaned)-1]
		if (firstChar == '"' && lastChar == '"') ||
		   (firstChar == '\'' && lastChar == '\'') ||
		   (firstChar == '`' && lastChar == '`') {
			cleaned = cleaned[1:len(cleaned)-1]
		}
	}
	
	// 最后再次清理空白
	cleaned = strings.TrimSpace(cleaned)
	
	return cleaned
}