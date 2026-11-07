package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/penwyp/catmit/internal/ci"
	"github.com/penwyp/catmit/internal/git"
)

type TagCIWorkflow struct {
	runner  git.Runner
	options TagCIOptions
	stdout  io.Writer
	stderr  io.Writer
	stdin   io.Reader
	getenv  func(string) string
	now     func() time.Time
}

func NewTagCIWorkflow(runner git.Runner, options TagCIOptions, stdout, stderr io.Writer, stdin io.Reader) *TagCIWorkflow {
	return &TagCIWorkflow{
		runner: runner, options: options, stdout: stdout, stderr: stderr, stdin: stdin,
		getenv: os.Getenv, now: time.Now,
	}
}

func (w *TagCIWorkflow) Run(ctx context.Context, command []string) error {
	if err := w.options.Validate(); err != nil {
		return err
	}
	info, err := ci.Read(w.options.Provider, w.getenv)
	if err != nil {
		return err
	}
	if _, err := w.runner.Run(ctx, "git", "rev-parse", "--git-dir"); err != nil {
		return fmt.Errorf("inspect build repository: %w", err)
	}
	if w.options.Checkout && info.Tag != "" {
		if err := w.checkoutTag(ctx, info.Tag); err != nil {
			return err
		}
	}
	head, err := w.runner.Run(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read build commit: %w", err)
	}
	head = strings.TrimSpace(head)
	branch, err := w.runner.Run(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return fmt.Errorf("read build branch: %w", err)
	}
	branch = strings.TrimSpace(branch)
	if branch == "HEAD" {
		branch = "detached"
	}
	code := branch + " @ " + head[:min(12, len(head))]
	version, source, err := w.resolveVersion(ctx, info.Tag, head)
	if err != nil {
		return err
	}
	w.stage("version", fmt.Sprintf("%s（%s）", version, source))
	if len(command) > 0 {
		w.stage("build", "开始执行："+strings.Join(command, " "))
		if err := w.build(ctx, command, version); err != nil {
			w.stage("build", "构建失败（"+code+"）")
			return fmt.Errorf("build %s: %w", command[0], err)
		}
		// Builds may generate files, but must not silently change the source commit.
		after, err := w.runner.Run(ctx, "git", "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("verify build commit: %w", err)
		}
		if strings.TrimSpace(after) != head {
			return fmt.Errorf("build changed HEAD from %s to %s", head, strings.TrimSpace(after))
		}
		w.stage("build", code+"  完成")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fmt.Fprintln(w.stderr, "  版本摘要")
	w.kv("触发", info.Trigger)
	w.kv("策略", strings.ReplaceAll(w.options.VersionOrder, ",", " > ")+"; env="+w.options.VersionEnv)
	w.kv("版本", version)
	w.kv("来源", source+"（"+ciSourceNote(source)+"）")
	w.kv("代码", code)
	w.kv("提交", head)
	_, err = fmt.Fprintln(w.stdout, version)
	return err
}

func (w *TagCIWorkflow) checkoutTag(ctx context.Context, tag string) error {
	if err := validateCIVersion(tag); err != nil {
		return fmt.Errorf("invalid webhook tag: %w", err)
	}
	if _, err := w.runner.Run(ctx, "git", "check-ref-format", "refs/tags/"+tag); err != nil {
		return fmt.Errorf("invalid webhook tag %q: %w", tag, err)
	}
	// Resolve the remote ref, never a potentially stale same-named local tag.
	manager := git.NewTagManager(w.runner)
	remoteCtx, cancel := context.WithTimeout(ctx, time.Duration(w.options.Version.Timeout)*time.Second)
	sha, err := manager.FetchRemoteTag(remoteCtx, w.options.Version.Remote, tag)
	cancel()
	if err != nil {
		return fmt.Errorf("resolve webhook tag %q: %w", tag, err)
	}
	if out, err := w.runner.Run(ctx, "git", "checkout", "--quiet", "--detach", sha); err != nil {
		return fmt.Errorf("checkout webhook tag %q: %w: %s", tag, err, strings.TrimSpace(out))
	}
	w.stage("checkout", fmt.Sprintf("tag %s → %s", tag, sha[:min(12, len(sha))]))
	return nil
}

func (w *TagCIWorkflow) resolveVersion(ctx context.Context, webhookTag, head string) (string, string, error) {
	for _, source := range w.options.sources() {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		var version string
		switch source {
		case "webhook-tag":
			version = webhookTag
		case "env":
			version = w.getenv(w.options.VersionEnv)
		case "history":
			var err error
			version, err = w.historyVersion(ctx, head)
			if ctx.Err() != nil {
				return "", "", ctx.Err()
			}
			if err != nil {
				w.stage("version", fmt.Sprintf("history 计算失败，继续尝试后续来源：%v", err))
				continue
			}
		case "timestamp":
			location, _ := time.LoadLocation(w.options.Version.Timezone) // Validated before any side effects.
			version = w.now().In(location).Format("20060102150405")
		}
		if version == "" {
			continue
		}
		if err := validateCIVersion(version); err != nil {
			return "", "", fmt.Errorf("invalid %s version: %w", source, err)
		}
		return version, source, nil
	}
	return "", "", fmt.Errorf("no usable version from --version-order %s", w.options.VersionOrder)
}

func (w *TagCIWorkflow) historyVersion(ctx context.Context, head string) (string, error) {
	shallow, err := w.runner.Run(ctx, "git", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return "", fmt.Errorf("inspect history depth: %w", err)
	}
	if strings.TrimSpace(shallow) == "true" {
		w.stage("history", "补全浅克隆历史")
		remoteCtx, cancel := context.WithTimeout(ctx, time.Duration(w.options.Version.Timeout)*time.Second)
		out, err := w.runner.Run(remoteCtx, "git", "fetch", "--unshallow", "--no-tags", w.options.Version.Remote)
		cancel()
		if err != nil {
			return "", fmt.Errorf("fetch full history: %w: %s", err, strings.TrimSpace(out))
		}
		shallow, err = w.runner.Run(ctx, "git", "rev-parse", "--is-shallow-repository")
		if err != nil {
			return "", fmt.Errorf("verify history depth: %w", err)
		}
		if strings.TrimSpace(shallow) != "false" {
			return "", fmt.Errorf("repository is still shallow after fetching full history")
		}
	}
	calculator := tagCalculator{manager: git.NewTagManager(w.runner), options: w.options.Version, now: w.now}
	calculation, err := calculator.calculate(ctx, head)
	if err != nil {
		return "", err
	}
	return calculation.plan.NextTag, nil
}

func (w *TagCIWorkflow) build(ctx context.Context, args []string, version string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = w.stdin, w.stderr, w.stderr
	cmd.Env = os.Environ()
	// Replace rather than append so stale inherited values cannot win.
	for _, key := range []string{"CATMIT_VERSION", w.options.VersionEnv} {
		filtered := cmd.Env[:0]
		for _, entry := range cmd.Env {
			if !strings.HasPrefix(entry, key+"=") {
				filtered = append(filtered, entry)
			}
		}
		cmd.Env = append(filtered, key+"="+version)
	}
	// catmit targets Linux and macOS. Cancel the build process group so shell
	// scripts cannot leave their compiler or other child processes running.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	return cmd.Run()
}

func (w *TagCIWorkflow) stage(stage, message string) {
	fmt.Fprintf(w.stderr, "[%s] %s\n", stage, message)
}

func (w *TagCIWorkflow) kv(key, value string) { fmt.Fprintf(w.stderr, "  %s  %s\n", key, value) }

func ciSourceNote(source string) string {
	switch source {
	case "webhook-tag":
		return "来自 tag 推送"
	case "env":
		return "上游手动指定"
	case "history":
		return "按提交历史生成"
	default:
		return "时间戳兜底"
	}
}
