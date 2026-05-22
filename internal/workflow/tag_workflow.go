package workflow

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/git"
	tagging "github.com/penwyp/catmit/internal/tag"
	"github.com/penwyp/catmit/pkg/output"
)

type ReleasePlan struct {
	Remote             string
	Branch             string
	Head               string
	CommitNeeded       bool
	CommitMessage      string
	BranchRemoteExists bool
	BranchAhead        int
	BranchBehind       int
	PushBranch         bool
	LatestTag          string
	NextTag            string
	RequestedBump      tagging.Bump
	ResolvedBump       tagging.Bump
	ExplicitTag        bool
	InitialTag         bool
}

type TagWorkflow struct {
	deps    *app.Dependencies
	config  *app.TagConfig
	output  io.Writer
	input   io.Reader
	manager git.TagManager
}

func NewTagWorkflow(deps *app.Dependencies, config *app.TagConfig, outputWriter io.Writer, inputReader io.Reader) *TagWorkflow {
	if inputReader == nil {
		inputReader = strings.NewReader("")
	}
	return &TagWorkflow{
		deps:    deps,
		config:  config,
		output:  outputWriter,
		input:   inputReader,
		manager: git.NewTagManager(deps.GetGitRunner()),
	}
}

func (w *TagWorkflow) Run(ctx context.Context) error {
	fmt.Fprintln(w.output, output.RenderStatusBar("Inspecting release state...", false))

	plan, err := w.Plan(ctx)
	if err != nil {
		return err
	}

	w.printPlan(plan)
	if w.config.DryRun {
		fmt.Fprintln(w.output)
		fmt.Fprintln(w.output, "Dry run: no changes were made.")
		return nil
	}

	if !w.config.Yes {
		ok, err := w.confirm()
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(w.output, "Canceled.")
			return nil
		}
	}

	return w.Execute(ctx, plan)
}

func (w *TagWorkflow) Plan(ctx context.Context) (*ReleasePlan, error) {
	if err := w.checkGitRepository(ctx); err != nil {
		return nil, err
	}

	remote := w.config.Remote
	branch, err := w.manager.CurrentBranch(ctx)
	if err != nil {
		return nil, err
	}

	head, err := w.manager.HeadSHA(ctx, true)
	if err != nil {
		return nil, err
	}

	branchStatus, err := w.branchStatus(ctx, remote, branch)
	if err != nil {
		return nil, err
	}
	if branchStatus.Behind > 0 {
		return nil, errors.Newf(
			errors.ErrTypeValidation,
			"current branch is behind %s/%s by %d commit(s); pull or rebase before tagging",
			remote,
			branch,
			branchStatus.Behind,
		)
	}

	worktreeStatus, err := w.manager.WorktreeStatus(ctx)
	if err != nil {
		return nil, err
	}
	if err := w.validateWorktreeStatus(worktreeStatus); err != nil {
		return nil, err
	}

	var commitMessage string
	if worktreeStatus.HasChanges {
		commitMessage, err = GenerateCommitMessage(ctx, w.deps, CommitMessageOptions{
			Language: w.config.Language,
			Timeout:  w.config.Timeout,
			SeedText: w.config.SeedText,
			Debug:    w.config.Debug,
		})
		if err != nil {
			return nil, err
		}
	}

	remoteTags, err := w.listRemoteTags(ctx, remote)
	if err != nil {
		return nil, err
	}

	latestVersion, hasLatest := tagging.LatestSemVerTag(remoteTags)
	latestTag := ""
	if hasLatest {
		latestTag = latestVersion.String()
	}

	requestedBump, err := tagging.NormalizeBump(w.config.Bump)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "invalid bump", err)
	}

	messages := make([]string, 0, 8)
	if commitMessage != "" {
		messages = append(messages, commitMessage)
	}
	if hasLatest {
		existingMessages, err := w.manager.CommitMessagesSince(ctx, latestTag, "HEAD")
		if err != nil {
			return nil, err
		}
		messages = append(messages, existingMessages...)
	}

	if hasLatest && !worktreeStatus.HasChanges && len(messages) == 0 {
		return nil, errors.Newf(
			errors.ErrTypeValidation,
			"current HEAD already matches latest remote tag %s; no new commit is available to tag",
			latestTag,
		)
	}

	resolvedBump := requestedBump
	if requestedBump == tagging.BumpAuto {
		resolvedBump = tagging.InferBump(messages)
	}

	nextVersion, initialTag, explicitTag, err := w.nextVersion(latestVersion, hasLatest, resolvedBump)
	if err != nil {
		return nil, err
	}
	nextTag := nextVersion.String()

	localTagExists, err := w.manager.LocalTagExists(ctx, nextTag)
	if err != nil {
		return nil, err
	}
	if localTagExists {
		return nil, errors.Newf(errors.ErrTypeValidation, "local tag %s already exists", nextTag)
	}

	if tagExists(remoteTags, nextTag) {
		return nil, errors.Newf(errors.ErrTypeValidation, "remote tag %s already exists on %s", nextTag, remote)
	}

	pushBranch := worktreeStatus.HasChanges || !branchStatus.RemoteExists || branchStatus.Ahead > 0

	return &ReleasePlan{
		Remote:             remote,
		Branch:             branch,
		Head:               head,
		CommitNeeded:       worktreeStatus.HasChanges,
		CommitMessage:      commitMessage,
		BranchRemoteExists: branchStatus.RemoteExists,
		BranchAhead:        branchStatus.Ahead,
		BranchBehind:       branchStatus.Behind,
		PushBranch:         pushBranch,
		LatestTag:          latestTag,
		NextTag:            nextTag,
		RequestedBump:      requestedBump,
		ResolvedBump:       resolvedBump,
		ExplicitTag:        explicitTag,
		InitialTag:         initialTag,
	}, nil
}

func (w *TagWorkflow) Execute(ctx context.Context, plan *ReleasePlan) error {
	if err := w.validateReadyToCreateTag(ctx, plan); err != nil {
		return err
	}

	if plan.CommitNeeded {
		if w.config.StageAll {
			fmt.Fprintln(w.output, output.RenderStatusBar("Staging changes...", false))
			if err := w.manager.StageAll(ctx); err != nil {
				return err
			}
		}

		fmt.Fprintln(w.output, output.RenderStatusBar("Committing...", false))
		if err := w.manager.Commit(ctx, plan.CommitMessage); err != nil {
			return err
		}
		fmt.Fprintln(w.output, output.RenderStatusBar("Committed successfully", true))
	}

	if plan.PushBranch {
		fmt.Fprintln(w.output, output.RenderStatusBar("Pushing branch...", false))
		if err := w.pushBranch(ctx, plan.Remote, plan.Branch); err != nil {
			return err
		}
		fmt.Fprintln(w.output, output.RenderStatusBar("Branch pushed successfully", true))
	}

	if plan.CommitNeeded || plan.PushBranch {
		if err := w.ensureTagAvailable(ctx, plan.Remote, plan.NextTag); err != nil {
			return err
		}
	}

	fmt.Fprintln(w.output, output.RenderStatusBar("Creating tag...", false))
	if err := w.manager.CreateAnnotatedTag(ctx, plan.NextTag, "Release "+plan.NextTag, "HEAD"); err != nil {
		return err
	}
	fmt.Fprintln(w.output, output.RenderStatusBar("Tag created successfully", true))

	fmt.Fprintln(w.output, output.RenderStatusBar("Pushing tag...", false))
	if err := w.pushTag(ctx, plan.Remote, plan.NextTag); err != nil {
		return err
	}
	fmt.Fprintln(w.output, output.RenderStatusBar("Tag pushed successfully", true))
	fmt.Fprintf(w.output, "Released %s\n", plan.NextTag)

	return nil
}

func (w *TagWorkflow) validateReadyToCreateTag(ctx context.Context, plan *ReleasePlan) error {
	errCh := make(chan error, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		errCh <- w.validateExecutionState(ctx, plan)
	}()

	go func() {
		defer wg.Done()
		errCh <- w.ensureTagAvailable(ctx, plan.Remote, plan.NextTag)
	}()

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *TagWorkflow) validateExecutionState(ctx context.Context, plan *ReleasePlan) error {
	branch, err := w.manager.CurrentBranch(ctx)
	if err != nil {
		return err
	}
	if branch != plan.Branch {
		return errors.Newf(
			errors.ErrTypeValidation,
			"current branch changed from %s to %s after release plan was created",
			plan.Branch,
			branch,
		)
	}

	head, err := w.manager.HeadSHA(ctx, true)
	if err != nil {
		return err
	}
	if head != plan.Head {
		return errors.Newf(
			errors.ErrTypeValidation,
			"current HEAD changed from %s to %s after release plan was created",
			plan.Head,
			head,
		)
	}

	worktreeStatus, err := w.manager.WorktreeStatus(ctx)
	if err != nil {
		return err
	}
	if err := w.validateWorktreeStatus(worktreeStatus); err != nil {
		return err
	}
	if plan.CommitNeeded && !worktreeStatus.HasChanges {
		return errors.New(errors.ErrTypeValidation, "planned commit is no longer possible because the worktree is clean")
	}
	if !plan.CommitNeeded && worktreeStatus.HasChanges {
		return errors.New(errors.ErrTypeValidation, "worktree changed after release plan was created; rerun catmit tag")
	}
	return nil
}

func (w *TagWorkflow) ensureTagAvailable(ctx context.Context, remote, tagName string) error {
	type tagCheckResult struct {
		name   string
		exists bool
		err    error
	}

	resultCh := make(chan tagCheckResult, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		exists, err := w.remoteTagExists(ctx, remote, tagName)
		resultCh <- tagCheckResult{name: "remote", exists: exists, err: err}
	}()

	go func() {
		defer wg.Done()
		exists, err := w.manager.LocalTagExists(ctx, tagName)
		resultCh <- tagCheckResult{name: "local", exists: exists, err: err}
	}()

	wg.Wait()
	close(resultCh)

	for result := range resultCh {
		if result.err != nil {
			return result.err
		}
		if !result.exists {
			continue
		}
		if result.name == "remote" {
			return errors.Newf(errors.ErrTypeValidation, "remote tag %s already exists on %s", tagName, remote)
		}
		return errors.Newf(errors.ErrTypeValidation, "local tag %s already exists", tagName)
	}
	return nil
}

func tagExists(tags []string, tagName string) bool {
	for _, tag := range tags {
		if tag == tagName {
			return true
		}
	}
	return false
}

func (w *TagWorkflow) branchStatus(ctx context.Context, remote, branch string) (git.BranchStatus, error) {
	remoteCtx, cancel := w.remoteOperationContext(ctx)
	defer cancel()
	return w.manager.BranchStatus(remoteCtx, remote, branch)
}

func (w *TagWorkflow) listRemoteTags(ctx context.Context, remote string) ([]string, error) {
	remoteCtx, cancel := w.remoteOperationContext(ctx)
	defer cancel()
	return w.manager.ListRemoteTags(remoteCtx, remote)
}

func (w *TagWorkflow) remoteTagExists(ctx context.Context, remote, tagName string) (bool, error) {
	remoteCtx, cancel := w.remoteOperationContext(ctx)
	defer cancel()
	return w.manager.RemoteTagExists(remoteCtx, remote, tagName)
}

func (w *TagWorkflow) pushBranch(ctx context.Context, remote, branch string) error {
	remoteCtx, cancel := w.remoteOperationContext(ctx)
	defer cancel()
	return w.manager.PushBranch(remoteCtx, remote, branch)
}

func (w *TagWorkflow) pushTag(ctx context.Context, remote, tagName string) error {
	remoteCtx, cancel := w.remoteOperationContext(ctx)
	defer cancel()
	return w.manager.PushTag(remoteCtx, remote, tagName)
}

func (w *TagWorkflow) remoteOperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(w.config.Timeout)*time.Second)
}

func (w *TagWorkflow) validateWorktreeStatus(status git.WorktreeStatus) error {
	if status.HasUnmergedChanges {
		return errors.New(
			errors.ErrTypeValidation,
			"worktree has unresolved merge conflicts; resolve them before tagging",
		)
	}
	if !w.config.StageAll && status.HasChanges {
		if !status.HasStagedChanges {
			return errors.New(
				errors.ErrTypeValidation,
				"worktree has changes but none are staged; stage files first or use --stage-all",
			)
		}
		if status.HasUnstagedChanges {
			return errors.New(
				errors.ErrTypeValidation,
				"worktree has unstaged or untracked changes; stage or clean them first, or use --stage-all",
			)
		}
	}
	return nil
}

func (w *TagWorkflow) checkGitRepository(ctx context.Context) error {
	runner := w.deps.GetGitRunner()
	if _, err := runner.Run(ctx, "git", "rev-parse", "--git-dir"); err != nil {
		return errors.ErrNoGitRepo
	}
	return nil
}

func (w *TagWorkflow) nextVersion(latestVersion tagging.Version, hasLatest bool, bump tagging.Bump) (tagging.Version, bool, bool, error) {
	if w.config.ExplicitTag != "" {
		explicitVersion, err := tagging.ParseVersion(w.config.ExplicitTag)
		if err != nil {
			return tagging.Version{}, false, false, errors.Wrap(errors.ErrTypeConfig, "invalid explicit tag", err)
		}
		if hasLatest && explicitVersion.Compare(latestVersion) <= 0 {
			return tagging.Version{}, false, false, errors.Newf(
				errors.ErrTypeValidation,
				"explicit tag %s must be greater than latest remote tag %s",
				explicitVersion.String(),
				latestVersion.String(),
			)
		}
		return explicitVersion, false, true, nil
	}

	if !hasLatest {
		initialVersion, err := tagging.ParseVersion(w.config.InitialVersion)
		if err != nil {
			return tagging.Version{}, false, false, errors.Wrap(errors.ErrTypeConfig, "invalid initial version", err)
		}
		return initialVersion, true, false, nil
	}

	next, err := latestVersion.Next(bump)
	if err != nil {
		return tagging.Version{}, false, false, errors.Wrap(errors.ErrTypeConfig, "invalid bump", err)
	}
	return next, false, false, nil
}

func (w *TagWorkflow) printPlan(plan *ReleasePlan) {
	fmt.Fprintln(w.output)
	fmt.Fprintln(w.output, "Release plan")
	fmt.Fprintf(w.output, "  Branch: %s\n", plan.Branch)
	fmt.Fprintf(w.output, "  HEAD: %s\n", plan.Head)
	fmt.Fprintf(w.output, "  Remote: %s\n", plan.Remote)
	fmt.Fprintf(w.output, "  Commit: %s\n", formatBoolAction(plan.CommitNeeded, "create", "skip"))
	if plan.CommitNeeded && plan.CommitMessage != "" {
		fmt.Fprintln(w.output, "  Commit message:")
		for _, line := range strings.Split(plan.CommitMessage, "\n") {
			fmt.Fprintf(w.output, "    %s\n", line)
		}
	}
	fmt.Fprintf(w.output, "  Push branch: %s\n", formatBoolAction(plan.PushBranch, "push", "skip"))
	if plan.BranchRemoteExists {
		fmt.Fprintf(w.output, "  Branch ahead/behind: +%d/-%d\n", plan.BranchAhead, plan.BranchBehind)
	} else {
		fmt.Fprintln(w.output, "  Branch ahead/behind: remote branch does not exist")
	}
	if plan.LatestTag == "" {
		fmt.Fprintln(w.output, "  Latest remote tag: none")
	} else {
		fmt.Fprintf(w.output, "  Latest remote tag: %s\n", plan.LatestTag)
	}
	if plan.ExplicitTag {
		fmt.Fprintln(w.output, "  Bump: manual")
	} else if plan.InitialTag {
		fmt.Fprintln(w.output, "  Bump: initial")
	} else {
		fmt.Fprintf(w.output, "  Bump: %s", plan.ResolvedBump)
		if plan.RequestedBump == tagging.BumpAuto {
			fmt.Fprint(w.output, " (auto)")
		}
		fmt.Fprintln(w.output)
	}
	fmt.Fprintf(w.output, "  Next tag: %s\n", plan.NextTag)
}

func (w *TagWorkflow) confirm() (bool, error) {
	fmt.Fprint(w.output, "\nProceed? [y/N] ")
	line, err := bufio.NewReader(w.input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func formatBoolAction(enabled bool, yes, no string) string {
	if enabled {
		return yes
	}
	return no
}
