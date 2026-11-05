package workflow

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type directoryGitRunner struct{ dir string }

func (r directoryGitRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Exercise real remote-only annotated tags, ancestry, same-commit promotion,
// counter ordering and both historical prefix styles using disposable repos.
func TestTagWorkflowGitReleaseCycle(t *testing.T) {
	for _, prefix := range []string{"", "v"} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			root := t.TempDir()
			remote, source, clone := filepath.Join(root, "remote.git"), filepath.Join(root, "source"), filepath.Join(root, "clone")
			git := func(dir string, args ...string) string {
				t.Helper()
				out, err := (directoryGitRunner{dir}).Run(context.Background(), "git", args...)
				require.NoError(t, err, "%v: %s", args, out)
				return strings.TrimSpace(out)
			}
			git(root, "init", "--bare", "--initial-branch=main", remote)
			git(root, "init", "--initial-branch=main", source)
			setIdentity := func(dir string) {
				git(dir, "config", "user.name", "Tag Test")
				git(dir, "config", "user.email", "tag-test@example.invalid")
				git(dir, "config", "commit.gpgsign", "false")
				git(dir, "config", "tag.gpgsign", "false")
			}
			setIdentity(source)
			require.NoError(t, os.WriteFile(filepath.Join(source, "file.txt"), []byte("base\n"), 0600))
			git(source, "add", "file.txt")
			git(source, "commit", "-m", "feat: initial release")
			git(source, "tag", "-a", prefix+"1.2.3", "-m", "baseline")
			git(source, "remote", "add", "origin", remote)
			git(source, "push", "origin", "main", "refs/tags/"+prefix+"1.2.3")
			git(root, "clone", "--no-tags", remote, clone)
			setIdentity(clone)
			assert.Empty(t, git(clone, "tag", "--list"))
			require.NoError(t, os.WriteFile(filepath.Join(clone, "file.txt"), []byte("fixed\n"), 0600))
			git(clone, "add", "file.txt")
			git(clone, "commit", "-m", "fix: release bug")
			head := git(clone, "rev-parse", "HEAD")
			var out bytes.Buffer
			wf := NewTagWorkflow(directoryGitRunner{clone}, TagOptions{
				Remote: "origin", Timeout: 10, Bump: "auto", InitialVersion: "v0.1.0", Timezone: "Asia/Shanghai",
			}, &out, strings.NewReader("y\n"))
			wf.now = func() time.Time { return time.Date(2026, 11, 5, 7, 30, 0, 0, time.UTC) }
			// CI calculation must work without a branch, clean worktree or local
			// baseline tag. It must leave local and remote refs unchanged.
			git(clone, "checkout", "--detach", head)
			require.NoError(t, os.WriteFile(filepath.Join(clone, "generated.txt"), []byte("artifact"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(clone, "file.txt"), []byte("build modified this file"), 0600))
			refsBefore := git(clone, "show-ref")
			remoteBefore := git(clone, "ls-remote", "origin")
			statusBefore := git(clone, "status", "--porcelain")
			wf.options.Print = true
			for i := 0; i < 2; i++ {
				out.Reset()
				require.NoError(t, wf.Run(context.Background()))
				assert.Equal(t, prefix+"1.2.4-dev.1.20261105T153000UTC8\n", out.String())
			}
			assert.Equal(t, refsBefore, git(clone, "show-ref"))
			assert.Equal(t, remoteBefore, git(clone, "ls-remote", "origin"))
			assert.Equal(t, statusBefore, git(clone, "status", "--porcelain"))
			assert.Equal(t, head, git(clone, "rev-parse", "HEAD"))
			assert.Empty(t, git(clone, "tag", "--list"))
			wf.options.Print = false
			out.Reset()
			require.NoError(t, os.Remove(filepath.Join(clone, "generated.txt")))
			require.NoError(t, os.WriteFile(filepath.Join(clone, "file.txt"), []byte("fixed\n"), 0600))
			git(clone, "checkout", "main")
			publish := func(want string) {
				t.Helper()
				plan, err := wf.Plan(context.Background())
				require.NoError(t, err)
				assert.Equal(t, want, plan.NextTag)
				assert.Regexp(t, `^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`, plan.NextTag)
				require.NoError(t, wf.Execute(context.Background(), plan))
				assert.Equal(t, head, git(clone, "rev-parse", "refs/tags/"+want+"^{commit}"))
				assert.Contains(t, git(clone, "ls-remote", "--tags", "origin", "refs/tags/"+want), "refs/tags/"+want)
			}
			publish(prefix + "1.2.4-dev.1.20261105T153000UTC8")
			publish(prefix + "1.2.4-dev.2.20261105T153000UTC8")
			wf.options.NoTimestamp = true
			publish(prefix + "1.2.4-dev.3")
			wf.options.NoTimestamp = false
			publish(prefix + "1.2.4-dev.4.20261105T153000UTC8")
			wf.options.Pre = "rc"
			publish(prefix + "1.2.4-rc.1.20261105T153000UTC8")
			wf.options.Pre, wf.options.Release = "", true
			publish(prefix + "1.2.4")
			_, err := wf.Plan(context.Background())
			require.ErrorContains(t, err, "no new commit since latest stable")
			wf.options.Print, wf.options.Release, wf.options.NoTimestamp = true, false, true
			out.Reset()
			require.NoError(t, wf.Run(context.Background()))
			assert.Equal(t, prefix+"1.2.5-dev.1\n", out.String())
			assert.Empty(t, git(clone, "status", "--porcelain"))
			assert.Empty(t, git(clone, "tag", "--list", prefix+"1.2.3"), "fetching the baseline must not create a local tag")
			assert.Equal(t, head, git(clone, "rev-parse", "HEAD"), "tag must never create a commit")
		})
	}
}
