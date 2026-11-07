package workflow

import (
	"context"
	"time"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/git"
	tagging "github.com/penwyp/catmit/internal/tag"
)

// tagCalculator reads committed history only; publishing and CI policy live in
// their respective workflows. It never checks out, creates or pushes a tag.
type tagCalculator struct {
	manager git.TagManager
	options TagOptions
	now     func() time.Time
}

type tagCalculation struct {
	plan          *ReleasePlan
	remoteTags    []string
	hasNewCommits bool
}

func (w *tagCalculator) calculate(ctx context.Context, head string) (*tagCalculation, error) {
	remoteCtx, cancel := w.remoteOperationContext(ctx)
	remoteTags, err := w.manager.ListRemoteTags(remoteCtx, w.options.Remote)
	cancel()
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
	requested, _ := tagging.NormalizeBump(w.options.Bump)
	detail := tagging.InferBumpWithDetail(messages)
	resolved := requested
	if requested == tagging.BumpAuto {
		resolved = detail.Bump
	}
	version, err := w.nextVersion(history, remoteTags, resolved)
	if err != nil {
		return nil, err
	}
	plan := &ReleasePlan{
		Remote: w.options.Remote, Head: head, NextTag: version.String(),
		RequestedBump: requested, ResolvedBump: resolved, BumpDetail: detail,
		ExplicitTag: w.options.ExplicitTag != "", InitialTag: !history.hasStable && !history.hasLatest,
	}
	if history.hasLatest {
		plan.LatestTag = history.latest.String()
	}
	if history.hasStable {
		plan.StableTag = history.stable.String()
	}
	return &tagCalculation{plan: plan, remoteTags: remoteTags, hasNewCommits: len(messages) > 0}, nil
}

func (w *tagCalculator) remoteOperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(w.options.Timeout)*time.Second)
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

func (w *tagCalculator) releaseHistory(ctx context.Context, tags []string, head string) (tagHistory, error) {
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

func (w *tagCalculator) nextVersion(history tagHistory, remoteTags []string, bump tagging.Bump) (tagging.Version, error) {
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
