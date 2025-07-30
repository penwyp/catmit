package pr

import (
	"context"
	"fmt"

	"github.com/penwyp/catmit/internal/template"
	"go.uber.org/zap"
)

// EnhancedCreator extends Creator with LLM capabilities
type EnhancedCreator struct {
	*Creator
	llmClient         LLMInterface
	prTemplateManager *template.PRTemplateManager
	commitAnalyzer    *CommitAnalyzer
	llmGenerator      *LLMGenerator
}

// NewEnhancedCreator creates a new enhanced PR creator with LLM support
func NewEnhancedCreator(
	git ExtendedGitRunner,
	providerDetector ProviderDetector,
	cliDetector CLIDetector,
	commandBuilder CommandBuilderInterface,
	commandRunner CommandRunner,
	llmClient LLMInterface,
) *EnhancedCreator {
	// Create base creator
	creator := NewCreator(git, providerDetector, cliDetector, commandBuilder, commandRunner)

	// Create enhanced components
	prTemplateManager := template.NewPRTemplateManager()
	commitAnalyzer := NewCommitAnalyzer(git)
	llmGenerator := NewLLMGenerator(llmClient)

	return &EnhancedCreator{
		Creator:           creator,
		llmClient:         llmClient,
		prTemplateManager: prTemplateManager,
		commitAnalyzer:    commitAnalyzer,
		llmGenerator:      llmGenerator,
	}
}

// Create creates a pull request with LLM-enhanced title and body
func (c *EnhancedCreator) Create(ctx context.Context, options CreateOptions) (string, error) {
	// Ensure PR template exists
	if err := c.prTemplateManager.EnsureTemplateExists(); err != nil {
		if c.logger != nil {
			c.logger.Warn("Failed to ensure PR template exists", zap.Error(err))
		}
	}

	// If title or body is empty and we have LLM, generate them
	if (options.Title == "" || options.Body == "") && c.llmClient != nil {
		// Analyze commits for PR
		analysisData, err := c.commitAnalyzer.AnalyzeForPR(ctx, options.Remote, options.BaseBranch)
		if err != nil {
			if c.logger != nil {
				c.logger.Warn("Failed to analyze commits for PR", zap.Error(err))
			}
			// Continue with fallback to branch name
			analysisData = &PRAnalysisData{
				BranchName: "",
			}
			// Try to get branch name
			if branch, err := c.git.GetCurrentBranch(ctx); err == nil {
				analysisData.BranchName = branch
			}
		}

		// Generate title if empty
		if options.Title == "" {
			title, err := c.llmGenerator.GeneratePRTitle(ctx, analysisData)
			if err != nil {
				if c.logger != nil {
					c.logger.Warn("Failed to generate PR title", zap.Error(err))
				}
				// Fallback to branch name
				if analysisData.BranchName != "" {
					options.Title = fmt.Sprintf("PR from %s", analysisData.BranchName)
				} else {
					options.Title = "New Pull Request"
				}
			} else {
				options.Title = title
				if c.logger != nil {
					c.logger.Debug("Generated PR title", zap.String("title", title))
				}
			}
		}

		// Generate body if empty
		if options.Body == "" && options.UseTemplate {
			// Load PR template
			prTemplate, err := c.prTemplateManager.LoadTemplate(ctx)
			if err != nil {
				if c.logger != nil {
					c.logger.Warn("Failed to load PR template", zap.Error(err))
				}
				// Use a simple body
				options.Body = fmt.Sprintf("PR from branch: %s", analysisData.BranchName)
			} else {
				// Generate body using LLM
				body, err := c.llmGenerator.GeneratePRBody(ctx, prTemplate, analysisData)
				if err != nil {
					if c.logger != nil {
						c.logger.Warn("Failed to generate PR body", zap.Error(err))
					}
					// Use template as-is
					options.Body = prTemplate
				} else {
					options.Body = body
					if c.logger != nil {
						c.logger.Debug("Generated PR body using LLM")
					}
				}
			}
		}

		// Disable Fill option since we're providing title and body
		options.Fill = false
	}

	// Call the base Create method
	return c.Creator.Create(ctx, options)
}

// CheckExists delegates to the base Creator's CheckExists method
func (c *EnhancedCreator) CheckExists(ctx context.Context, options CreateOptions) (bool, string, error) {
	return c.Creator.CheckExists(ctx, options)
}
