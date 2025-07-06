package collector

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Runner 抽象出命令执行器，方便在单元测试中注入 Mock。
// 实际运行时使用 exec.Command 实现。
//
// 返回值约定：成功时输出字节数组，错误时返回非 nil error。
// 日志输出由调用方处理。
//
// NOTE: 目前仅支持同步返回，后续可扩展为流式读取。
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Collector 负责收集 Git 日志与 diff 信息。
// 通过依赖注入的 Runner 以实现可测试性。
// 所有方法均以 context 控制生命周期。
type Collector struct {
	runner Runner
}

// New 创建 Collector 实例。
func New(r Runner) *Collector {
	return &Collector{runner: r}
}

// ErrNoDiff 表示当前仓库没有待提交的 diff。
var ErrNoDiff = fmt.Errorf("nothing to commit")

// 安全验证：确保分支名称和文件路径不包含危险字符
var (
	// 允许的分支名称格式：字母、数字、短划线、斜杠、点、下划线
	validBranchName = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)
	// 不允许的危险字符模式
	dangerousChars = regexp.MustCompile(`[;&|$\x00-\x1f\x7f-\x9f]`)
)

// sanitizeOutput 清理输出中的危险字符
func sanitizeOutput(s string) string {
	// 移除控制字符和潜在的危险字符
	return dangerousChars.ReplaceAllString(s, "")
}

// RecentCommits 返回最近 n 条 commit 信息（仅 subject 部分）。
func (c *Collector) RecentCommits(ctx context.Context, n int) ([]string, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be positive")
	}
	// 防止过大的 n 值导致性能问题
	if n > 1000 {
		return nil, fmt.Errorf("n too large, maximum is 1000")
	}

	out, err := c.runner.Run(ctx, "git", "log", "--pretty=format:%s", fmt.Sprintf("-n%d", n))
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	// 若 commit 数不足 n，返回实际获取到的数量。
	return lines, nil
}

// Diff 收集 staged 与 unstaged diff，若无差异返回 ErrNoDiff。
func (c *Collector) Diff(ctx context.Context) (string, error) {
	// --no-ext-diff 避免外部 diff 工具干扰，--cached 获取 staged diff。
	staged, err := c.runner.Run(ctx, "git", "diff", "--cached", "--no-ext-diff")
	if err != nil {
		return "", fmt.Errorf("git diff --cached failed: %w", err)
	}

	// 未暂存的改动。
	unstaged, err := c.runner.Run(ctx, "git", "diff", "--no-ext-diff")
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}

	combined := string(staged) + string(unstaged)
	combined = strings.TrimSpace(combined)
	if combined == "" {
		// 可能是新文件删除等导致 diff 为空，检查 git status
		status, err := c.runner.Run(ctx, "git", "status", "--porcelain")
		if err != nil {
			return "", fmt.Errorf("git status --porcelain failed: %w", err)
		}
		statusStr := strings.TrimSpace(string(status))
		if statusStr == "" {
			return "", ErrNoDiff
		}
		return statusStr, nil
	}
	return combined, nil
}

// BranchName 返回当前 Git 分支名称。
func (c *Collector) BranchName(ctx context.Context) (string, error) {
	out, err := c.runner.Run(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}
	branchName := strings.TrimSpace(string(out))
	// 安全验证：确保分支名称格式合法
	if !validBranchName.MatchString(branchName) {
		return "", fmt.Errorf("invalid branch name format: %s", sanitizeOutput(branchName))
	}
	return branchName, nil
}

// ChangedFiles 返回当前 staged 文件列表。
func (c *Collector) ChangedFiles(ctx context.Context) ([]string, error) {
	out, err := c.runner.Run(ctx, "git", "diff", "--cached", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only failed: %w", err)
	}
	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	// untracked files
	untracked, _ := c.runner.Run(ctx, "git", "ls-files", "--others", "--exclude-standard")
	if len(untracked) > 0 {
		files = append(files, strings.Split(strings.TrimSpace(string(untracked)), "\n")...)
	}
	// remove empty and sanitize file paths
	var res []string
	for _, f := range files {
		if f != "" {
			// 清理文件路径中的危险字符，但保留路径分隔符
			sanitized := sanitizeOutput(f)
			res = append(res, sanitized)
		}
	}
	if len(res) == 0 {
		return []string{}, nil
	}
	return res, nil
}
