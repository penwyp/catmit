package cmd

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/pkg/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagCommandHelp(t *testing.T) {
	cmd := newTagCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
	for _, text := range []string{"--print", "--notimestamp", "--timezone", "--release", "--pre", "--help", "Increment rules:", "Asia/Shanghai", "UTC8", "No LLM", "same tested commit", "counter", "No project configuration"} {
		assert.Contains(t, out.String(), text)
	}
	for _, removed := range []string{"seed", "lang", "stage-all", "timestamp", "config"} {
		assert.Nil(t, cmd.Flags().Lookup(removed))
	}
}

func TestTagCommandInvalidFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--release", "--pre"}, {"--pre", "bad.channel"},
		{"--timezone", "UTC", "--notimestamp"}, {"--timezone", "Unknown/Zone"},
		{"--timezone", "Local"}, {"--timezone="}, {"--release", "--timezone", "UTC"},
		{"--tag", "v1.0.0", "--pre"}, {"--tag", "v1.0.0", "--bump", "auto"},
		{"--tag", "v1.0.0", "--initial", "v0.1.0"}, {"--tag", "v1.0.0", "--notimestamp"},
		{"--tag", "v1.0.0", "--timezone", "UTC"}, {"--initial", "v1.0.0-rc.1"},
		{"--seed", "text"}, {"--stage-all=false"}, {"--lang", "en"}, {"--timestamp"},
		{"some old seed text"}, {"--timeout", "0"}, {"--remote=-bad"}, {"--bump", "invalid"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := newTagCommand()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			require.Error(t, cmd.Execute())
		})
	}
}

// This runner allows only read-only initial-release planning. Any accidental
// attempt to stage, commit, push, or invoke a different executable fails.
type tagCommandRunner struct{ dirty bool }

func (r tagCommandRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	if name != "git" {
		return "", fmt.Errorf("unexpected executable %s", name)
	}
	switch strings.Join(args, " ") {
	case "rev-parse --git-dir":
		return ".git", nil
	case "status --porcelain":
		if r.dirty {
			return "?? new-file", nil
		}
		return "", nil
	case "rev-parse --abbrev-ref HEAD":
		return "main", nil
	case "rev-parse HEAD":
		return "abc123", nil
	case "ls-remote --heads origin refs/heads/main", "ls-remote --tags origin":
		return "", nil
	case "log --format=%B%x00 abc123":
		return "initial commit", nil
	}
	if len(args) > 0 && args[0] == "rev-parse" && strings.Contains(strings.Join(args, " "), "refs/tags/") {
		return "", fmt.Errorf("tag absent")
	}
	return "", fmt.Errorf("unexpected git arguments: %v", args)
}

func TestTagCommandUsesOnlyCLIWithoutLLM(t *testing.T) {
	previous := testDeps
	t.Cleanup(func() { testDeps = previous })
	t.Setenv("CATMIT_LLM_API_KEY", "")
	t.Setenv("CATMIT_TAG_RELEASE", "true")
	t.Setenv("CATMIT_TAG_TIMEZONE", "UTC")
	t.Setenv("CATMIT_TAG_NOTIMESTAMP", "true")
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--dry-run"}, "v0.1.0-dev.1+"},
		{[]string{"--dry-run", "--pre"}, "v0.1.0-dev.1+"},
		{[]string{"--dry-run", "--pre", "rc", "--notimestamp"}, "Next tag: v0.1.0-rc.1\n"},
		{[]string{"--dry-run", "--pre=beta", "--notimestamp"}, "Next tag: v0.1.0-beta.1\n"},
		{[]string{"--dry-run", "--release"}, "Next tag: v0.1.0\n"},
		{[]string{"--dry-run", "--tag", "1.2.3-rc.1"}, "Next tag: 1.2.3-rc.1\n"},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			testDeps = &app.Dependencies{
				GitRunnerFunc: func() git.Runner { return tagCommandRunner{} },
				ClientFunc:    func() *llm.Client { panic("tag must not access an LLM") },
			}
			cmd := newTagCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())
			assert.Contains(t, out.String(), tt.want)
			if len(tt.args) == 1 {
				assert.Contains(t, out.String(), "UTC8")
			}
		})
	}
	testDeps.GitRunnerFunc = func() git.Runner { return tagCommandRunner{dirty: true} }
	cmd := newTagCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--yes"})
	require.ErrorContains(t, cmd.Execute(), "uncommitted changes")
}

// Reject every operation except those needed for version calculation.
type printVersionRunner struct {
	shallow bool
	fail    bool
}

func (r printVersionRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	if r.fail {
		return "", fmt.Errorf("remote read failed")
	}
	if name != "git" {
		return "", fmt.Errorf("unexpected executable: %s", name)
	}
	switch strings.Join(args, " ") {
	case "rev-parse --git-dir":
		return ".git", nil
	case "rev-parse --is-shallow-repository":
		if r.shallow {
			return "true", nil
		}
		return "false", nil
	case "rev-parse HEAD":
		return "abc123", nil
	case "ls-remote --tags origin":
		return "", nil
	case "log --format=%B%x00 abc123":
		return "initial commit", nil
	default:
		return "", fmt.Errorf("unexpected publishing operation: %v", args)
	}
}

func TestTagPrintOutputContract(t *testing.T) {
	previous := testDeps
	t.Cleanup(func() { testDeps = previous })
	testDeps = &app.Dependencies{GitRunnerFunc: func() git.Runner { return printVersionRunner{} }}
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--print"}, `^v0\.1\.0-dev\.1\+[0-9]{8}T[0-9]{6}UTC8\n$`},
		{[]string{"--print", "--notimestamp", "--yes"}, `^v0\.1\.0-dev\.1\n$`},
		{[]string{"--print", "--pre", "rc", "--notimestamp"}, `^v0\.1\.0-rc\.1\n$`},
		{[]string{"--print", "--release"}, `^v0\.1\.0\n$`},
		{[]string{"--print", "--timezone", "UTC", "--debug"}, `^v0\.1\.0-dev\.1\+[0-9]{8}T[0-9]{6}Z\n$`},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			cmd := newTagCommand()
			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())
			assert.Regexp(t, tt.want, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestTagPrintErrorsLeaveStdoutEmpty(t *testing.T) {
	previous := testDeps
	t.Cleanup(func() { testDeps = previous })
	for _, tt := range []struct {
		args   []string
		runner printVersionRunner
	}{
		{args: []string{"--print", "--dry-run"}},
		{args: []string{"--print", "--timezone", "Unknown/Zone"}},
		{args: []string{"--print", "--unknown"}},
		{args: []string{"--print"}, runner: printVersionRunner{shallow: true}},
		{args: []string{"--print"}, runner: printVersionRunner{fail: true}},
	} {
		testDeps = &app.Dependencies{GitRunnerFunc: func() git.Runner { return tt.runner }}
		cmd := newTagCommand()
		var stdout, stderr bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs(tt.args)
		require.Error(t, cmd.Execute())
		assert.Empty(t, stdout.String(), "%v", tt.args)
		assert.NotEmpty(t, stderr.String())
	}
}
