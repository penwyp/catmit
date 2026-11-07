package workflow

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ciRepository struct{ root, remote, source, clone, baseline, head string }

func ciGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := (directoryGitRunner{dir}).Run(context.Background(), "git", args...)
	require.NoError(t, err, "%v: %s", args, out)
	return strings.TrimSpace(out)
}

func ciChdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(previous)) })
}

func newCIRepository(t *testing.T, shallow bool) ciRepository {
	t.Helper()
	r := ciRepository{root: t.TempDir()}
	r.remote, r.source, r.clone = filepath.Join(r.root, "remote.git"), filepath.Join(r.root, "source"), filepath.Join(r.root, "clone")
	ciGit(t, r.root, "init", "--bare", "--initial-branch=main", r.remote)
	ciGit(t, r.root, "init", "--initial-branch=main", r.source)
	ciGit(t, r.source, "config", "user.name", "CI Test")
	ciGit(t, r.source, "config", "user.email", "ci@example.invalid")
	ciGit(t, r.source, "config", "commit.gpgsign", "false")
	ciGit(t, r.source, "config", "tag.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(r.source, "file.txt"), []byte("baseline\n"), 0600))
	ciGit(t, r.source, "add", "file.txt")
	ciGit(t, r.source, "commit", "-m", "feat: baseline")
	r.baseline = ciGit(t, r.source, "rev-parse", "HEAD")
	ciGit(t, r.source, "tag", "-a", "v1.2.3", "-m", "baseline")
	require.NoError(t, os.WriteFile(filepath.Join(r.source, "file.txt"), []byte("new feature\n"), 0600))
	ciGit(t, r.source, "add", "file.txt")
	ciGit(t, r.source, "commit", "-m", "feat: next feature")
	r.head = ciGit(t, r.source, "rev-parse", "HEAD")
	ciGit(t, r.source, "remote", "add", "origin", r.remote)
	ciGit(t, r.source, "push", "origin", "main", "refs/tags/v1.2.3")
	args := []string{"clone", "--no-tags"}
	if shallow {
		args = append(args, "--depth=1")
	}
	ciGit(t, r.root, append(args, "file://"+r.remote, r.clone)...)
	return r
}

func TestTagCICheckoutUsesRemoteCommitAndStreamsBuild(t *testing.T) {
	r := newCIRepository(t, false)
	// Local tag deliberately disagrees with the remote annotated tag.
	ciGit(t, r.clone, "tag", "v1.2.3", r.head)
	refsBefore := ciGit(t, r.clone, "show-ref")
	remoteBefore := ciGit(t, r.clone, "ls-remote", "origin")
	t.Setenv("CATMIT_VERSION", "stale-generic")
	t.Setenv("ZHIYAN_PKG_VERSION", "stale-upstream")
	options := ciTestOptions()
	options.Checkout = true
	var stdout, stderr bytes.Buffer
	wf := NewTagCIWorkflow(directoryGitRunner{r.clone}, options, &stdout, &stderr, nil)
	wf.getenv = func(key string) string {
		switch key {
		case "BK_CI_REPO_GIT_WEBHOOK_TAG_NAME":
			return "v1.2.3"
		case "BK_CI_START_TYPE":
			return "WEB_HOOK"
		case "BK_CI_REPO_GIT_WEBHOOK_EVENT_TYPE":
			return "TAG_PUSH"
		default:
			return os.Getenv(key)
		}
	}
	// Workflows run in the caller's working directory, including a repository subdirectory.
	subdir := filepath.Join(r.clone, "app")
	require.NoError(t, os.Mkdir(subdir, 0700))
	ciChdir(t, subdir)
	command := []string{"sh", "-c", `printf 'stdout:%s:%s\n' "$CATMIT_VERSION" "$ZHIYAN_PKG_VERSION"; printf 'stderr:build-log\n' >&2; test "$(cat ../file.txt)" = baseline`}
	require.NoError(t, wf.Run(context.Background(), command))
	assert.Equal(t, "v1.2.3\n", stdout.String())
	assert.Contains(t, stderr.String(), "stdout:v1.2.3:v1.2.3")
	assert.Contains(t, stderr.String(), "stderr:build-log")
	assert.Contains(t, stderr.String(), "webhook(TAG_PUSH)")
	assert.Contains(t, stderr.String(), "提交  "+r.baseline)
	assert.Equal(t, r.baseline, ciGit(t, r.clone, "rev-parse", "HEAD"))
	assert.Equal(t, refsBefore, ciGit(t, r.clone, "show-ref"), "do not replace local tags or create other refs")
	assert.Equal(t, remoteBefore, ciGit(t, r.clone, "ls-remote", "origin"))
	assert.Less(t, strings.Index(stderr.String(), "stderr:build-log"), strings.Index(stderr.String(), "版本摘要"))
}

func TestTagCIPrepareOnlyUnshallowsHistory(t *testing.T) {
	r := newCIRepository(t, true)
	remoteBefore := ciGit(t, r.clone, "ls-remote", "origin")
	ciGit(t, r.clone, "checkout", "--detach", r.head)
	require.NoError(t, os.WriteFile(filepath.Join(r.clone, "generated.txt"), []byte("artifact"), 0600))
	options := ciTestOptions()
	options.VersionOrder = "history"
	var stdout, stderr bytes.Buffer
	wf := NewTagCIWorkflow(directoryGitRunner{r.clone}, options, &stdout, &stderr, nil)
	wf.getenv = func(string) string { return "" }
	require.NoError(t, wf.Run(context.Background(), nil))
	assert.Equal(t, "v1.3.0-dev.1\n", stdout.String())
	assert.Contains(t, stderr.String(), "补全浅克隆历史")
	assert.NotContains(t, stderr.String(), "[build]")
	assert.Equal(t, "false", ciGit(t, r.clone, "rev-parse", "--is-shallow-repository"))
	assert.Empty(t, ciGit(t, r.clone, "tag", "--list"))
	assert.Equal(t, r.head, ciGit(t, r.clone, "rev-parse", "HEAD"))
	assert.Equal(t, remoteBefore, ciGit(t, r.clone, "ls-remote", "origin"))
	assert.FileExists(t, filepath.Join(r.clone, "generated.txt"))
}

func TestTagCICheckoutFailureNeverFallsBackOrBuilds(t *testing.T) {
	for _, tag := range []string{"missing", "v1.2.3", "bad\nname", "../bad"} {
		t.Run(tag, func(t *testing.T) {
			r := newCIRepository(t, false)
			require.NoError(t, os.WriteFile(filepath.Join(r.clone, "file.txt"), []byte("local changes\n"), 0600))
			options := ciTestOptions()
			options.Checkout = true
			var stdout, stderr bytes.Buffer
			wf := NewTagCIWorkflow(directoryGitRunner{r.clone}, options, &stdout, &stderr, nil)
			wf.getenv = func(key string) string {
				if key == "BK_CI_REPO_GIT_WEBHOOK_TAG_NAME" {
					return tag
				}
				return ""
			}
			ciChdir(t, r.clone)
			require.Error(t, wf.Run(context.Background(), []string{"sh", "-c", "touch built"}))
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "版本摘要")
			assert.NoFileExists(t, filepath.Join(r.clone, "built"))
			assert.Equal(t, r.head, ciGit(t, r.clone, "rev-parse", "HEAD"))
		})
	}
}

func TestTagCIBuildFailurePreservesErrorAndLogs(t *testing.T) {
	r := newCIRepository(t, false)
	options := ciTestOptions()
	var stdout, stderr bytes.Buffer
	wf := NewTagCIWorkflow(directoryGitRunner{r.clone}, options, &stdout, &stderr, nil)
	wf.getenv = func(key string) string {
		if key == options.VersionEnv {
			return "manual"
		}
		return ""
	}
	err := wf.Run(context.Background(), []string{"sh", "-c", "echo failure-stdout; echo failure-stderr >&2; exit 7"})
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.ExitCode())
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "failure-stdout")
	assert.Contains(t, stderr.String(), "failure-stderr")
	assert.NotContains(t, stderr.String(), "版本摘要")
}

func TestTagCIRejectsBuildChangingCommit(t *testing.T) {
	r := newCIRepository(t, false)
	options := ciTestOptions()
	var stdout, stderr bytes.Buffer
	wf := NewTagCIWorkflow(directoryGitRunner{r.clone}, options, &stdout, &stderr, nil)
	wf.getenv = func(key string) string {
		if key == options.VersionEnv {
			return "manual"
		}
		return ""
	}
	ciChdir(t, r.clone)
	err := wf.Run(context.Background(), []string{"git", "checkout", "--detach", r.baseline})
	require.ErrorContains(t, err, "build changed HEAD")
	assert.Empty(t, stdout.String())
	assert.NotContains(t, stderr.String(), "版本摘要")
}

type cancelBuildWriter struct {
	buffer bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelBuildWriter) String() string { return w.buffer.String() }

func (w *cancelBuildWriter) Write(p []byte) (int, error) {
	n, err := w.buffer.Write(p)
	if strings.Contains(w.buffer.String(), "child-started") {
		w.cancel()
	}
	return n, err
}

func TestTagCICancelBuildDoesNotEmitVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	options := ciTestOptions()
	var stdout bytes.Buffer
	stderr := &cancelBuildWriter{cancel: cancel}
	wf := NewTagCIWorkflow(&ciTestRunner{}, options, &stdout, stderr, nil)
	wf.getenv = func(key string) string {
		if key == options.VersionEnv {
			return "manual"
		}
		return ""
	}
	err := wf.Run(ctx, []string{"sh", "-c", "printf 'child-%s\\n' started; sleep 30"})
	require.Error(t, err)
	assert.True(t, errors.Is(ctx.Err(), context.Canceled))
	assert.Empty(t, stdout.String())
	assert.NotContains(t, stderr.String(), "版本摘要")
}
