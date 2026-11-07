package cmd

import (
	"bytes"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"strings"
	"testing"

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/git"
	"github.com/penwyp/catmit/pkg/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ciCommandRunner struct{}

func (ciCommandRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	if name != "git" {
		return "", fmt.Errorf("unexpected executable: %s", name)
	}
	switch strings.Join(args, " ") {
	case "rev-parse --git-dir":
		return ".git", nil
	case "rev-parse HEAD":
		return strings.Repeat("a", 40), nil
	case "rev-parse --abbrev-ref HEAD":
		return "HEAD", nil
	default:
		return "", fmt.Errorf("unexpected Git operation: %v", args)
	}
}

func TestTagCIHelpExplainsPriorityAndOutput(t *testing.T) {
	command := newTagCommand()
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)
	command.SetArgs([]string{"ci", "--help"})
	require.NoError(t, command.Execute())
	for _, text := range []string{"--version-order", "webhook-tag,env,history,timestamp", "从左到右", "命中后立即停止", "失败或为空", "BK_CI_REPO_GIT_WEBHOOK_TAG_NAME", "--version-env", "--checkout", "stdout", "stderr", "setEnv", "-- sh build.sh"} {
		assert.Contains(t, out.String(), text)
	}
}

func TestTagCICommandValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--provider", "other"}, {"--version-order", ""}, {"--version-order", "env,env"},
		{"--version-order", "auto"}, {"--version-env", "bad-name"},
		{"--timezone", "bad/zone"}, {"--timeout", "0"}, {"--remote=-bad"},
		{"--release", "--pre", "rc"}, {"--initial", "bad"}, {"--bump", "bad"},
		{"sh", "build.sh"}, {"--print"}, {"--fallback", "timestamp"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			command := newTagCommand()
			var stdout, stderr bytes.Buffer
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			command.SetArgs(append([]string{"ci"}, args...))
			require.Error(t, command.Execute())
			assert.Empty(t, stdout.String())
		})
	}
}

func TestTagCICommandPrepareAndBuild(t *testing.T) {
	previous := testDeps
	t.Cleanup(func() { testDeps = previous })
	testDeps = &app.Dependencies{
		GitRunnerFunc: func() git.Runner { return ciCommandRunner{} },
		ClientFunc:    func() *llm.Client { panic("CI must not initialize an LLM") },
	}
	t.Setenv("BK_CI_REPO_GIT_WEBHOOK_TAG_NAME", "v2.0.0")
	t.Setenv("ZHIYAN_PKG_VERSION", "manual")
	for _, tt := range []struct {
		name string
		args []string
		code int
	}{
		{name: "prepare"},
		{name: "build", args: []string{"--", "sh", "-c", `printf '%s:%s\n' "$CATMIT_VERSION" "$ZHIYAN_PKG_VERSION"; echo build-stderr >&2`}},
		{name: "failure", args: []string{"--", "sh", "-c", "echo failed; exit 7"}, code: 7},
	} {
		t.Run(tt.name, func(t *testing.T) {
			command := newTagCommand()
			var stdout, stderr bytes.Buffer
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			args := []string{"ci", "--provider", "bk-ci", "--version-order", "webhook-tag,env,history,timestamp", "--version-env", "ZHIYAN_PKG_VERSION"}
			command.SetArgs(append(args, tt.args...))
			err := command.Execute()
			if tt.code != 0 {
				var exitErr *ExitError
				require.ErrorAs(t, err, &exitErr)
				assert.Equal(t, tt.code, exitErr.Code)
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), "failed")
			} else {
				require.NoError(t, err)
				assert.Equal(t, "v2.0.0\n", stdout.String())
				assert.Contains(t, stderr.String(), "版本摘要")
				if tt.name == "build" {
					assert.Contains(t, stderr.String(), "v2.0.0:v2.0.0")
					assert.Contains(t, stderr.String(), "build-stderr")
				} else {
					assert.NotContains(t, stderr.String(), "[build]")
				}
			}
		})
	}
}

func TestTagCIHelpFitsTerminalWidth(t *testing.T) {
	for _, width := range []int{40, 60, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			t.Setenv("COLUMNS", strconv.Itoa(width))
			command := newTagCommand()
			var stdout, stderr bytes.Buffer
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			command.SetArgs([]string{"ci", "-h"})
			require.NoError(t, command.Execute())
			assert.Empty(t, stderr.String())
			for _, line := range strings.Split(stdout.String(), "\n") {
				assert.LessOrEqual(t, ansi.StringWidth(line), width, "line: %s", line)
			}
			for _, mixed := range []string{"Usage:", "Flags:", "Examples:", "(default", "help for ci"} {
				assert.NotContains(t, stdout.String(), mixed)
			}
			for _, section := range []string{"用法", "版本来源", "代码与构建", "输出约定", "版本与代码选项", "运行选项", "历史计算选项", "示例"} {
				assert.Contains(t, stdout.String(), section)
			}
		})
	}
}
