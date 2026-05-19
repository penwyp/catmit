package cmd

import (
	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	tagDebug          bool
	tagDryRun         bool
	tagYes            bool
	tagLang           string
	tagTimeout        int
	tagRemote         string
	tagBump           string
	tagExplicit       string
	tagInitialVersion string
	tagStageAll       bool
	tagSeed           string
)

var tagCmd = &cobra.Command{
	Use:   "tag [SEED_TEXT]",
	Short: "Create and push the next semantic version tag",
	Long: `Create and push a semantic version tag.

The command can generate and commit pending changes, push the branch when needed,
detect the latest remote semantic version tag, calculate the next tag, and push it.`,
	Example: `  # Inspect, confirm, then commit/push/tag/push tag as needed
  catmit tag

  # Fully automatic release
  catmit tag --yes

  # Force a minor release
  catmit tag --bump minor

  # Preview the release plan
  catmit tag --dry-run

  # Use an explicit tag
  catmit tag --tag v2.0.0`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTag,
}

func init() {
	rootCmd.AddCommand(tagCmd)

	tagCmd.Flags().BoolVar(&tagDebug, "debug", false, "enable debug output for troubleshooting")
	tagCmd.Flags().BoolVar(&tagDryRun, "dry-run", false, "preview release plan without executing")
	tagCmd.Flags().BoolVarP(&tagYes, "yes", "y", false, "skip confirmation and execute immediately")
	tagCmd.Flags().StringVarP(&tagLang, "lang", "l", "en", "output language for generated commit message (en/zh)")
	tagCmd.Flags().IntVarP(&tagTimeout, "timeout", "t", 30, "timeout in seconds")
	tagCmd.Flags().StringVarP(&tagRemote, "remote", "r", "origin", "remote to inspect and push to")
	tagCmd.Flags().StringVar(&tagBump, "bump", "auto", "version bump strategy (auto/patch/minor/major)")
	tagCmd.Flags().StringVar(&tagExplicit, "tag", "", "explicit semantic version tag to create")
	tagCmd.Flags().StringVar(&tagInitialVersion, "initial", "v0.1.0", "initial tag when no remote semantic version tag exists")
	tagCmd.Flags().BoolVar(&tagStageAll, "stage-all", true, "automatically stage all changes before committing")
	tagCmd.Flags().StringVarP(&tagSeed, "seed", "s", "", "seed text for commit message generation")
}

func runTag(cmd *cobra.Command, args []string) error {
	appLogger, err := logger.New(tagDebug)
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to initialize logger", err)
	}
	defer func() { _ = appLogger.Sync() }()

	seedText := tagSeed
	if seedText == "" && len(args) > 0 {
		seedText = args[0]
	}

	config := &app.TagConfig{
		Debug:          tagDebug,
		DryRun:         tagDryRun,
		Yes:            tagYes,
		Language:       tagLang,
		Timeout:        tagTimeout,
		Remote:         tagRemote,
		Bump:           tagBump,
		ExplicitTag:    tagExplicit,
		InitialVersion: tagInitialVersion,
		StageAll:       tagStageAll,
		SeedText:       seedText,
	}
	if err := config.Validate(); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "invalid tag configuration", err)
	}

	var deps *app.Dependencies
	if testDeps != nil {
		deps = testDeps
	} else {
		deps = app.NewDependencies(appLogger, tagDebug)
	}

	tagWorkflow := workflow.NewTagWorkflow(deps, config, cmd.OutOrStdout(), cmd.InOrStdin())
	if err := tagWorkflow.Run(cmd.Context()); err != nil {
		if errors.Is(err, errors.ErrNoGitRepo) {
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			errors.HandleFatal(err)
		}

		errType := errors.GetType(err)
		switch errType {
		case errors.ErrTypeGit, errors.ErrTypeNetwork, errors.ErrTypeAuth,
			errors.ErrTypeTimeout, errors.ErrTypeLLM, errors.ErrTypeProvider,
			errors.ErrTypePR, errors.ErrTypeExternal, errors.ErrTypeValidation:
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			errors.HandleFatal(err)
		case errors.ErrTypeConfig:
			return err
		default:
			cmd.SilenceUsage = true
			return err
		}
	}

	return nil
}
