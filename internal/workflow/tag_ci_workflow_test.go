package workflow

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ciTestOptions() TagCIOptions {
	return TagCIOptions{
		Provider: "bk-ci", VersionOrder: DefaultVersionOrder, VersionEnv: "ZHIYAN_PKG_VERSION",
		Version: TagOptions{Timeout: 10, Remote: "origin", Bump: "auto", InitialVersion: "v0.1.0", Timezone: "Asia/Shanghai", NoTimestamp: true},
	}
}

type ciTestRunner struct {
	calls       []string
	failHistory bool
}

func (r *ciTestRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	call := strings.Join(args, " ")
	r.calls = append(r.calls, call)
	if name != "git" {
		return "", fmt.Errorf("unexpected executable %s", name)
	}
	switch call {
	case "rev-parse --git-dir":
		return ".git", nil
	case "rev-parse HEAD":
		return "0123456789abcdef0123456789abcdef01234567", nil
	case "rev-parse --abbrev-ref HEAD":
		return "HEAD", nil
	case "rev-parse --is-shallow-repository":
		return "false", nil
	case "ls-remote --tags origin":
		if r.failHistory {
			return "", fmt.Errorf("remote unavailable")
		}
		return "", nil
	case "log --format=%B%x00 0123456789abcdef0123456789abcdef01234567":
		return "fix: change", nil
	default:
		return "", fmt.Errorf("unexpected Git operation %s", call)
	}
}

func TestTagCIVersionPriority(t *testing.T) {
	for _, tt := range []struct {
		name, tag, env, order, want, source string
		failHistory, history                bool
	}{
		{name: "webhook beats supplied env", tag: "v1.2.3", env: "manual", want: "v1.2.3", source: "webhook-tag"},
		{name: "supplied env skips failed remote", env: "manual-build", want: "manual-build", source: "env", failHistory: true},
		{name: "history when inputs absent", want: "v0.1.0-dev.1", source: "history", history: true},
		{name: "history failure falls through", want: "20261105153000", source: "timestamp", failHistory: true, history: true},
		{name: "explicit reordered inputs", tag: "v1.2.3", env: "manual", order: "env,webhook-tag,history", want: "manual", source: "env"},
		{name: "timestamp first skips history", order: "timestamp,history", want: "20261105153000", source: "timestamp"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options := ciTestOptions()
			if tt.order != "" {
				options.VersionOrder = tt.order
			}
			runner := &ciTestRunner{failHistory: tt.failHistory}
			var stdout, stderr bytes.Buffer
			wf := NewTagCIWorkflow(runner, options, &stdout, &stderr, nil)
			wf.getenv = func(key string) string {
				switch key {
				case "BK_CI_REPO_GIT_WEBHOOK_TAG_NAME":
					return tt.tag
				case "ZHIYAN_PKG_VERSION":
					return tt.env
				}
				return ""
			}
			wf.now = func() time.Time { return time.Date(2026, 11, 5, 7, 30, 0, 0, time.UTC) }
			require.NoError(t, wf.Run(context.Background(), nil))
			assert.Equal(t, tt.want+"\n", stdout.String())
			assert.Contains(t, stderr.String(), "来源  "+tt.source)
			assert.Contains(t, stderr.String(), "触发  manual  手动触发")
			assert.Equal(t, 1, strings.Count(stderr.String(), "版本摘要"))
			assert.Contains(t, stderr.String(), "提交  0123456789abcdef0123456789abcdef01234567")
			assert.Contains(t, stderr.String(), "env=ZHIYAN_PKG_VERSION")
			assert.Equal(t, tt.history, strings.Contains(strings.Join(runner.calls, "\n"), "ls-remote"))
			assert.NotContains(t, strings.Join(runner.calls, "\n"), "checkout")
			if tt.failHistory && tt.history {
				assert.Contains(t, stderr.String(), "remote unavailable")
			}
		})
	}
}

func TestTagCIFailuresDoNotEmitVersion(t *testing.T) {
	for _, tt := range []struct {
		name, order, env string
		canceled         bool
	}{
		{name: "no implicit timestamp", order: "history"},
		{name: "sources exhausted", order: "webhook-tag,env"},
		{name: "multiline env is fatal", order: "env,timestamp", env: "v1\nv2"},
		{name: "whitespace env is fatal", order: "env,timestamp", env: " "},
		{name: "cancellation is not fallback", order: "history,timestamp", canceled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options := ciTestOptions()
			options.VersionOrder = tt.order
			var stdout, stderr bytes.Buffer
			wf := NewTagCIWorkflow(&ciTestRunner{failHistory: true}, options, &stdout, &stderr, nil)
			wf.getenv = func(key string) string {
				if key == options.VersionEnv {
					return tt.env
				}
				return ""
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.canceled {
				cancel()
			}
			require.Error(t, wf.Run(ctx, nil))
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "版本摘要")
		})
	}
}

func TestTagCIOptionsRejectInvalidPolicy(t *testing.T) {
	for _, order := range []string{"", "env,", "auto", "env,env", "env, history", "timestamp,timestamp"} {
		options := ciTestOptions()
		options.VersionOrder = order
		require.Error(t, options.Validate(), order)
	}
	for _, env := range []string{"", "1VERSION", "BAD-NAME", "X=Y", "X\nY"} {
		options := ciTestOptions()
		options.VersionEnv = env
		require.Error(t, options.Validate(), env)
	}
	options := ciTestOptions()
	options.Provider = "unknown"
	require.Error(t, options.Validate())
	options = ciTestOptions()
	options.Version.Timezone = "Unknown/Zone"
	require.Error(t, options.Validate())
}

type ciFailedGitRunner struct{ *ciTestRunner }

func (r ciFailedGitRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "ls-remote" {
		return "fatal: authentication failed for the configured remote\n", fmt.Errorf("exit status 128")
	}
	return r.ciTestRunner.Run(ctx, name, args...)
}

func TestTagCIHistoryFallbackRetainsGitDiagnostics(t *testing.T) {
	options := ciTestOptions()
	options.VersionOrder = "history,timestamp"
	var stdout, stderr bytes.Buffer
	wf := NewTagCIWorkflow(ciFailedGitRunner{&ciTestRunner{}}, options, &stdout, &stderr, nil)
	wf.getenv = func(string) string { return "" }
	require.NoError(t, wf.Run(context.Background(), nil))
	assert.Regexp(t, `^[0-9]{14}\n$`, stdout.String())
	assert.Contains(t, stderr.String(), "fatal: authentication failed for the configured remote")
	assert.Contains(t, stderr.String(), "来源  timestamp")
}
