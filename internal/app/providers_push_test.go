package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/penwyp/catmit/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushCapturesOutputAndFiltersMRLinks(t *testing.T) {
	const pushOutput = "Counting objects: 100%\n" +
		"remote: | Open in TAPD: https://tapd.woa.com/merge_requests/create |\n" +
		"remote: | Open in TGIT: https://git.woa.com/a/b/-/merge_requests/new |\n"
	cases := []struct {
		name      string
		pushURL   string
		fail      bool
		wantLinks int
	}{
		{"gongfeng", "git@git.woa.com:a/b.git", false, 2},
		{"github", "git@github.com:a/b.git", false, 0},
		{"subdomain", "git@other.git.woa.com:a/b.git", false, 0},
		{"failed push", "git@git.woa.com:a/b.git", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// Different fetch and push URLs ensure filtering uses the push URL.
			fakeGit := `#!/bin/sh
case "$*" in
  'rev-parse --abbrev-ref HEAD') printf '%s\n' 'feature/test' ;;
  'remote get-url --push origin') printf '%s\n' "$TEST_PUSH_URL" ;;
  'remote get-url origin') printf '%s\n' 'git@github.com:a/b.git' ;;
  'push origin feature/test')
    printf '%s' "$TEST_PUSH_OUTPUT"
    printf '%s\n' 'remote: hook diagnostic' >&2
    if [ "$TEST_PUSH_FAIL" = 1 ]; then exit 1; fi
    ;;
  *) exit 2 ;;
esac
`
			require.NoError(t, os.WriteFile(filepath.Join(dir, "git"), []byte(fakeGit), 0755))
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TEST_PUSH_URL", tc.pushURL)
			t.Setenv("TEST_PUSH_OUTPUT", pushOutput)
			t.Setenv("TEST_PUSH_FAIL", "0")
			if tc.fail {
				t.Setenv("TEST_PUSH_FAIL", "1")
			}

			// Capture both terminal streams to catch accidental raw-output passthrough.
			terminal, err := os.CreateTemp(dir, "terminal")
			require.NoError(t, err)
			defer terminal.Close()
			stdout, stderr := os.Stdout, os.Stderr
			os.Stdout, os.Stderr = terminal, terminal
			defer func() { os.Stdout, os.Stderr = stdout, stderr }()

			committer := &defaultCommitter{
				mrLinks: []git.MRLink{{Label: "Previous push", URL: "https://git.woa.com/old"}},
			}
			err = committer.Push(context.Background())
			if tc.fail {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "git push failed")
				assert.Contains(t, err.Error(), "remote: hook diagnostic")
			} else {
				require.NoError(t, err)
			}
			links := committer.LastPushMRLinks()
			require.Len(t, links, tc.wantLinks)
			if tc.wantLinks > 0 {
				assert.Equal(t, "Open in TAPD", links[0].Label)
				assert.Equal(t, "https://git.woa.com/a/b/-/merge_requests/new", links[1].URL)
			}
			output, err := os.ReadFile(terminal.Name())
			require.NoError(t, err)
			assert.Empty(t, string(output))
		})
	}
}
