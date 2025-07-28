package cmd

import (

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	// Common flags
	prDebug   bool   // enable debug output for troubleshooting
	prDryRun  bool   // preview PR creation without actually creating
	prLang    string // language for PR description (ISO 639-1)
	prTimeout int    // API timeout in seconds
	prYes     bool   // skip confirmation

	// PR-specific flags
	prBase     string // base branch for pull request
	prDraft    bool   // create pull request as draft
	prProvider string // override detected provider
	prRemote   string // remote to use for pull request
	prTemplate bool   // use PR template if available
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Create a pull request for the current branch",
	Long: `Create a pull request for the current branch with AI-generated title and description.

This command analyzes the commits on your current branch and generates an appropriate
PR title and description. It supports multiple Git providers (GitHub, GitLab, Gitea)
and can use PR templates if available.`,
	Example: `  # Create PR interactively
  catmit pr

  # Create PR without confirmation
  catmit pr --yes

  # Create draft PR against specific branch
  catmit pr --draft --base main

  # Preview PR creation
  catmit pr --dry-run`,
	RunE: runPR,
}

func init() {
	rootCmd.AddCommand(prCmd)

	// Common flags
	prCmd.Flags().BoolVar(&prDebug, "debug", false, "enable debug output for troubleshooting")
	prCmd.Flags().BoolVar(&prDryRun, "dry-run", false, "preview PR creation without executing")
	prCmd.Flags().StringVarP(&prLang, "lang", "l", "en", "output language (en/zh)")
	prCmd.Flags().IntVarP(&prTimeout, "timeout", "t", 30, "timeout in seconds")
	prCmd.Flags().BoolVarP(&prYes, "yes", "y", false, "skip confirmation and create PR immediately")

	// PR-specific flags
	prCmd.Flags().StringVar(&prBase, "base", "", "base branch for pull request")
	prCmd.Flags().BoolVar(&prDraft, "draft", false, "create pull request as draft")
	prCmd.Flags().StringVar(&prProvider, "provider", "", "override detected git provider")
	prCmd.Flags().StringVar(&prRemote, "remote", "origin", "remote to use for pull request")
	prCmd.Flags().BoolVar(&prTemplate, "template", true, "use PR template if available")
}

func runPR(cmd *cobra.Command, args []string) error {
	// Initialize logger
	appLogger, err := logger.New(prDebug)
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to initialize logger", err)
	}
	defer func() { _ = appLogger.Sync() }()

	// Create PR-specific configuration
	config := &app.PROnlyConfig{
		Debug:    prDebug,
		DryRun:   prDryRun,
		Language: prLang,
		Timeout:  prTimeout,
		Yes:      prYes,
		PRConfig: app.PRConfig{
			Remote:      prRemote,
			BaseBranch:  prBase,
			Draft:       prDraft,
			Provider:    prProvider,
			UseTemplate: prTemplate,
		},
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "invalid configuration", err)
	}

	// Create dependencies
	var deps *app.Dependencies
	if testDeps != nil {
		deps = testDeps
	} else {
		deps = app.NewDependencies(appLogger, prDebug)
	}

	// Create and run PR workflow
	prWorkflow := workflow.NewPRWorkflow(deps, config, cmd.OutOrStdout())

	ctx := cmd.Context()
	if err := prWorkflow.Run(ctx); err != nil {
		// Handle specific error types
		if errors.Is(err, errors.ErrNoGitRepo) {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			errors.HandleFatal(err)
		}

		// Check error type to determine if help should be shown
		errType := errors.GetType(err)
		switch errType {
		case errors.ErrTypeGit, errors.ErrTypeNetwork, errors.ErrTypeAuth,
			errors.ErrTypeTimeout, errors.ErrTypeLLM, errors.ErrTypeProvider,
			errors.ErrTypePR, errors.ErrTypeExternal:
			// Runtime/execution errors - don't show help
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			errors.HandleFatal(err)
		case errors.ErrTypeConfig, errors.ErrTypeValidation:
			// Configuration/validation errors - show help
			return err
		default:
			// Unknown errors - default to not showing help
			cmd.SilenceUsage = true
			return err
		}
	}

	return nil
}