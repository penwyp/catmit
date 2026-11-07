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
		_, _ = fmt.Fprintln(w.output, output.RenderStatusBar("Inspecting release state...", false))
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
		_, _ = fmt.Fprintln(w.output)
		_, _ = fmt.Fprintln(w.output, "Dry run: no commits, tags or pushes were made.")
		return nil
	}

	if !w.options.Yes {
		ok, err := w.confirm()
		if err != nil {
			return err
		}
		if !ok {
			_, _ = fmt.Fprintln(w.output, "Canceled.")
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
	calculator := tagCalculator{manager: w.manager, options: w.options, now: w.now}
	calculation, err := calculator.calculate(ctx, head)
	if err != nil {
		return nil, err
	}
	plan := calculation.plan
	if !w.options.Print && plan.StableTag != "" && !calculation.hasNewCommits {
		return nil, errors.Newf(errors.ErrTypeValidation,
			"no new commit since latest stable tag %s; commit changes before tagging", plan.StableTag)
	}
	nextTag := plan.NextTag
	if !w.options.Print {
		localTagExists, err := w.manager.LocalTagExists(ctx, nextTag)
		if err != nil {
			return nil, err
		}
		if localTagExists {
			return nil, errors.Newf(errors.ErrTypeValidation, "local tag %s already exists", nextTag)
		}
		if conflict := conflictingTag(calculation.remoteTags, nextTag); conflict != "" {
			return nil, errors.Newf(errors.ErrTypeValidation, "remote tag %s conflicts with planned tag %s on %s", conflict, nextTag, remote)
		}
	}
	plan.Branch = branch
	plan.BranchRemoteExists = branchStatus.RemoteExists
	plan.BranchAhead = branchStatus.Ahead
	plan.BranchBehind = branchStatus.Behind
	plan.PushBranch = !w.options.Print && (!branchStatus.RemoteExists || branchStatus.Ahead > 0)
	return plan, nil
}

func (w *TagWorkflow) Execute(ctx context.Context, plan *ReleasePlan) error {
	if err := w.validateReadyToCreateTag(ctx, plan); err != nil {
		return err
	}

	if plan.PushBranch {
		_, _ = fmt.Fprintln(w.output, output.RenderStatusBar("Pushing branch...", false))
		if err := w.pushBranch(ctx, plan.Remote, plan.Branch); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(w.output, output.RenderStatusBar("Branch pushed successfully", true))
	}

	if plan.PushBranch {
		if err := w.validateReadyToCreateTag(ctx, plan); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintln(w.output, output.RenderStatusBar("Creating tag...", false))
	if err := w.manager.CreateAnnotatedTag(ctx, plan.NextTag, "Release "+plan.NextTag, plan.Head); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w.output, output.RenderStatusBar("Tag created successfully", true))

	_, _ = fmt.Fprintln(w.output, output.RenderStatusBar("Pushing tag...", false))
	if err := w.pushTag(ctx, plan.Remote, plan.NextTag); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w.output, output.RenderStatusBar("Tag pushed successfully", true))
	_, _ = fmt.Fprintf(w.output, "Released %s\n", plan.NextTag)

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

// Recheck semantic identity and channel counters as well as exact spelling. A
// concurrent publisher may use a different timestamp or promote this core.
func conflictingTag(tags []string, tagName string) string {
	candidate, err := tagging.ParseVersion(tagName)
	if err != nil {
		return ""
	}
	candidateParts := strings.Split(candidate.Prerelease, ".")
	for _, tag := range tags {
		version, err := tagging.ParseVersion(tag)
		if err != nil {
			continue
		}
		if version.Compare(candidate) == 0 {
			return tag
		}
		if candidate.Prerelease != "" && version.Core().Compare(candidate.Core()) == 0 {
			parts := strings.Split(version.Prerelease, ".")
			sameCounter := len(candidateParts) >= 2 && len(parts) >= 2 && parts[1] == candidateParts[1]
			if version.Prerelease == "" || (parts[0] == candidateParts[0] && (sameCounter || version.Compare(candidate) > 0)) {
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

func (w *TagWorkflow) printPlan(plan *ReleasePlan) {
	_, _ = fmt.Fprintln(w.output)
	_, _ = fmt.Fprintln(w.output, "Release plan")
	_, _ = fmt.Fprintf(w.output, "  Branch: %s\n", plan.Branch)
	_, _ = fmt.Fprintf(w.output, "  HEAD: %s\n", plan.Head)
	_, _ = fmt.Fprintf(w.output, "  Remote: %s\n", plan.Remote)
	_, _ = fmt.Fprintf(w.output, "  Push branch: %s\n", formatBoolAction(plan.PushBranch, "push", "skip"))
	if plan.BranchRemoteExists {
		_, _ = fmt.Fprintf(w.output, "  Branch ahead/behind: +%d/-%d\n", plan.BranchAhead, plan.BranchBehind)
	} else {
		_, _ = fmt.Fprintln(w.output, "  Branch ahead/behind: remote branch does not exist")
	}
	if plan.LatestTag == "" {
		_, _ = fmt.Fprintln(w.output, "  Latest remote tag: none")
	} else {
		_, _ = fmt.Fprintf(w.output, "  Latest remote tag: %s\n", plan.LatestTag)
	}
	if plan.StableTag != "" {
		_, _ = fmt.Fprintf(w.output, "  Stable baseline: %s\n", plan.StableTag)
	}
	next, _ := tagging.ParseVersion(plan.NextTag)
	if next.Prerelease == "" {
		_, _ = fmt.Fprintln(w.output, "  Mode: stable release")
	} else {
		_, _ = fmt.Fprintln(w.output, "  Mode: prerelease")
	}
	if plan.ExplicitTag {
		_, _ = fmt.Fprintln(w.output, "  Bump: manual")
	} else if plan.InitialTag {
		_, _ = fmt.Fprintln(w.output, "  Bump: initial")
	} else {
		_, _ = fmt.Fprintf(w.output, "  Bump: %s", plan.ResolvedBump)
		if plan.RequestedBump == tagging.BumpAuto {
			_, _ = fmt.Fprint(w.output, " (auto)")
		}
		_, _ = fmt.Fprintln(w.output)
	}
	_, _ = fmt.Fprintf(w.output, "  Next tag: %s\n", plan.NextTag)
}

func (w *TagWorkflow) confirm() (bool, error) {
	_, _ = fmt.Fprint(w.output, "\nProceed? [y/N] ")
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
