package collector

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
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

// FileStatus 表示文件的Git状态信息
type FileStatus struct {
	Path        string // 文件路径
	IndexStatus rune   // 暂存区状态 (M, A, D, R, C等)
	WorkStatus  rune   // 工作区状态 (M, A, D, R, C等)
	IsRenamed   bool   // 是否为重命名
	OldPath     string // 重命名前的路径(如果适用)
}

// FileStatusSummary 文件状态摘要，包含分支信息和文件状态列表
type FileStatusSummary struct {
	BranchName string       // 当前分支名
	Files      []FileStatus // 文件状态列表
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

// shouldIgnoreFile 判断是否应该忽略某个文件
// 根据文档建议，过滤锁文件、构建产物、二进制文件等噪音
func shouldIgnoreFile(filePath string) bool {
	// 标准化路径（使用正斜杠）
	filePath = filepath.ToSlash(filePath)
	fileName := filepath.Base(filePath)
	
	// 1. 锁文件和依赖文件
	lockFiles := []string{
		"package-lock.json", "yarn.lock", "pnpm-lock.yaml", 
		"go.sum", "go.mod", "composer.lock", "Pipfile.lock",
		"poetry.lock", "Gemfile.lock", "mix.lock",
	}
	for _, lock := range lockFiles {
		if fileName == lock {
			return true
		}
	}
	
	// 2. 构建产物目录
	buildDirs := []string{
		"dist/", "build/", "target/", "out/", "bin/",
		"node_modules/", "vendor/", ".git/",
		"__pycache__/", ".pytest_cache/", ".coverage/",
		".vscode/", ".idea/", ".DS_Store",
	}
	for _, dir := range buildDirs {
		if strings.HasPrefix(filePath, dir) || strings.Contains(filePath, "/"+dir) {
			return true
		}
	}
	
	// 3. 二进制文件和媒体文件扩展名
	ext := strings.ToLower(filepath.Ext(fileName))
	binaryExts := []string{
		".exe", ".dll", ".so", ".dylib", ".a", ".lib",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg", ".ico",
		".mp3", ".mp4", ".avi", ".mov", ".pdf", ".zip", ".tar", ".gz",
		".woff", ".woff2", ".ttf", ".eot", ".otf",
	}
	for _, binExt := range binaryExts {
		if ext == binExt {
			return true
		}
	}
	
	// 4. 日志文件和临时文件
	if strings.HasSuffix(fileName, ".log") || 
	   strings.HasSuffix(fileName, ".tmp") || 
	   strings.HasSuffix(fileName, ".temp") ||
	   strings.HasSuffix(fileName, ".bak") ||
	   strings.HasSuffix(fileName, ".swp") ||
	   strings.HasPrefix(fileName, ".") && strings.HasSuffix(fileName, ".tmp") {
		return true
	}
	
	return false
}

// filterFiles 过滤文件列表，移除不需要的文件
func filterFiles(files []string) []string {
	var filtered []string
	for _, file := range files {
		if !shouldIgnoreFile(file) {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// parseGitStatusPorcelain 解析 git status --porcelain -b 的输出
// 返回文件状态摘要信息，包含分支名和文件状态列表
func parseGitStatusPorcelain(output string) (*FileStatusSummary, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return &FileStatusSummary{}, nil
	}
	
	summary := &FileStatusSummary{
		Files: make([]FileStatus, 0),
	}
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// 解析分支信息 (## branch_name)
		if strings.HasPrefix(line, "## ") {
			branchInfo := strings.TrimPrefix(line, "## ")
			// 处理分支名可能包含的跟踪信息 (如 "main...origin/main")
			if idx := strings.Index(branchInfo, "..."); idx != -1 {
				summary.BranchName = branchInfo[:idx]
			} else {
				summary.BranchName = branchInfo
			}
			continue
		}
		
		// 解析文件状态信息
		if len(line) < 3 {
			continue
		}
		
		indexStatus := rune(line[0])
		workStatus := rune(line[1])
		filePath := line[3:] // 跳过状态字符和空格
		
		fileStatus := FileStatus{
			IndexStatus: indexStatus,
			WorkStatus:  workStatus,
		}
		
		// 处理重命名情况 (R100 old_path -> new_path)
		if indexStatus == 'R' || indexStatus == 'C' {
			if idx := strings.Index(filePath, " -> "); idx != -1 {
				fileStatus.IsRenamed = true
				fileStatus.OldPath = filePath[:idx]
				fileStatus.Path = filePath[idx+4:]
			} else {
				fileStatus.Path = filePath
			}
		} else {
			fileStatus.Path = filePath
		}
		
		// 应用文件过滤逻辑
		if !shouldIgnoreFile(fileStatus.Path) {
			summary.Files = append(summary.Files, fileStatus)
		}
	}
	
	return summary, nil
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

// ChangedFiles 返回当前 staged 文件列表，已过滤掉不需要的文件类型。
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
	
	// 应用文件过滤逻辑，移除不需要的文件
	res = filterFiles(res)
	
	if len(res) == 0 {
		return []string{}, nil
	}
	return res, nil
}

// FileStatusSummary 返回详细的文件状态摘要信息
// 使用 git status --porcelain -b 获取完整的状态信息
func (c *Collector) FileStatusSummary(ctx context.Context) (*FileStatusSummary, error) {
	out, err := c.runner.Run(ctx, "git", "status", "--porcelain", "-b")
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain -b failed: %w", err)
	}
	
	summary, err := parseGitStatusPorcelain(string(out))
	if err != nil {
		return nil, fmt.Errorf("failed to parse git status output: %w", err)
	}
	
	return summary, nil
}

// getFilePriority 根据文件状态和类型计算优先级
// 返回值越小优先级越高
func getFilePriority(status FileStatus) int {
	// 1. 根据Git状态设置基础优先级
	var basePriority int
	switch status.IndexStatus {
	case 'A': // 新增文件 - 最高优先级
		basePriority = 10
	case 'M': // 修改文件 - 高优先级
		basePriority = 20
	case 'D': // 删除文件 - 中等优先级
		basePriority = 30
	case 'R': // 重命名文件 - 中等优先级
		basePriority = 35
	case 'C': // 复制文件 - 中等优先级
		basePriority = 40
	default: // 其他状态 - 较低优先级
		basePriority = 50
	}
	
	// 2. 根据文件扩展名调整优先级
	ext := strings.ToLower(filepath.Ext(status.Path))
	switch ext {
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".rs", ".rb":
		// 主要编程语言文件 - 优先级提升
		basePriority -= 5
	case ".md", ".txt", ".json", ".yaml", ".yml", ".xml":
		// 配置和文档文件 - 优先级略微提升
		basePriority -= 2
	case ".html", ".css", ".scss", ".less":
		// 前端文件 - 保持原优先级
		basePriority += 0
	default:
		// 其他文件 - 优先级降低
		basePriority += 5
	}
	
	// 3. 根据文件路径调整优先级
	if strings.Contains(status.Path, "test") || strings.Contains(status.Path, "spec") {
		// 测试文件 - 优先级降低
		basePriority += 10
	}
	
	return basePriority
}

// sortFilesByPriority 根据优先级对文件进行排序
// 根据文档建议，优先处理新增文件和修改量小的文件
func sortFilesByPriority(files []FileStatus) []FileStatus {
	// 创建副本避免修改原始切片
	sorted := make([]FileStatus, len(files))
	copy(sorted, files)
	
	sort.Slice(sorted, func(i, j int) bool {
		priorityI := getFilePriority(sorted[i])
		priorityJ := getFilePriority(sorted[j])
		
		// 优先级相同时，按文件名排序确保一致性
		if priorityI == priorityJ {
			return sorted[i].Path < sorted[j].Path
		}
		
		return priorityI < priorityJ
	})
	
	return sorted
}
