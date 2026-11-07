package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBKCIContext(t *testing.T) {
	for _, tt := range []struct{ start, event, want string }{
		{"WEB_HOOK", "TAG_PUSH", "webhook(TAG_PUSH)  TAG 推送"},
		{"WEB_HOOK", "PUSH", "webhook(PUSH)  分支推送"},
		{"WEB_HOOK", "MERGE_REQUEST", "webhook(MR)  MR 触发"},
		{"WEB_HOOK", "MERGE_REQUEST_ACCEPT", "webhook(MR)  MR 触发"},
		{"WEB_HOOK", "OTHER", "webhook(OTHER)  事件触发"},
		{"WEB_HOOK", "", "webhook  事件触发"},
		{"MANUAL", "", "manual  手动触发"},
		{"TIME_TRIGGER", "", "schedule  定时触发"},
		{"PIPELINE", "", "pipeline  子流水线"},
		{"SERVICE", "", "service  服务调用"},
		{"REMOTE", "", "remote  远程触发"},
		{"", "", "manual  手动触发"}, {"OTHER", "", "manual  手动触发"},
	} {
		env := map[string]string{"BK_CI_START_TYPE": tt.start, "BK_CI_REPO_GIT_WEBHOOK_EVENT_TYPE": tt.event, "BK_CI_REPO_GIT_WEBHOOK_TAG_NAME": "v1.2.3"}
		info, err := Read("bk-ci", func(key string) string { return env[key] })
		require.NoError(t, err)
		assert.Equal(t, tt.want, info.Trigger)
		assert.Equal(t, "v1.2.3", info.Tag)
	}
	_, err := Read("unsupported", func(string) string { t.Fatal("must not read env for unknown provider"); return "" })
	require.Error(t, err)
}
