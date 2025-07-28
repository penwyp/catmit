package cmd

import (
	"context"
	"fmt"

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/workflow"
	"github.com/penwyp/catmit/pkg/gitinfo"
	"github.com/spf13/cobra"
)

// version holds the current version of catmit
// This will be set at build time via ldflags
var version = "dev"

// GetVersionString returns a formatted version string
func GetVersionString() string {
	return fmt.Sprintf("catmit version %s", version)
}

var rootCmd = &cobra.Command{
	Use:   "catmit [SEED_TEXT]",
	Short: "AI-powered commit message generator with comprehensive change analysis",
	Long: `catmit is an AI-powered tool that generates high-quality Git commit messages 
by analyzing your staged changes, unstaged modifications, and untracked files.

Features:
- Analyzes all types of changes including untracked files
- Follows Conventional Commits specification
- Smart token budgeting for large changesets
- Interactive review and editing capabilities
- Multiple language support (English/Chinese)`,
	RunE: run,
}

var (
	flagLang     string // commit message language (ISO 639-1)
	flagTimeout  int    // API timeout in seconds
	flagYes      bool   // skip confirmation and commit immediately
	flagDryRun   bool   // print message but do not commit
	flagDebug    bool   // enable debug output for troubleshooting
	flagPush     bool   // automatically push after successful commit
	flagStageAll bool   // automatically stage all changes (tracked and untracked) if none are staged
	flagVersion  bool   // show version information
	flagSeed     string // seed text for commit message generation
)

func init() {
	// Disable automatic completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Disable automatic help command
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	rootCmd.Flags().StringVarP(&flagLang, "lang", "l", "en", "commit message language (ISO 639-1)")
	rootCmd.Flags().IntVarP(&flagTimeout, "timeout", "t", 20, "API timeout in seconds")
	rootCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation and commit immediately")
	rootCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "print message but do not commit")
	rootCmd.Flags().BoolVar(&flagDebug, "debug", false, "enable debug output for troubleshooting")
	rootCmd.Flags().BoolVarP(&flagPush, "push", "p", true, "automatically push after successful commit")
	rootCmd.Flags().BoolVar(&flagStageAll, "stage-all", true, "automatically stage all changes (tracked and untracked) if none are staged")
	rootCmd.Flags().BoolVar(&flagVersion, "version", false, "show version information")
	rootCmd.Flags().StringVarP(&flagSeed, "seed", "s", "", "seed text for commit message generation")

	// Add check-auth command
	// Note: debug flag is not available in init(), will be false by default
	checkAuthCmd := NewCheckAuthCommand(
		newAuthGitRunner(false),
		newAuthProviderDetector(),
		newAuthCLIDetector(),
	)
	rootCmd.AddCommand(checkAuthCmd)

	// Disable Cobra's auto-generated completion command
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Disable Cobra's auto-generated help command
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
}

func ExecuteContext(ctx context.Context) error { return rootCmd.ExecuteContext(ctx) }

// testDeps is used for dependency injection in tests
var testDeps *app.Dependencies

func run(cmd *cobra.Command, args []string) error {
	// Handle version flag
	if flagVersion {
		fmt.Println(GetVersionString())
		return nil
	}

	// Initialize logger
	appLogger, err := logger.New(flagDebug)
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to initialize logger", err)
	}
	defer func() { _ = appLogger.Sync() }()

	// Prioritize --seed flag over positional argument
	seedText := flagSeed
	if seedText == "" && len(args) > 0 {
		seedText = args[0]
	}

	// Create application configuration
	config := &app.Config{
		Debug:       flagDebug,
		Language:    flagLang,
		Timeout:     flagTimeout,
		AutoConfirm: flagYes,
		DryRun:      flagDryRun,
		Push:        flagPush,
		StageAll:    flagStageAll,
		SeedText:    seedText,
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "invalid configuration", err)
	}

	// Create dependencies (use test deps if available)
	var deps *app.Dependencies
	if testDeps != nil {
		deps = testDeps
	} else {
		deps = app.NewDependencies(appLogger, flagDebug)
	}

	// Create and run workflow
	wf := workflow.New(deps, config, cmd.OutOrStdout())

	ctx := cmd.Context()
	if err := wf.Run(ctx); err != nil {
		// Handle specific error types
		if errors.Is(err, errors.ErrNoGitRepo) {
			// Set both SilenceUsage and SilenceErrors to prevent Cobra's error output
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			// Use error handler for proper exit code
			errors.HandleFatal(err)
		}
		if errors.Is(err, gitinfo.ErrNoDiff) {
			// Display the error message but exit with code 0 since it's not really an error
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			handler := errors.NewHandler(false)
			_ = handler.Handle(err)
			return nil
		}

		// Check error type to determine if help should be shown
		errType := errors.GetType(err)
		switch errType {
		case errors.ErrTypeGit, errors.ErrTypeNetwork, errors.ErrTypeAuth,
			errors.ErrTypeTimeout, errors.ErrTypeLLM, errors.ErrTypeProvider,
			errors.ErrTypePR, errors.ErrTypeExternal:
			// Runtime/execution errors - don't show help, just show error message
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			errors.HandleFatal(err)
		case errors.ErrTypeConfig, errors.ErrTypeValidation:
			// Configuration/validation errors - show help as it may be useful
			return err
		default:
			// Unknown errors - default to not showing help to avoid noise
			cmd.SilenceUsage = true
			return err
		}
	}

	return nil
}
