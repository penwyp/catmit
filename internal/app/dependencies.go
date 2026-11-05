// Package app provides application-level configuration, dependency injection,
// and initialization logic for the catmit CLI tool.
package app

import (
	"fmt"

	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/internal/pr"
	tagging "github.com/penwyp/catmit/internal/tag"
	"github.com/penwyp/catmit/pkg/gitinfo"
	"github.com/penwyp/catmit/pkg/llm"
	"github.com/penwyp/catmit/pkg/prompt"
	"go.uber.org/zap"
)

// Dependencies holds all the application dependencies.
// This struct serves as the central dependency injection container.
type Dependencies struct {
	Logger        *zap.Logger
	CollectorFunc func() *gitinfo.Collector
	PromptFunc    func(lang string) *prompt.Builder
	ClientFunc    func() *llm.Client
	CommitterFunc func() git.Committer
	GitRunnerFunc func() git.Runner
	PRCreatorFunc func() *pr.Creator
}

// NewDependencies creates a new Dependencies instance with default implementations.
// This can be customized for testing by replacing individual functions.
func NewDependencies(logger *zap.Logger, debug bool) *Dependencies {
	return &Dependencies{
		Logger: logger,
		CollectorFunc: func() *gitinfo.Collector {
			return defaultCollectorProvider(debug, logger)
		},
		PromptFunc: func(lang string) *prompt.Builder {
			return defaultPromptProvider(lang)
		},
		ClientFunc: func() *llm.Client {
			return defaultClientProvider(logger)
		},
		CommitterFunc: func() git.Committer {
			return defaultCommitterProvider(debug, logger)
		},
		GitRunnerFunc: func() git.Runner {
			return git.NewRunnerWithLogger(debug, logger)
		},
		PRCreatorFunc: func() *pr.Creator {
			return defaultPRCreatorProvider(debug, logger)
		},
	}
}

// GetCollector returns a collector instance
func (d *Dependencies) GetCollector() *gitinfo.Collector {
	return d.CollectorFunc()
}

// GetPromptBuilder returns a prompt builder for the specified language
func (d *Dependencies) GetPromptBuilder(lang string) *prompt.Builder {
	return d.PromptFunc(lang)
}

// GetClient returns a client instance
func (d *Dependencies) GetClient() *llm.Client {
	return d.ClientFunc()
}

// GetCommitter returns a committer instance
func (d *Dependencies) GetCommitter() git.Committer {
	return d.CommitterFunc()
}

// GetCommitterWithPRConfig returns a committer instance with PR configuration
func (d *Dependencies) GetCommitterWithPRConfig(prConfig PRConfig) git.Committer {
	// Use the providers function to create a PR-enabled committer
	debug := d.Logger != nil && d.Logger.Core().Enabled(zap.DebugLevel)

	// Check if we have an LLM client available for enhanced PR creation
	llmClient := d.GetClient()
	if llmClient != nil && prConfig.UseTemplate {
		// Use enhanced committer with LLM support
		llmAdapter := &LLMClientAdapter{client: llmClient}
		return newEnhancedCommitter(debug, d.Logger, true, prConfig.Remote, prConfig.BaseBranch, prConfig.Draft, prConfig.UseTemplate, llmAdapter)
	}

	// Fall back to default committer
	return newDefaultCommitter(debug, d.Logger, true, prConfig.Remote, prConfig.BaseBranch, prConfig.Draft, prConfig.UseTemplate)
}

// GetGitRunner returns a git runner instance
func (d *Dependencies) GetGitRunner() git.Runner {
	return d.GitRunnerFunc()
}

// GetPRCreator returns a PR creator instance
func (d *Dependencies) GetPRCreator() *pr.Creator {
	return d.PRCreatorFunc()
}

// GetExtendedGitRunner returns an ExtendedGitRunner for PR analysis
func (d *Dependencies) GetExtendedGitRunner() pr.ExtendedGitRunner {
	debug := d.Logger != nil && d.Logger.Core().Enabled(zap.DebugLevel)
	return &GitRunnerAdapter{
		runner: d.GetGitRunner(),
		debug:  debug,
		logger: d.Logger,
	}
}

// PRConfig holds PR-specific configuration
type PRConfig struct {
	Remote      string
	BaseBranch  string
	Draft       bool
	Provider    string
	UseTemplate bool
}

// Config holds application configuration
type Config struct {
	Debug       bool
	Language    string
	Timeout     int
	AutoConfirm bool
	DryRun      bool
	Push        bool
	StageAll    bool
	SeedText    string
	CreatePR    bool
	PRConfig    PRConfig
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Add validation logic here if needed
	return nil
}

// PROnlyConfig holds configuration for PR-only workflow
type PROnlyConfig struct {
	Debug    bool
	DryRun   bool
	Language string
	Timeout  int
	Yes      bool
	PRConfig PRConfig
}

// Validate validates the PR-only configuration
func (c *PROnlyConfig) Validate() error {
	// Add validation logic here if needed
	return nil
}

// TagConfig holds configuration for the tag workflow.
type TagConfig struct {
	Debug          bool
	DryRun         bool
	Yes            bool
	Language       string
	Timeout        int
	Remote         string
	Bump           string
	ExplicitTag    string
	InitialVersion string
	StageAll       bool
	SeedText       string
}

// Validate validates the tag workflow configuration.
func (c *TagConfig) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	if c.Remote == "" {
		return fmt.Errorf("remote must not be empty")
	}
	if _, err := tagging.NormalizeBump(c.Bump); err != nil {
		return err
	}
	if _, err := tagging.ParseVersion(c.InitialVersion); err != nil {
		return fmt.Errorf("invalid initial version: %w", err)
	}
	if c.ExplicitTag != "" {
		if _, err := tagging.ParseVersion(c.ExplicitTag); err != nil {
			return fmt.Errorf("invalid tag: %w", err)
		}
	}
	return nil
}
