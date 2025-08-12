package squash

import (
	"context"
	"fmt"
	"strings"
)

const (
	squashPrompt = `You are a Git commit message expert who values absolute clarity.

Task: Squash the Git commit messages I provide into a single, comprehensive commit message that strictly follows the Conventional Commits specification.

Input commit messages:
{{.CommitMessages}}

Rules:
1. Identify the correct change type (feat, fix, refactor, docs, style, test, or chore) and, if appropriate, add an optional scope in parentheses.
2. Write the first line as <type>(scope?): <subject>:
   - Use the imperative mood.
   - Limit to ≤ 72 characters.
3. Insert **one blank line** after the first line.
4. Craft the body using **up to 6 bullet points**, each beginning with "- ":
   - Group related changes and summarize significant points.
   - Preserve the context and intent of the original commits.
   - Limit each bullet to ≤ 100 characters.
5. Preserve all issue/PR identifiers exactly as written (e.g., #123, JIRA-456).
6. Keep the tone professional and technical.

**IMPORTANT:** Output **only** the consolidated commit message itself—no prefixes, explanations, or meta-text. Start directly with the commit type and message.
`
)

// ClientInterface defines the client interface required by squash
type ClientInterface interface {
	GenerateCommitMessage(ctx context.Context, prompt string) (string, error)
}

// Squash provides functionality to merge multiple commit messages
type Squash struct {
	client ClientInterface
	lang   string
}

// New creates a new Squash instance
func New(client ClientInterface, lang string) *Squash {
	return &Squash{
		client: client,
		lang:   lang,
	}
}

// Generate generates a consolidated commit message from multiple commit messages
func (s *Squash) Generate(ctx context.Context, messages []string) (string, error) {
	// Validate input
	if len(messages) < 2 {
		return "", fmt.Errorf("at least 2 commit messages are required")
	}

	// Prepare prompt
	prompt := s.buildPrompt(messages)

	// Call LLM
	response, err := s.client.GenerateCommitMessage(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	// Clean output to ensure no unnecessary prefixes
	cleaned := cleanCommitMessage(response)
	return cleaned, nil
}

// buildPrompt constructs the LLM prompt
func (s *Squash) buildPrompt(messages []string) string {
	// Select language template
	template := squashPrompt
	if s.lang == "zh" {
		template += "\nOutput as Chinese."
	}
	// Format commit messages
	formattedMessages := ""
	for i, msg := range messages {
		formattedMessages += fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(msg))
	}

	// Replace placeholder in template
	prompt := strings.Replace(template, "{{.CommitMessages}}", formattedMessages, 1)

	return prompt
}

// ValidateMessages validates the input commit messages
func (s *Squash) ValidateMessages(messages []string) error {
	if len(messages) < 2 {
		return fmt.Errorf("at least 2 commit messages are required")
	}

	// Check if any message is empty
	for i, msg := range messages {
		if strings.TrimSpace(msg) == "" {
			return fmt.Errorf("commit message %d is empty", i+1)
		}
	}

	// Check total length (rough token estimation)
	totalLength := 0
	for _, msg := range messages {
		totalLength += len(msg)
	}

	// Assume each character is about 0.25 tokens, leave enough space for prompt and response
	if totalLength > 12000 { // ~3000 tokens
		return fmt.Errorf("total input is too long, please reduce the number or length of messages")
	}

	return nil
}

// cleanCommitMessage cleans the commit message and removes possible prefixes
func cleanCommitMessage(message string) string {
	// Remove common AI prefixes
	prefixes := []string{
		"Here's the consolidated commit message:",
		"Here is the consolidated commit message:",
		"The consolidated commit message is:",
		"Consolidated commit message:",
		"Here's the commit message:",
		"Here is the commit message:",
		"The commit message is:",
		// Chinese versions
		"合并后的提交信息：",
		"这是合并后的提交信息：",
		"提交信息：",
		"以下是合并后的提交信息：",
		"生成的提交信息：",
	}

	cleaned := strings.TrimSpace(message)

	// Try to remove various prefixes (case-insensitive)
	for _, prefix := range prefixes {
		if strings.HasPrefix(strings.ToLower(cleaned), strings.ToLower(prefix)) {
			cleaned = strings.TrimSpace(cleaned[len(prefix):])
			break
		}
	}

	// If the message is surrounded by quotes, remove them
	if len(cleaned) > 2 {
		firstChar := cleaned[0]
		lastChar := cleaned[len(cleaned)-1]
		if (firstChar == '"' && lastChar == '"') ||
			(firstChar == '\'' && lastChar == '\'') ||
			(firstChar == '`' && lastChar == '`') {
			cleaned = cleaned[1 : len(cleaned)-1]
		}
	}

	// Final trim of whitespace
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}
