package workflow

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/git"
	tagging "github.com/penwyp/catmit/internal/tag"
	"github.com/penwyp/catmit/pkg/output"
)

type ReleasePlan struct {
	Remote             string
	Branch             string
	Head               string
	BranchRemoteExists bool
	BranchAhead        int
	BranchBehind       int
	PushBranch         bool
	LatestTag          string
	StableTag          string
	NextTag            string
	RequestedBump      tagging.Bump
	ResolvedBump       tagging.Bump
	BumpDetail         tagging.BumpDetail
	ExplicitTag        bool
	InitialTag         bool
}

type TagWorkflow struct {
	runner  git.Runner
	options TagOptions
	now     func() time.Time
	output  io.Writer
	input   io.Reader
	manager git.TagManager
}

func NewTagWorkflow(runner git.Runner, options TagOptions, outputWriter io.Writer, inputReader io.Reader) *TagWorkflow {
	if inputReader == nil {
		inputReader = strings.NewReader("")
	}
	return &TagWorkflow{
		runner:  runner,
		options: options,
		now:     time.Now,
		output:  outputWriter,
		input:   inputReader,
		manager: git.NewTagManager(runner),
	}
}

func (w *TagWorkflow) Run(ctx context.Context) error {
	if !w.options.Print {
		fmt.Fprintln(w.output, output.RenderStatusBar("Inspecting release state...", false))
	}

	plan, err := w.Plan(ctx)
	if err != nil {
		return err
	}
	if w.options.Print {
		_, err := fmt.Fprintln(w.output, plan.NextTag)
		return err
	}

	w.printPlan(plan)
	if w.options.DryRun {
		fmt.Fprintln(w.output)
		fmt.Fprintln(w.output, "Dry run: no commits, tags or pushes were made.")
		return nil
	}

	if !w.options.Yes {
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
	if err := w.options.Validate(); err != nil {
		return nil, errors.Wrap(errors.ErrTypeConfig, "invalid tag options", err)
	}
	if err := w.checkGitRepository(ctx); err != nil {
		return nil, err
	}
	if w.options.Print {
		shallow, err := w.runner.Run(ctx, "git", "rev-parse", "--is-shallow-repository")
		if err != nil {
			return nil, errors.Wrap(errors.ErrTypeGit, "failed to inspect history depth", err)
		}
		if strings.TrimSpace(shallow) == "true" {
			return nil, errors.New(errors.ErrTypeValidation,
				"--print requires full commit history; disable shallow checkout or run git fetch --unshallow before calculating a version")
		}
	}
	remote := w.options.Remote
	var branch string
	var branchStatus git.BranchStatus
	// Calculation only uses committed HEAD/history. CI may check out detached
	// HEAD and leave generated files; neither affects the version calculation.
	if !w.options.Print {
		worktreeStatus, err := w.manager.WorktreeStatus(ctx)
		if err != nil {
			return nil, err
		}
		if err := w.validateWorktreeStatus(worktreeStatus); err != nil {
			return nil, err
		}
		branch, err = w.manager.CurrentBranch(ctx)
		if err != nil {
			return nil, err
		}
		branchStatus, err = w.branchStatus(ctx, remote, branch)
		if err != nil {
			return nil, err
		}
		if branchStatus.Behind > 0 {
			return nil, errors.Newf(errors.ErrTypeValidation,
				"current branch is behind %s/%s by %d commit(s); pull or rebase before tagging",
				remote, branch, branchStatus.Behind)
		}
	}
	head, err := w.manager.HeadSHA(ctx, false)
	if err != nil {
		return nil, err
	}
	remoteTags, err := w.listRemoteTags(ctx, remote)
	if err != nil {
		return nil, err
	}
	history, err := w.releaseHistory(ctx, remoteTags, head)
	if err != nil {
		return nil, err
	}
	messages, err := w.manager.CommitMessagesSince(ctx, history.stableSHA, head)
	if err != nil {
		return nil, err
	}
	if !w.options.Print && history.hasStable && len(messages) == 0 {
		return nil, errors.Newf(errors.ErrTypeValidation,
			"no new commit since latest stable tag %s; commit changes before tagging", history.stable.String())
	}
	requestedBump, _ := tagging.NormalizeBump(w.options.Bump)
	resolvedBump := requestedBump
	bumpDetail := tagging.InferBumpWithDetail(messages)
	if requestedBump == tagging.BumpAuto {
		resolvedBump = bumpDetail.Bump
		if history.hasStable && w.options.ExplicitTag == "" && !bumpDetail.AllConventional() {
			if !w.options.Print {
				return nil, errors.New(errors.ErrTypeValidation,
					"cannot infer version from non-Conventional Commits; specify --bump patch, minor or major")
			}

		}
	}
	nextVersion, err := w.nextVersion(history, remoteTags, resolvedBump)
	if err != nil {
		return nil, err
	}
	nextTag := nextVersion.String()
	if !w.options.Print {
		localTagExists, err := w.manager.LocalTagExists(ctx, nextTag)
		if err != nil {
			return nil, err
		}
		if localTagExists {
			return nil, errors.Newf(errors.ErrTypeValidation, "local tag %s already exists", nextTag)
		}
		if conflict := conflictingTag(remoteTags, nextTag); conflict != "" {
			return nil, errors.Newf(errors.ErrTypeValidation, "remote tag %s conflicts with planned tag %s on %s", conflict, nextTag, remote)
		}
	}
	plan := &ReleasePlan{
		Remote: remote, Branch: branch, Head: head,
		BranchRemoteExists: branchStatus.RemoteExists,
		BranchAhead:        branchStatus.Ahead, BranchBehind: branchStatus.Behind,
		PushBranch: !w.options.Print && (!branchStatus.RemoteExists || branchStatus.Ahead > 0),
		NextTag:    nextTag, RequestedBump: requestedBump, ResolvedBump: resolvedBump,
		BumpDetail: bumpDetail, ExplicitTag: w.options.ExplicitTag != "",
		InitialTag: !history.hasStable && !history.hasLatest,
	}
	if history.hasLatest {
		plan.LatestTag = history.latest.String()
	}
	if history.hasStable {
		plan.StableTag = history.stable.String()
	}
	return plan, nil
}

// releaseHistory selects reachable tags, so unrelated branches cannot set this
// branch's release target. A fetched commit SHA also works in clones without tags.
type tagHistory struct {
	latest    tagging.Version
	hasLatest bool
	stable    tagging.Version
	stableSHA string
	hasStable bool
}

func (w *TagWorkflow) releaseHistory(ctx context.Context, tags []string, head string) (tagHistory, error) {
	var history tagHistory
	for _, version := range tagging.SortedVersions(tags) {
		remoteCtx, cancel := w.remoteOperationContext(ctx)
		sha, err := w.manager.FetchRemoteTag(remoteCtx, w.options.Remote, version.String())
		cancel()
		if err != nil {
			return history, err
		}
		reachable, err := w.manager.IsAncestor(ctx, sha, head)
		if err != nil {
			return history, err
		}
		if !reachable {
			continue
		}
		if !history.hasLatest {
			history.latest, history.hasLatest = version, true
		}
		if version.Prerelease == "" {
			history.stable, history.hasStable, history.stableSHA = version, true, sha
			break
		}
	}
	return history, nil
}

func (w *TagWorkflow) Execute(ctx context.Context, plan *ReleasePlan) error {
	if err := w.validateReadyToCreateTag(ctx, plan); err != nil {
		return err
	}

	if plan.PushBranch {
		fmt.Fprintln(w.output, output.RenderStatusBar("Pushing branch...", false))
		if err := w.pushBranch(ctx, plan.Remote, plan.Branch); err != nil {
			return err
		}
		fmt.Fprintln(w.output, output.RenderStatusBar("Branch pushed successfully", true))
	}

	if plan.PushBranch {
		if err := w.validateReadyToCreateTag(ctx, plan); err != nil {
			return err
		}
	}

	fmt.Fprintln(w.output, output.RenderStatusBar("Creating tag...", false))
	if err := w.manager.CreateAnnotatedTag(ctx, plan.NextTag, "Release "+plan.NextTag, plan.Head); err != nil {
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

	head, err := w.manager.HeadSHA(ctx, false)
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
		if err == nil && !exists {
			var tags []string
			tags, err = w.listRemoteTags(ctx, remote)
			if err == nil {
				if conflict := conflictingTag(tags, tagName); conflict != "" {
					err = errors.Newf(errors.ErrTypeValidation,
						"remote tag %s conflicts with planned tag %s; rerun catmit tag", conflict, tagName)
				}
			}
		}
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

// Recheck semantic identity as well as exact spelling. A concurrent publisher
// may use a different prefix/timestamp or promote this core while we confirm.
func conflictingTag(tags []string, tagName string) string {
	candidate, err := tagging.ParseVersion(tagName)
	if err != nil {
		return ""
	}
	channel := strings.Split(candidate.Prerelease, ".")[0]
	for _, tag := range tags {
		version, err := tagging.ParseVersion(tag)
		if err != nil {
			continue
		}
		if version.Compare(candidate) == 0 {
			return tag
		}
		if candidate.Prerelease != "" && version.Core().Compare(candidate.Core()) == 0 {
			if version.Prerelease == "" || (strings.Split(version.Prerelease, ".")[0] == channel && version.Compare(candidate) > 0) {
				return tag
			}
		}
	}
	return ""
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
	return context.WithTimeout(ctx, time.Duration(w.options.Timeout)*time.Second)
}

func (w *TagWorkflow) validateWorktreeStatus(status git.WorktreeStatus) error {
	if status.HasUnmergedChanges {
		return errors.New(
			errors.ErrTypeValidation,
			"worktree has unresolved merge conflicts; resolve them before tagging",
		)
	}
	if status.HasChanges || status.HasStagedChanges || status.HasUnstagedChanges {
		return errors.New(errors.ErrTypeValidation,
			"worktree has uncommitted changes; commit or clean them before tagging (tag never stages files, commits or calls an LLM)")
	}
	return nil
}

func (w *TagWorkflow) checkGitRepository(ctx context.Context) error {
	if _, err := w.runner.Run(ctx, "git", "rev-parse", "--git-dir"); err != nil {
		return errors.ErrNoGitRepo
	}
	return nil
}

func (w *TagWorkflow) nextVersion(history tagHistory, remoteTags []string, bump tagging.Bump) (tagging.Version, error) {
	if w.options.ExplicitTag != "" {
		explicit, err := tagging.ParseVersion(w.options.ExplicitTag)
		if err != nil {
			return tagging.Version{}, errors.Wrap(errors.ErrTypeConfig, "invalid explicit tag", err)
		}
		if history.hasStable && explicit.Compare(history.stable) <= 0 {
			return tagging.Version{}, errors.Newf(errors.ErrTypeValidation,
				"explicit tag %s must be greater than latest stable tag %s", explicit.String(), history.stable.String())
		}
		return explicit, nil
	}
	base, err := tagging.ParseVersion(w.options.InitialVersion)
	if history.hasStable {
		base, err = history.stable.Next(bump)
	}
	if err != nil {
		return tagging.Version{}, errors.Wrap(errors.ErrTypeConfig, "cannot calculate release target", err)
	}
	// Continue an existing prerelease target without repeatedly bumping its core.
	// An explicit --bump can intentionally select a different release line.
	requested, _ := tagging.NormalizeBump(w.options.Bump)
	if history.hasLatest && history.latest.Prerelease != "" && (requested == tagging.BumpAuto || !history.hasStable) {
		previous := history.latest.Core()
		if previous.Compare(base) >= 0 || !history.hasStable {
			base = previous
		}
	}
	if w.options.Release {
		return base.Core(), nil
	}
	channel := w.options.Pre
	if channel == "" {
		channel = "dev"
	}
	location, err := time.LoadLocation(w.options.Timezone)
	if err != nil {
		return tagging.Version{}, errors.Wrap(errors.ErrTypeConfig, "invalid timezone", err)
	}
	next, err := tagging.NextPrerelease(base, channel, w.options.NoTimestamp, remoteTags, w.now().In(location))
	if err != nil {
		return tagging.Version{}, errors.Wrap(errors.ErrTypeValidation, "cannot calculate prerelease", err)
	}
	return next, nil
}

func (w *TagWorkflow) printPlan(plan *ReleasePlan) {
	fmt.Fprintln(w.output)
	fmt.Fprintln(w.output, "Release plan")
	fmt.Fprintf(w.output, "  Branch: %s\n", plan.Branch)
	fmt.Fprintf(w.output, "  HEAD: %s\n", plan.Head)
	fmt.Fprintf(w.output, "  Remote: %s\n", plan.Remote)
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
	if plan.StableTag != "" {
		fmt.Fprintf(w.output, "  Stable baseline: %s\n", plan.StableTag)
	}
	next, _ := tagging.ParseVersion(plan.NextTag)
	if next.Prerelease == "" {
		fmt.Fprintln(w.output, "  Mode: stable release")
	} else {
		fmt.Fprintln(w.output, "  Mode: prerelease")
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
