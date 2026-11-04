package ui

import (
	"context"

	"github.com/penwyp/catmit/internal/git"
)

// CommitStage represents the stage of commit/push operation
type CommitStage int

const (
	CommitStageInit CommitStage = iota
	CommitStageCommitting
	CommitStageCommitted
	CommitStagePushing
	CommitStagePushed
	CommitStagePushFailed
	CommitStageCreatingPR
	CommitStagePRCreated
	CommitStagePRFailed
	CommitStageDone
)


// commitInterface defines the interface for commit and push operations
type commitInterface interface {
	Commit(ctx context.Context, message string) error
	Push(ctx context.Context) error
	StageAll(ctx context.Context) error
	HasStagedChanges(ctx context.Context) bool
	CreatePullRequest(ctx context.Context) (string, error)
	NeedsPush(ctx context.Context) (bool, error)
}







// --- Command and message types ---

type commitDoneMsg struct {
	err error
}

type pushDoneMsg struct {
	err error
	// mrLinks holds the merge request links announced by the remote, which is
	// only populated for Tencent Gongfeng (git.woa.com) remotes.
	mrLinks []git.MRLink
}

// mrLinkProvider is implemented by committers that can report the merge
// request links announced by the remote during the last push.
type mrLinkProvider interface {
	LastPushMRLinks() []git.MRLink
}

// collectMRLinks returns the merge request links reported by the committer,
// if it supports them.
func collectMRLinks(committer commitInterface) []git.MRLink {
	provider, ok := committer.(mrLinkProvider)
	if !ok {
		return nil
	}
	return provider.LastPushMRLinks()
}

type finalTimeoutMsg struct{}


