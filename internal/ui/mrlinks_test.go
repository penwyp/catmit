package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/penwyp/catmit/internal/git"
	"github.com/stretchr/testify/assert"
)

// mockMRCommitCommitter is a commitInterface that also reports the merge
// request links announced by the remote (the git.woa.com behaviour).
type mockMRCommitCommitter struct {
	mockCommitCommitter
	links []git.MRLink
}

func (m *mockMRCommitCommitter) LastPushMRLinks() []git.MRLink {
	return m.links
}

var mrLinkFixtures = []git.MRLink{
	{Label: "Open in TAPD", URL: "https://tapd.woa.com/tapd_fe/dev/gongfeng/merge_requests/create?from=develop"},
	{Label: "Open in TGIT", URL: "https://git.woa.com/nosql/oss_backend/-/merge_requests/new?ADTAG=git-cli"},
}

func TestPushDoneMsgCarriesMRLinks(t *testing.T) {
	committer := &mockMRCommitCommitter{links: mrLinkFixtures}

	model := NewCommitWorkflowModel(
		context.Background(),
		&mockCommitCollector{},
		&mockCommitPromptBuilder{},
		&mockCommitClient{},
		committer,
		"seed",
		"en",
		5*time.Second,
		true,
		true,
		PRConfig{},
	)
	model.phase = WorkflowPhaseCommit
	model.commitStage = CommitStagePushing

	newModel, _ := model.Update(pushDoneMsg{err: nil, mrLinks: mrLinkFixtures})
	content := newModel.(*CommitWorkflowModel).renderCommitContent()

	assert.Contains(t, content, "Merge request links:")
	assert.Contains(t, content, "Open in TAPD: https://tapd.woa.com/")
	assert.Contains(t, content, "Open in TGIT: https://git.woa.com/nosql/oss_backend/-/merge_requests/new")
}

func TestRenderPushedMRLinksEmpty(t *testing.T) {
	model, _, _, _, _ := createTestCommitWorkflowModel()
	model.commitStage = CommitStagePushed

	content := model.renderCommitContent()

	assert.NotContains(t, content, "Merge request links:")
	assert.Contains(t, content, "Pushed successfully")
}

func TestCollectMRLinksForUnsupportedCommitter(t *testing.T) {
	_, _, _, _, committer := createTestCommitWorkflowModel()

	assert.Nil(t, collectMRLinks(committer))
}

func TestCollectMRLinksForGongfengCommitter(t *testing.T) {
	committer := &mockMRCommitCommitter{links: mrLinkFixtures}

	assert.Equal(t, mrLinkFixtures, collectMRLinks(committer))
}

func TestCommitWorkflowModelStartPushCollectsMRLinks(t *testing.T) {
	committer := &mockMRCommitCommitter{links: mrLinkFixtures}
	committer.On("Push", context.Background()).Return(nil)

	model := NewCommitWorkflowModel(
		context.Background(),
		&mockCommitCollector{},
		&mockCommitPromptBuilder{},
		&mockCommitClient{},
		committer,
		"seed",
		"en",
		5*time.Second,
		true,
		true,
		PRConfig{},
	)

	msg, ok := model.startPush()().(pushDoneMsg)
	assert.True(t, ok)
	assert.NoError(t, msg.err)
	assert.Equal(t, mrLinkFixtures, msg.mrLinks)

	// The rendered summary must show the links only for gongfeng pushes.
	rendered := model.renderCommitContent()
	assert.False(t, strings.Contains(rendered, "Open in TAPD"))
}
