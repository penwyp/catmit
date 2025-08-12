package ui

import (
	"context"
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
}

type finalTimeoutMsg struct{}


