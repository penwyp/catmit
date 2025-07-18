package prompt

import (
	"context"
	"fmt"
	"strings"
	
	"github.com/penwyp/catmit/collector"
)

// CollectorInterface 定义collector接口，用于获取Git数据
type CollectorInterface interface {
	FileStatusSummary(ctx context.Context) (*collector.FileStatusSummary, error)
	ComprehensiveDiff(ctx context.Context) (string, error)
	BranchName(ctx context.Context) (string, error)
	RecentCommits(ctx context.Context, n int) ([]string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
}

// TokenBudget 定义token预算配置
type TokenBudget struct {
	MaxTokens       int // 最大token数
	ReservedTokens  int // 为系统prompt等预留的token数
	AvailableTokens int // 可用于diff内容的token数
}

// Builder 负责构建发送给 LLM 的 Prompt 文本。
// 支持语言注入、token预算控制与智能diff截断。
type Builder struct {
	lang        string      // ISO 639-1 语言代码，例如 "en", "zh"
	diffLimit   int         // Diff 最大长度（字节），0 表示不限制
	truncMarker string      // 截断标记，可自定义，便于测试
	tokenBudget TokenBudget // Token预算配置
}

// NewBuilder 创建 Prompt Builder。
// diffLimit == 0 表示不对 diff 做截断。
func NewBuilder(lang string, diffLimit int) *Builder {
	return &Builder{
		lang:        lang,
		diffLimit:   diffLimit,
		truncMarker: "(diff truncated)",
		tokenBudget: TokenBudget{
			MaxTokens:       8000, // 默认预算
			ReservedTokens:  2000, // 为系统prompt和其他信息预留
			AvailableTokens: 6000, // 可用于diff内容
		},
	}
}

// NewBuilderWithTokenBudget 创建带有指定token预算的Prompt Builder
func NewBuilderWithTokenBudget(lang string, diffLimit int, maxTokens int) *Builder {
	reservedTokens := maxTokens / 4 // 预留25%的token
	return &Builder{
		lang:        lang,
		diffLimit:   diffLimit,
		truncMarker: "(diff truncated)",
		tokenBudget: TokenBudget{
			MaxTokens:       maxTokens,
			ReservedTokens:  reservedTokens,
			AvailableTokens: maxTokens - reservedTokens,
		},
	}
}

// estimateTokens 估算文本的token数量
// 简化算法：1个token约等于4个字符（英文），2个字符（中文）
func estimateTokens(text string) int {
	charCount := len(text)
	// 简化估算：平均每个token 3个字符
	return (charCount + 2) / 3
}


// smartTruncateDiff 智能截断单个文件的diff
// 如果diff太大，使用头尾保留法
func (b *Builder) smartTruncateDiff(diff string, maxTokens int) string {
	if estimateTokens(diff) <= maxTokens {
		return diff
	}
	
	lines := strings.Split(diff, "\n")
	if len(lines) <= 20 { // 小于20行，直接返回
		return diff
	}
	
	// 计算头尾行数，保留关键信息
	headLines := maxTokens / 6 // 约1/3的token用于头部
	tailLines := maxTokens / 6 // 约1/3的token用于尾部
	
	if headLines > len(lines)/3 {
		headLines = len(lines) / 3
	}
	if tailLines > len(lines)/3 {
		tailLines = len(lines) / 3
	}
	
	head := strings.Join(lines[:headLines], "\n")
	tail := strings.Join(lines[len(lines)-tailLines:], "\n")
	
	return fmt.Sprintf("%s\n\n--- Diff truncated (showing %d head + %d tail lines) ---\n\n%s",
		head, headLines, tailLines, tail)
}

// BuildSystemPrompt 构建系统提示词，包含角色定义、任务说明、格式规则和示例。
// 根据 docs/prompt-analyze.md 最佳实践，遵循"大师级"prompt模板结构。
func (b *Builder) BuildSystemPrompt() string {
	// ROLE - 角色与身份设定
	rolePrompt := "You are an expert software engineer who writes concise, high-quality Git commit messages following the Conventional Commits specification."
	
	// TASK - 任务描述
	taskPrompt := "Generate a Git commit message for the provided code changes."
	
	// 语言指令
	var langInst string
	switch strings.ToLower(b.lang) {
	case "zh":
		langInst = "The commit message MUST be in Chinese."
	default:
		langInst = "The commit message MUST be in English."
	}
	
	// INSTRUCTIONS & RULES - 格式与规则
	formatRules := `# INSTRUCTIONS & RULES
1. **Format**: MUST follow Conventional Commits: <type>(<scope>): <subject>
2. **Type**: Choose from feat, fix, refactor, chore, docs, style, test
3. **Subject**: Use imperative mood, max 50 chars, no period at the end
4. **Body**: If needed, explain the 'why', not the 'how', after a blank line`

	// EXAMPLE - 示例 (Few-Shot Learning)
	examples := `# EXAMPLE
- **Diff**: + return sessionStorage.getItem('token'); - return localStorage.getItem('token');
- **Commit**: refactor(auth): use sessionStorage for token storage`

	// YOUR RESPONSE - 输出要求
	outputReq := `# YOUR RESPONSE
Generate ONLY the commit message text.`
	
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

// BuildUserPromptWithBudget 使用token预算和文件优先级构建智能的用户提示词
// 实现文档建议的智能数据预处理和token预算控制
func (b *Builder) BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error) {
	// Convert the interface to CollectorInterface
	col := collector.(CollectorInterface)
	var parts []string
	
	// 种子文本
	if seed != "" {
		parts = append(parts, "Seed: "+seed)
	}
	
	// 获取文件状态摘要
	summary, err := col.FileStatusSummary(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get file status summary: %w", err)
	}
	
	// 分支信息
	if summary.BranchName != "" {
		parts = append(parts, "Branch: "+summary.BranchName)
	}
	
	// 构建文件摘要
	if len(summary.Files) > 0 {
		var fileSummary []string
		for _, file := range summary.Files {
			status := string(file.IndexStatus)
			if file.WorkStatus != ' ' && file.WorkStatus != 0 {
				status += string(file.WorkStatus)
			}
			if file.IsRenamed {
				fileSummary = append(fileSummary, fmt.Sprintf("%s: %s -> %s", status, file.OldPath, file.Path))
			} else {
				fileSummary = append(fileSummary, fmt.Sprintf("%s: %s", status, file.Path))
			}
		}
		parts = append(parts, "Summary of Staged Files:\n"+strings.Join(fileSummary, "\n"))
	}
	
	// 获取最近的提交历史
	commits, err := col.RecentCommits(ctx, 3)
	if err == nil && len(commits) > 0 {
		parts = append(parts, "Recent commits:\n"+strings.Join(commits, "\n"))
	}
	
	// 使用token预算控制的diff内容
	diffContent, err := b.buildBudgetedDiff(ctx, col, summary.Files)
	if err != nil {
		return "", fmt.Errorf("failed to build diff content: %w", err)
	}
	
	if diffContent != "" {
		parts = append(parts, "Git diff (may be truncated for large files):\n```diff\n"+diffContent+"\n```")
	}
	
	if len(parts) == 0 {
		return "No changes detected.", nil
	}
	
	return strings.Join(parts, "\n\n"), nil
}

// buildBudgetedDiff 根据token预算和文件优先级构建diff内容
func (b *Builder) buildBudgetedDiff(ctx context.Context, collector CollectorInterface, files []collector.FileStatus) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	
	// 获取完整的diff
	fullDiff, err := collector.ComprehensiveDiff(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}
	
	// 如果diff很小，直接返回
	if estimateTokens(fullDiff) <= b.tokenBudget.AvailableTokens {
		return fullDiff, nil
	}
	
	// 超出预算，使用智能截断
	return b.smartTruncateDiff(fullDiff, b.tokenBudget.AvailableTokens), nil
}

// Build 生成最终 Prompt（保持向后兼容）。
// 已废弃：建议使用 BuildSystemPrompt() 和 BuildUserPrompt() 分别构建。
func (b *Builder) Build(seed string, diff string, commits []string, branch string, files []string) string {
	system := b.BuildSystemPrompt()
	user := b.BuildUserPrompt(seed, diff, commits, branch, files)
	return system + "\n\n" + user
}
