package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
)

// LLMGenerator generates PR titles and bodies using LLM
type LLMGenerator struct {
	llmClient LLMInterface
	log       logger.Logger
}

// NewLLMGenerator creates a new LLM generator
func NewLLMGenerator(llmClient LLMInterface) *LLMGenerator {
	return &LLMGenerator{
		llmClient: llmClient,
		log:       logger.NewDefault(),
	}
}

// GeneratePRTitle generates a PR title from analysis data
func (g *LLMGenerator) GeneratePRTitle(ctx context.Context, data *PRAnalysisData) (string, error) {
	g.log.Debugf("Generating PR title with %d commits", len(data.Commits))

	var prompt string
	
	if len(data.Commits) > 0 {
		// Generate from commits
		prompt = g.buildTitlePromptFromCommits(data)
	} else {
		// Fallback to branch name
		prompt = g.buildTitlePromptFromBranch(data.BranchName)
	}

	// Call LLM - using empty system prompt since we embed instructions in user prompt
	title, err := g.llmClient.GetCommitMessage(ctx, "", prompt)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to generate PR title", err)
	}

	// Clean and format the title
	title = g.cleanTitle(title)
	
	return title, nil
}

// GeneratePRBody generates a PR body by filling the template
func (g *LLMGenerator) GeneratePRBody(ctx context.Context, template string, data *PRAnalysisData) (string, error) {
	g.log.Debugf("Generating PR body with template")

	// Build prompt for filling the template
	prompt := g.buildBodyPrompt(template, data)

	// Call LLM - using empty system prompt since we embed instructions in user prompt
	body, err := g.llmClient.GetCommitMessage(ctx, "", prompt)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to generate PR body", err)
	}

	// Clean the response
	body = g.cleanBody(body)
	
	return body, nil
}

// buildTitlePromptFromCommits builds a prompt for title generation from commits
func (g *LLMGenerator) buildTitlePromptFromCommits(data *PRAnalysisData) string {
	var sb strings.Builder
	
	sb.WriteString("Generate a concise PR title based on the following commits.\n")
	sb.WriteString("The title should follow the format: [Type] Brief description\n")
	sb.WriteString("Types: Feature, Fix, Refactor, Docs, Chore\n")
	sb.WriteString("Keep the title under 72 characters.\n\n")
	
	sb.WriteString("Commits:\n")
	for _, commit := range data.Commits {
		// Include first line of commit message
		firstLine := strings.Split(commit.Message, "\n")[0]
		sb.WriteString(fmt.Sprintf("- %s\n", firstLine))
	}
	
	if data.DiffStats != "" {
		sb.WriteString("\nChange statistics:\n")
		sb.WriteString(data.DiffStats)
		sb.WriteString("\n")
	}
	
	sb.WriteString("\nGenerate ONLY the PR title, nothing else:")
	
	return sb.String()
}

// buildTitlePromptFromBranch builds a prompt for title generation from branch name
func (g *LLMGenerator) buildTitlePromptFromBranch(branchName string) string {
	var sb strings.Builder
	
	sb.WriteString("Generate a PR title based on the branch name.\n")
	sb.WriteString("The title should follow the format: [Type] Brief description\n")
	sb.WriteString("Types: Feature, Fix, Refactor, Docs, Chore\n")
	sb.WriteString("Keep the title under 72 characters.\n\n")
	
	sb.WriteString(fmt.Sprintf("Branch name: %s\n\n", branchName))
	
	sb.WriteString("Examples:\n")
	sb.WriteString("- feature/user-auth → [Feature] Add user authentication\n")
	sb.WriteString("- fix/email-validation → [Fix] Correct email validation logic\n")
	sb.WriteString("- refactor/jwt-simplify → [Refactor] Simplify JWT token generation\n")
	sb.WriteString("- docs/readme-update → [Docs] Update README documentation\n\n")
	
	sb.WriteString("Generate ONLY the PR title, nothing else:")
	
	return sb.String()
}

// buildBodyPrompt builds a prompt for filling the PR template
func (g *LLMGenerator) buildBodyPrompt(template string, data *PRAnalysisData) string {
	var sb strings.Builder
	
	sb.WriteString("Fill in the following PR template based on the provided information.\n")
	sb.WriteString("Keep the template structure and markdown formatting.\n")
	sb.WriteString("Check appropriate checkboxes with [x] based on the changes.\n")
	sb.WriteString("Be concise but informative.\n\n")
	
	// Provide context
	sb.WriteString("Context:\n")
	sb.WriteString(fmt.Sprintf("Branch: %s\n", data.BranchName))
	
	if len(data.Commits) > 0 {
		sb.WriteString("\nCommit messages:\n")
		for _, commit := range data.Commits {
			sb.WriteString(fmt.Sprintf("---\n%s\n", commit.Message))
		}
	}
	
	if len(data.ChangedFiles) > 0 {
		sb.WriteString("\nChanged files:\n")
		for _, file := range data.ChangedFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", file))
		}
	}
	
	if data.DiffStats != "" {
		sb.WriteString("\nChange statistics:\n")
		sb.WriteString(data.DiffStats)
		sb.WriteString("\n")
	}
	
	// Note about tests and docs
	if data.HasTests {
		sb.WriteString("\nNote: Test files were modified.\n")
	}
	if data.HasDocs {
		sb.WriteString("Note: Documentation files were modified.\n")
	}
	
	sb.WriteString("\nTemplate to fill:\n")
	sb.WriteString("```markdown\n")
	sb.WriteString(template)
	sb.WriteString("\n```\n\n")
	
	sb.WriteString("Fill the template with appropriate content based on the changes. ")
	sb.WriteString("Return ONLY the filled template, nothing else:")
	
	return sb.String()
}

// cleanTitle cleans the generated title
func (g *LLMGenerator) cleanTitle(title string) string {
	// Remove quotes if present
	title = strings.Trim(title, `"'`)
	
	// Remove any markdown formatting
	title = strings.ReplaceAll(title, "#", "")
	title = strings.ReplaceAll(title, "*", "")
	title = strings.ReplaceAll(title, "_", " ")
	
	// Trim whitespace
	title = strings.TrimSpace(title)
	
	// Ensure it starts with [Type] format
	if !strings.HasPrefix(title, "[") {
		// Try to infer type from content
		lowerTitle := strings.ToLower(title)
		if strings.Contains(lowerTitle, "fix") {
			title = "[Fix] " + title
		} else if strings.Contains(lowerTitle, "add") || strings.Contains(lowerTitle, "feature") {
			title = "[Feature] " + title
		} else if strings.Contains(lowerTitle, "refactor") {
			title = "[Refactor] " + title
		} else if strings.Contains(lowerTitle, "doc") || strings.Contains(lowerTitle, "readme") {
			title = "[Docs] " + title
		} else {
			title = "[Chore] " + title
		}
	}
	
	// Truncate if too long
	if len(title) > 72 {
		title = title[:69] + "..."
	}
	
	return title
}

// cleanBody cleans the generated body
func (g *LLMGenerator) cleanBody(body string) string {
	// Remove any wrapper text that LLM might add
	lines := strings.Split(body, "\n")
	inTemplate := false
	var cleanLines []string
	
	for _, line := range lines {
		// Skip lines that look like LLM meta-text
		if strings.HasPrefix(line, "Here is") || strings.HasPrefix(line, "Here's") ||
		   strings.Contains(line, "filled template") || strings.Contains(line, "Based on") {
			continue
		}
		
		// Detect start of actual template content
		if strings.Contains(line, "### 📝 Change Type") || strings.Contains(line, "Change Type") {
			inTemplate = true
		}
		
		if inTemplate || strings.TrimSpace(line) != "" {
			cleanLines = append(cleanLines, line)
		}
	}
	
	// Join and clean
	body = strings.Join(cleanLines, "\n")
	
	// Remove markdown code block wrappers if present
	body = strings.TrimPrefix(body, "```markdown\n")
	body = strings.TrimPrefix(body, "```md\n")
	body = strings.TrimPrefix(body, "```\n")
	body = strings.TrimSuffix(body, "\n```")
	
	// Final trim
	body = strings.TrimSpace(body)
	
	return body
}