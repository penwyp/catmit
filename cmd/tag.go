package cmd

import (
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/internal/logger"
	"github.com/penwyp/catmit/internal/workflow"
	"github.com/spf13/cobra"
)

var tagCmd = newTagCommand()

func init() {
	rootCmd.AddCommand(tagCmd)
}

func newTagCommand() *cobra.Command {
	var options workflow.TagOptions
	var debug bool
	cmd := &cobra.Command{
		Use:          "tag",
		Short:        "Calculate or publish a semantic version tag without an LLM",
		SilenceUsage: true,
		Long: `Create and push an annotated semantic version tag for committed code.

All behavior is controlled by command-line flags. No project configuration,
LLM configuration, API key, staging or automatic commit is used. The worktree
must be clean when publishing. The current branch is pushed first only when needed.

--print calculates the next version and writes ONLY that version plus a newline
to stdout. No confirmation, commit, tag creation or push occurs. Diagnostics
(including --debug) go to stderr. Detached HEAD and dirty worktrees are allowed;
only committed history is considered. Remote history is still fetched, so Git
read access and full history are required. --print conflicts with --dry-run.
Printing does not reserve a version or advance the counter: repeated calls can
produce the same version (especially with --notimestamp or within one second).

Default: a dev prerelease with a counter and an Asia/Shanghai (UTC+8) timestamp,
for example v1.2.4-dev.1+20261105T153000UTC8. --notimestamp removes only the
build timestamp; the prerelease counter still increments. --release creates a
plain stable version (v1.2.4), with no prerelease suffix or timestamp.

Increment rules:
  1. Use the latest remote stable tag reachable from HEAD as the baseline;
     preserve its v/no-v prefix. With no history, use --initial (v0.1.0).
  2. --bump auto examines ALL commits since that stable baseline:
     breaking changes (!) -> major; feat -> minor; otherwise -> patch.
     Non-Conventional Commits require an explicit --bump when publishing.
     With --print, recognized changes still determine the bump; if none imply
     minor/major, use patch. A warning goes to stderr. No LLM is involved.
     The initial release uses --initial without inferring a bump.
  3. Continue the existing prerelease target without bumping its core each time.
     New features/breaking changes may raise it; an explicit --bump overrides it.
  4. Increment the counter separately for each core version and channel:
     v1.2.4-dev.1 -> v1.2.4-dev.2 -> v1.2.4-rc.1 -> v1.2.4.
     --release can promote the same tested commit, without another commit.
     After v1.2.4, new fixes start v1.2.5-dev.1. No changes since the stable
     release means no new tag. Repeated prereleases of an unreleased commit
     are allowed and receive new counters.
  5. Build timestamps do not affect SemVer precedence. Switching --notimestamp
     on/off preserves counter ordering. Existing tags are never overwritten.
     --tag uses an exact name, bypassing automatic naming and timestamps.

Timezones:
  --timezone defaults to Asia/Shanghai. UTC gives 20261105T073000Z;
  Asia/Tokyo gives 20261105T163000UTC9 for the same instant. IANA daylight-saving
  offsets are honored; fractional offsets use UTC5-30, UTC-3-30, etc.
  Explicit --timezone and --notimestamp cannot be combined.
  --release has no timestamp and cannot be combined with explicit --timezone.

--pre [channel] defaults to dev; --pre=rc is also supported. --pre and --release
are mutually exclusive. --tag cannot be combined with naming flags.`,
		Example: `  # Default prerelease, with timestamp in UTC+8
  catmit tag

  # Preview the next tag (fetches Git refs, but does not commit, tag or push)
  catmit tag --dry-run

  # Capture the next version for a pipeline without publishing it
  VERSION=$(catmit tag --print)

  # Counter-only prerelease
  catmit tag --notimestamp

  # Release candidate with UTC timestamp
  catmit tag --pre rc --timezone UTC

  # Stable release, or explicitly choose a minor release
  catmit tag --release
  catmit tag --release --bump minor

  # Exact tag, preserving the supplied spelling
  catmit tag --tag 1.2.4-rc.1

  # Skip confirmation
  catmit tag --yes`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && cmd.Flags().Changed("pre") && options.Pre == "dev" {
				options.Pre = args[0]
				return nil
			}
			return cobra.NoArgs(cmd, args)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTag(cmd, options, debug)
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&debug, "debug", false, "enable Git debug output")
	flags.BoolVar(&options.DryRun, "dry-run", false, "preview without creating a tag or pushing")
	flags.BoolVar(&options.Print, "print", false, "print only the next version; never create or push a tag")
	flags.BoolVarP(&options.Yes, "yes", "y", false, "skip confirmation")
	flags.IntVarP(&options.Timeout, "timeout", "t", 30, "timeout per remote Git operation in seconds")
	flags.StringVarP(&options.Remote, "remote", "r", "origin", "remote to inspect and push to")
	flags.StringVar(&options.Bump, "bump", "auto", "core version bump from the stable baseline (auto/patch/minor/major)")
	flags.StringVar(&options.ExplicitTag, "tag", "", "exact semantic version tag; disables automatic naming")
	flags.StringVar(&options.InitialVersion, "initial", "v0.1.0", "initial core version when no reachable remote version exists")
	flags.StringVar(&options.Pre, "pre", "", "prerelease channel (default dev); accepts --pre rc or --pre=rc")
	flags.Lookup("pre").NoOptDefVal = "dev"
	flags.BoolVar(&options.Release, "release", false, "create a stable release without a suffix or timestamp")
	flags.BoolVar(&options.NoTimestamp, "notimestamp", false, "omit prerelease timestamp; keep the incrementing counter")
	flags.StringVar(&options.Timezone, "timezone", "Asia/Shanghai", "timestamp timezone: UTC or an IANA name")
	cmd.MarkFlagsMutuallyExclusive("pre", "release", "tag")
	cmd.MarkFlagsMutuallyExclusive("timezone", "notimestamp")
	cmd.MarkFlagsMutuallyExclusive("print", "dry-run")
	for _, flag := range []string{"bump", "initial", "notimestamp", "timezone"} {
		cmd.MarkFlagsMutuallyExclusive("tag", flag)
	}
	cmd.MarkFlagsMutuallyExclusive("release", "timezone")
	return cmd
}

func runTag(cmd *cobra.Command, options workflow.TagOptions, debug bool) error {
	if err := options.Validate(); err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "invalid tag options", err)
	}
	newLogger := logger.New
	if options.Print {
		newLogger = logger.NewStderr
	}
	appLogger, err := newLogger(debug)
	if err != nil {
		return errors.Wrap(errors.ErrTypeConfig, "failed to initialize logger", err)
	}
	defer func() { _ = appLogger.Sync() }()

	// Tagging depends only on Git; it does not initialize app/LLM dependencies.
	var runner git.Runner = git.NewRunnerWithLogger(debug, appLogger)
	if testDeps != nil {
		runner = testDeps.GetGitRunner()
	}
	wf := workflow.NewTagWorkflow(runner, options, cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin())
	if err := wf.Run(cmd.Context()); err != nil {
		cmd.SilenceUsage = true
		return err
	}
	return nil
}
