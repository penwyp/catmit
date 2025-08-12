package prompt

import (
	"context"
	"fmt"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/pkg/gitinfo"
)

// CollectorInterface defines the collector interface for obtaining Git data
type CollectorInterface interface {
	FileStatusSummary(ctx context.Context) (*gitinfo.FileStatusSummary, error)
	ComprehensiveDiff(ctx context.Context) (string, error)
	BranchName(ctx context.Context) (string, error)
	RecentCommits(ctx context.Context, n int) ([]string, error)
	ChangedFiles(ctx context.Context) ([]string, error)
}

// TokenBudget defines the token budget configuration
type TokenBudget struct {
	MaxTokens       int // Maximum number of tokens
	ReservedTokens  int // Tokens reserved for system prompt, etc.
	AvailableTokens int // Tokens available for diff content
}

// Builder is responsible for constructing the Prompt text sent to the LLM.
// Supports language injection, token budget control, and smart diff truncation.
type Builder struct {
	lang        string      // ISO 639-1 language code, e.g., "en", "zh"
	diffLimit   int         // Maximum diff length (bytes), 0 means no limit
	truncMarker string      // Truncation marker, customizable for testing
	tokenBudget TokenBudget // Token budget configuration
}

// NewBuilder creates a Prompt Builder.
// diffLimit == 0 means no truncation for diff.
func NewBuilder(lang string, diffLimit int) *Builder {
	// Validate language code
	if lang != "" && lang != "en" && lang != "zh" {
		// Warning: unsupported language, default to English
		lang = "en"
	}
	return &Builder{
		lang:        lang,
		diffLimit:   diffLimit,
		truncMarker: "(diff truncated)",
		tokenBudget: TokenBudget{
			MaxTokens:       8000, // Default budget
			ReservedTokens:  2000, // Reserved for system prompt and other info
			AvailableTokens: 6000, // Available for diff content
		},
	}
}


// estimateTokens estimates the number of tokens in the text
// Simplified algorithm: 1 token ≈ 4 English chars, 2 Chinese chars
func estimateTokens(text string) int {
	charCount := len(text)
	// Simplified estimate: average 1 token per 3 chars
	return (charCount + 2) / 3
}

// smartTruncateDiff intelligently truncates a single file's diff
// If the diff is too large, use head-tail retention
func (b *Builder) smartTruncateDiff(diff string, maxTokens int) string {
	if estimateTokens(diff) <= maxTokens {
		return diff
	}

	lines := strings.Split(diff, "\n")
	if len(lines) <= 20 { // Less than 20 lines, return as is
		return diff
	}

	// Calculate number of head and tail lines to retain key info
	headLines := maxTokens / 6 // About 1/3 of tokens for head
	tailLines := maxTokens / 6 // About 1/3 of tokens for tail

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

// BuildSystemPrompt constructs the system prompt, including role definition, task description, format rules, and examples.
// Follows "master-level" prompt template structure as per docs/prompt-analyze.md best practices.
func (b *Builder) BuildSystemPrompt() string {
	// ROLE - Role and identity setting
	rolePrompt := "You are an expert software engineer who writes concise, high-quality Git commit messages following the Conventional Commits specification."

	// TASK - Task description
	taskPrompt := "Generate a Git commit message for the provided code changes."

	// Language instruction
	var langInst string
	switch strings.ToLower(b.lang) {
	case "zh":
		langInst = "The commit message MUST be in Chinese."
	default:
		langInst = "The commit message MUST be in English."
	}

	// INSTRUCTIONS & RULES - Format and rules
	formatRules := `# INSTRUCTIONS & RULES
1. **Format**: MUST follow Conventional Commits: <type>(<scope>): <subject>
2. **Type**: Choose from feat, fix, refactor, chore, docs, style, test
3. **Subject**: Use imperative mood, max 50 chars, no period at the end
4. **Body**: If needed, explain the 'why', not the 'how', after a blank line`

	// EXAMPLE - Example (Few-Shot Learning)
	examples := `# EXAMPLE
- **Diff**: + return sessionStorage.getItem('token'); - return localStorage.getItem('token');
- **Commit**: refactor(auth): use sessionStorage for token storage`

	// YOUR RESPONSE - Output requirement
	outputReq := `# YOUR RESPONSE
Generate ONLY the commit message text.`

	return strings.Join([]string{rolePrompt, taskPrompt, langInst, formatRules, examples, outputReq}, "\n\n")
}

// BuildUserPrompt constructs the user prompt, including contextual data (branch, files, commit history, diff).
// According to docs/prompt-analyze.md best practices, user prompt should include real data.
func (b *Builder) BuildUserPrompt(seed string, diff string, commits []string, branch string, files []string) string {
	var parts []string

	// Seed text
	if seed != "" {
		parts = append(parts, "Seed: "+seed)
	}

	// Context info
	if branch != "" {
		parts = append(parts, "Branch: "+branch)
	}
	if len(files) > 0 {
		parts = append(parts, "Changed files: "+strings.Join(files, ", "))
	}
	if len(commits) > 0 {
		parts = append(parts, "Recent commits:\n"+strings.Join(commits, "\n"))
	}

	// Handle diff truncation
	diffPart := diff
	if b.diffLimit > 0 && len(diff) > b.diffLimit {
		half := b.diffLimit / 2
		diffPart = diff[:half] + "\n" + b.truncMarker + "\n" + diff[len(diff)-half:]
	}

	// Add diff
	if diffPart != "" {
		parts = append(parts, "Git diff:\n```diff\n"+diffPart+"\n```")
	}

	if len(parts) == 0 {
		return "No changes detected."
	}

	return strings.Join(parts, "\n\n")
}

// BuildUserPromptWithBudget constructs an intelligent user prompt using token budget and file priority
// Implements smart data preprocessing and token budget control as suggested in documentation
func (b *Builder) BuildUserPromptWithBudget(ctx context.Context, collector interface{}, seed string) (string, error) {
	// Convert the interface to CollectorInterface
	col := collector.(CollectorInterface)
	var parts []string

	// Seed text
	if seed != "" {
		parts = append(parts, "Seed: "+seed)
	}

	// Get file status summary
	summary, err := col.FileStatusSummary(ctx)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get file status summary", err)
	}

	// Branch info
	if summary.BranchName != "" {
		parts = append(parts, "Branch: "+summary.BranchName)
	}

	// Build file summary
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

	// Get recent commit history
	commits, err := col.RecentCommits(ctx, 3)
	if err == nil && len(commits) > 0 {
		parts = append(parts, "Recent commits:\n"+strings.Join(commits, "\n"))
	}

	// Diff content controlled by token budget
	diffContent, err := b.buildBudgetedDiff(ctx, col, summary.Files)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to build diff content", err)
	}

	if diffContent != "" {
		parts = append(parts, "Git diff (may be truncated for large files):\n```diff\n"+diffContent+"\n```")
	}

	if len(parts) == 0 {
		return "No changes detected.", nil
	}

	return strings.Join(parts, "\n\n"), nil
}

// buildBudgetedDiff constructs diff content based on token budget and file priority
func (b *Builder) buildBudgetedDiff(ctx context.Context, collector CollectorInterface, files []gitinfo.FileStatus) (string, error) {
	if len(files) == 0 {
		return "", nil
	}

	// Get the full diff
	fullDiff, err := collector.ComprehensiveDiff(ctx)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get diff", err)
	}

	// If the diff is small, return as is
	if estimateTokens(fullDiff) <= b.tokenBudget.AvailableTokens {
		return fullDiff, nil
	}

	// Exceeds budget, use smart truncation
	return b.smartTruncateDiff(fullDiff, b.tokenBudget.AvailableTokens), nil
}

// Build generates the final Prompt (kept for backward compatibility).
// Deprecated: use BuildSystemPrompt() and BuildUserPrompt() separately.
func (b *Builder) Build(seed string, diff string, commits []string, branch string, files []string) string {
	system := b.BuildSystemPrompt()
	user := b.BuildUserPrompt(seed, diff, commits, branch, files)
	return system + "\n\n" + user
}

// BuildPRSystemPrompt builds the system prompt for PR generation
func (b *Builder) BuildPRSystemPrompt() string {
	langPart := ""
	switch b.lang {
	case "zh":
		langPart = " 用中文撰写PR标题和描述。"
	case "en":
		langPart = " Write PR title and description in English."
	default:
		langPart = ""
	}

	return `You are an AI assistant that generates pull request titles and descriptions based on commit history.
Generate a concise PR title (one line) and a detailed description explaining the changes.
Format: First line is the title, followed by a blank line, then the description.` + langPart
}

// BuildPRUserPrompt builds the user prompt for PR generation
func (b *Builder) BuildPRUserPrompt(commits []string) string {
	if len(commits) == 0 {
		return "No commits found."
	}

	prompt := "Based on these commits, generate a PR title and description:\n\n"
	prompt += "Commits:\n"
	for _, commit := range commits {
		prompt += commit + "\n"
	}

	return prompt
}
