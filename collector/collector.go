// Package collector provides Git repository analysis and data collection functionality
// for the catmit tool. It implements a modern, interface-segregated architecture
// with performance optimizations including caching, batched operations, and retry logic.
//
// Architecture Overview:
// The collector package follows interface segregation principles with focused interfaces:
// - GitReader: Pure git command execution
// - ChangeAnalyzer: High-level change analysis 
// - FileContentProvider: File content access
// - EnhancedDiffProvider: Comprehensive diff including untracked files
//
// Performance Features (Phase 3):
// - Command result caching with configurable TTL
// - Batched git operations for concurrent execution
// - Memory optimizations for large file lists
// - Retry logic with exponential backoff
// - Enhanced error handling with detailed context
//
// Usage Example:
//   runner := &RealRunner{}
//   collector := New(runner)
//   
//   // Get comprehensive diff including untracked files
//   diff, err := collector.ComprehensiveDiff(ctx)
//   if err != nil {
//       return errors.Wrap(errors.ErrTypeGit, "failed to get diff", err)
//   }
//   
//   // Analyze changes for commit message generation
//   changes, err := collector.AnalyzeChanges(ctx)
//   if err != nil {
//       return errors.Wrap(errors.ErrTypeGit, "failed to analyze changes", err)
//   }
package collector

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	
	"github.com/penwyp/catmit/internal/errors"
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
	// Existing fields
	Path        string // 文件路径
	IndexStatus rune   // 暂存区状态 (M, A, D, R, C等)
	WorkStatus  rune   // 工作区状态 (M, A, D, R, C等)
	IsRenamed   bool   // 是否为重命名
	OldPath     string // 重命名前的路径(如果适用)
	
	// New enhanced fields for Phase 2
	Priority        int    // Priority score (1-100, lower is higher priority)
	ContentType     string // File content type (code, config, docs, test, etc.)
	ChangeMagnitude string // Estimated change magnitude (small, medium, large)
	AffectedArea    string // Primary affected area (frontend, backend, etc.)
	IsUntracked     bool   // Whether this is an untracked file
	FileSize        int64  // File size in bytes (for untracked files)
}

// FileStatusSummary 文件状态摘要，包含分支信息和文件状态列表
type FileStatusSummary struct {
	BranchName string       // 当前分支名
	Files      []FileStatus // 文件状态列表
}

// CacheEntry represents a cached git command result with metadata.
// Each entry includes the command output, execution timestamp, and any error
// that occurred during execution.
type CacheEntry struct {
	result    []byte    // Command output bytes
	timestamp time.Time // When the command was executed
	err       error     // Error from command execution (if any)
}

// PerformanceCache provides thread-safe caching for git commands to improve performance.
// It implements a simple TTL-based cache that automatically expires old entries.
//
// The cache is particularly effective for:
// - Branch name lookups (rarely change during a session)
// - Recent commit history (static for a given HEAD)
// - Status information when called multiple times
//
// Thread Safety:
// The cache uses RWMutex to allow concurrent reads while protecting writes.
// This enables multiple goroutines to read cached values simultaneously.
//
// Memory Management:
// Old entries can be cleaned manually using CleanExpiredCache() or by
// calling ClearCache() to remove all entries.
type PerformanceCache struct {
	cache map[string]*CacheEntry // Key-value store for cached results
	mutex sync.RWMutex           // Protects concurrent access to cache
	ttl   time.Duration          // Time-to-live for cache entries
}

// NewPerformanceCache creates a new performance cache with specified TTL
func NewPerformanceCache(ttl time.Duration) *PerformanceCache {
	return &PerformanceCache{
		cache: make(map[string]*CacheEntry),
		ttl:   ttl,
	}
}

// Get retrieves a cached result if it exists and is still valid
func (pc *PerformanceCache) Get(key string) ([]byte, error, bool) {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	
	entry, exists := pc.cache[key]
	if !exists {
		return nil, nil, false
	}
	
	// Check if cache entry is still valid
	if time.Since(entry.timestamp) > pc.ttl {
		return nil, nil, false
	}
	
	return entry.result, entry.err, true
}

// Set stores a result in the cache
func (pc *PerformanceCache) Set(key string, result []byte, err error) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	
	pc.cache[key] = &CacheEntry{
		result:    result,
		timestamp: time.Now(),
		err:       err,
	}
}

// Clear removes all cached entries
func (pc *PerformanceCache) Clear() {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	
	pc.cache = make(map[string]*CacheEntry)
}

// Collector 负责收集 Git 日志与 diff 信息。
// 通过依赖注入的 Runner 以实现可测试性。
// 所有方法均以 context 控制生命周期。
//
// Collector implements the new focused interfaces:
// - GitReader: for pure git command execution
// - ChangeAnalyzer: for high-level change analysis
// - FileContentProvider: for file content access
//
// Phase 3 Enhancement: Added performance optimizations including caching,
// batched operations, memory optimizations, and enhanced error handling.
type Collector struct {
	runner      Runner
	cache       *PerformanceCache
	retryConfig *RetryConfig
}

// New 创建 Collector 实例。
func New(r Runner) *Collector {
	return &Collector{
		runner:      r,
		cache:       NewPerformanceCache(30 * time.Second), // 30-second cache TTL
		retryConfig: DefaultRetryConfig(),
	}
}

// NewWithCache 创建带有自定义缓存配置的 Collector 实例。
func NewWithCache(r Runner, cacheTTL time.Duration) *Collector {
	return &Collector{
		runner:      r,
		cache:       NewPerformanceCache(cacheTTL),
		retryConfig: DefaultRetryConfig(),
	}
}

// NewWithConfig 创建带有自定义配置的 Collector 实例。
func NewWithConfig(r Runner, cacheTTL time.Duration, retryConfig *RetryConfig) *Collector {
	return &Collector{
		runner:      r,
		cache:       NewPerformanceCache(cacheTTL),
		retryConfig: retryConfig,
	}
}

// runWithCache executes a git command with caching support
// Cache key is generated from command name and arguments
func (c *Collector) runWithCache(ctx context.Context, name string, args ...string) ([]byte, error) {
	// Generate cache key from command and args
	cacheKey := name + ":" + strings.Join(args, ":")
	
	// Try to get from cache first
	if result, err, found := c.cache.Get(cacheKey); found {
		return result, err
	}
	
	// Execute command and cache result
	result, err := c.runner.Run(ctx, name, args...)
	
	// Check if error indicates not being in a git repository
	if err != nil && isNotGitRepositoryError(err) {
		err = errors.ErrNoGitRepo
	}
	
	c.cache.Set(cacheKey, result, err)
	
	return result, err
}

// ClearCache clears the performance cache
// Useful for testing or when you need fresh results
func (c *Collector) ClearCache() {
	c.cache.Clear()
}

// BatchGitOperations represents a batch of git operations that can be executed concurrently.
// This is particularly useful for operations that are independent of each other,
// such as getting staged files and untracked files simultaneously.
//
// Benefits:
// - Improved performance through parallel execution
// - Better resource utilization
// - Reduced total execution time for multiple git commands
//
// Thread Safety:
// The batch operations use goroutines with WaitGroup synchronization.
// Each operation runs in its own goroutine and results are collected safely.
//
// Example Usage:
//   batch := NewBatchGitOperations()
//   batch.AddOperation(func(ctx context.Context) (interface{}, error) {
//       return collector.StagedDiff(ctx)
//   })
//   batch.AddOperation(func(ctx context.Context) (interface{}, error) {
//       return collector.UntrackedFiles(ctx)
//   })
//   batch.ExecuteBatch(ctx)
//   results, errors := batch.GetResults()
type BatchGitOperations struct {
	operations []func(context.Context) (interface{}, error) // Functions to execute
	results    []interface{}                                // Results from operations
	errors     []error                                      // Errors from operations
}

// NewBatchGitOperations creates a new batch operations instance
func NewBatchGitOperations() *BatchGitOperations {
	return &BatchGitOperations{
		operations: make([]func(context.Context) (interface{}, error), 0),
	}
}

// AddOperation adds a git operation to the batch
func (b *BatchGitOperations) AddOperation(op func(context.Context) (interface{}, error)) {
	b.operations = append(b.operations, op)
}

// ExecuteBatch executes all operations in the batch concurrently
func (b *BatchGitOperations) ExecuteBatch(ctx context.Context) {
	b.results = make([]interface{}, len(b.operations))
	b.errors = make([]error, len(b.operations))
	
	var wg sync.WaitGroup
	for i, op := range b.operations {
		wg.Add(1)
		go func(index int, operation func(context.Context) (interface{}, error)) {
			defer wg.Done()
			result, err := operation(ctx)
			b.results[index] = result
			b.errors[index] = err
		}(i, op)
	}
	wg.Wait()
}

// GetResults returns the results of all operations
func (b *BatchGitOperations) GetResults() ([]interface{}, []error) {
	return b.results, b.errors
}

// optimizeStringSlice optimizes string slices to reduce memory allocation
func optimizeStringSlice(slice []string) []string {
	if len(slice) == 0 {
		return []string{}
	}
	
	// Remove empty strings and duplicates
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice))
	
	for _, s := range slice {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, trimmed)
		}
	}
	
	return result
}

// ErrNoDiff 表示当前仓库没有待提交的 diff。
var ErrNoDiff = errors.New(errors.ErrTypeGit, "nothing to commit").WithSuggestion("使用 'git add' 暂存您的更改")

// Enhanced error types for better error handling and categorization.
// These provide semantic meaning to different types of failures.
var (
	ErrGitCommandFailed  = errors.New(errors.ErrTypeGit, "git command failed")
	ErrInvalidRepository = errors.New(errors.ErrTypeGit, "not a valid git repository").WithSuggestion("请在 Git 仓库中运行此命令")
	ErrNotGitRepository  = errors.ErrNoGitRepo // Use predefined error from framework
	ErrNetworkTimeout    = errors.ErrNetworkTimeout // Use predefined error from framework
	ErrPermissionDenied  = errors.New(errors.ErrTypeGit, "permission denied").WithSuggestion("检查文件和目录权限")
)

// GitError represents a structured error from git operations with rich context.
// This error type provides comprehensive information about what went wrong,
// enabling better error handling and debugging.
//
// Fields:
// - Command: The git command that failed (e.g., "git")
// - Args: Command arguments (e.g., ["diff", "--cached"])
// - ExitCode: Process exit code (if available)
// - Stderr/Stdout: Command output for debugging
// - Cause: Underlying error that caused the failure
// - Context: Human-readable context about the failure
// - Timestamp: When the error occurred
//
// Example Usage:
//   var gitErr *GitError
//   if errors.As(err, &gitErr) {
//       log.Printf("Git command failed: %s %v (exit: %d) - %s",
//           gitErr.Command, gitErr.Args, gitErr.ExitCode, gitErr.Context)
//   }
type GitError struct {
	Command   string    // Git command that failed
	Args      []string  // Command arguments
	ExitCode  int       // Process exit code
	Stderr    string    // Standard error output
	Stdout    string    // Standard output
	Cause     error     // Underlying error
	Context   string    // Additional context
	Timestamp time.Time // When the error occurred
}

// Error implements the error interface
func (e *GitError) Error() string {
	return fmt.Sprintf("git command failed: %s %s (exit code: %d) - %s",
		e.Command, strings.Join(e.Args, " "), e.ExitCode, e.Context)
}

// ToCatmitError converts GitError to CatmitError
func (e *GitError) ToCatmitError() *errors.CatmitError {
	return errors.Wrap(errors.ErrTypeGit, e.Error(), e.Cause)
}

// Unwrap returns the underlying error
func (e *GitError) Unwrap() error {
	return e.Cause
}

// RetryConfig defines retry behavior for git operations
type RetryConfig struct {
	MaxRetries int
	InitialDelay time.Duration
	MaxDelay time.Duration
	BackoffFactor float64
}

// DefaultRetryConfig provides sensible defaults for retry behavior
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries: 3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay: 5 * time.Second,
		BackoffFactor: 2.0,
	}
}

// runWithRetry executes a git command with retry logic
func (c *Collector) runWithRetry(ctx context.Context, config *RetryConfig, name string, args ...string) ([]byte, error) {
	var lastErr error
	delay := config.InitialDelay
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retrying
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		
		result, err := c.runWithCache(ctx, name, args...)
		if err == nil {
			return result, nil
		}
		
		lastErr = err
		
		// Check if error indicates not being in a git repository
		if isNotGitRepositoryError(err) {
			return nil, errors.ErrNoGitRepo
		}
		
		// Check if error is retryable
		if !isRetryableError(err) {
			break
		}
		
		// Calculate next delay with exponential backoff
		delay = time.Duration(float64(delay) * config.BackoffFactor)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
		}
	}
	
	// Create enhanced error with context
	gitErr := &GitError{
		Command:   name,
		Args:      args,
		Cause:     lastErr,
		Context:   fmt.Sprintf("failed after %d attempts", config.MaxRetries+1),
		Timestamp: time.Now(),
	}
	return nil, gitErr.ToCatmitError()
}

// isRetryableError determines if an error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	
	// Network-related errors are retryable
	if strings.Contains(errStr, "network") || 
	   strings.Contains(errStr, "timeout") ||
	   strings.Contains(errStr, "connection") {
		return true
	}
	
	// Temporary file system errors are retryable
	if strings.Contains(errStr, "resource temporarily unavailable") ||
	   strings.Contains(errStr, "device busy") {
		return true
	}
	
	// Permission errors are generally not retryable
	if strings.Contains(errStr, "permission denied") ||
	   strings.Contains(errStr, "access denied") {
		return false
	}
	
	return false
}

// isNotGitRepositoryError determines if an error indicates we're not in a git repository
// This helps provide user-friendly error messages for non-git directories
func isNotGitRepositoryError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	
	// Common patterns that indicate not being in a git repository
	patterns := []string{
		"not a git repository",
		"Not a git repository",
		"fatal: not a git repository",
		"fatal: Not a git repository",
		// Exit code 129 with these specific git commands often indicates non-git directory
		"git diff --cached failed: exit status 129",
		"git status --porcelain failed: exit status 129",
		"git rev-parse --abbrev-ref HEAD failed: exit status 129",
		// Another common pattern
		"fatal: not a git repository (or any of the parent directories): .git",
	}
	
	for _, pattern := range patterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	
	return false
}

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
// Currently unused but kept for potential future use
// nolint:unused
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
// Phase 3 Enhancement: Added caching and memory optimization
func (c *Collector) RecentCommits(ctx context.Context, n int) ([]string, error) {
	if n <= 0 {
		return nil, errors.New(errors.ErrTypeValidation, "n must be positive")
	}
	// 防止过大的 n 值导致性能问题
	if n > 1000 {
		return nil, errors.New(errors.ErrTypeValidation, "n too large, maximum is 1000")
	}

	// Use cached execution for better performance
	out, err := c.runWithCache(ctx, "git", "log", "--pretty=format:%s", fmt.Sprintf("-n%d", n))
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "git log failed", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	// 若 commit 数不足 n，返回实际获取到的数量。
	// Apply memory optimization to the result
	return optimizeStringSlice(lines), nil
}

// Diff 收集 staged 与 unstaged diff，若无差异返回 ErrNoDiff。
// DEPRECATED: Use ComprehensiveDiff for including untracked files
//
// Phase 3 Enhancement: This method now uses ComprehensiveDiff by default
// with fallback to legacy behavior for backward compatibility.
func (c *Collector) Diff(ctx context.Context) (string, error) {
	// Try the new comprehensive diff first to include untracked files
	comprehensiveDiff, err := c.ComprehensiveDiff(ctx)
	if err == nil {
		return comprehensiveDiff, nil
	}
	
	// If comprehensive diff fails, fall back to legacy behavior
	// This ensures backward compatibility while gradually migrating to new behavior
	if !errors.Is(err, ErrNoDiff) {
		// Log the error but continue with fallback
		// In production, this could be logged for monitoring
		// fmt.Printf("ComprehensiveDiff failed, falling back to legacy: %v\n", err)
		_ = err // explicitly ignore the error for linting
	}
	
	return c.CombinedDiff(ctx)
}

// BranchName 返回当前 Git 分支名称。
// Phase 3 Enhancement: Added caching for better performance
func (c *Collector) BranchName(ctx context.Context) (string, error) {
	// Use cached execution - branch name rarely changes during a session
	out, err := c.runWithCache(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "git rev-parse failed", err)
	}
	branchName := strings.TrimSpace(string(out))
	// 安全验证：确保分支名称格式合法
	if !validBranchName.MatchString(branchName) {
		return "", errors.New(errors.ErrTypeValidation, fmt.Sprintf("invalid branch name format: %s", sanitizeOutput(branchName)))
	}
	return branchName, nil
}

// ChangedFiles 返回当前 staged 文件列表，已过滤掉不需要的文件类型。
// Phase 3 Enhancement: Added batched operations and memory optimization
func (c *Collector) ChangedFiles(ctx context.Context) ([]string, error) {
	// Use batched operations to get both staged and untracked files concurrently
	batch := NewBatchGitOperations()
	
	// Add staged files operation
	batch.AddOperation(func(ctx context.Context) (interface{}, error) {
		return c.runWithCache(ctx, "git", "diff", "--cached", "--name-only")
	})
	
	// Add untracked files operation
	batch.AddOperation(func(ctx context.Context) (interface{}, error) {
		return c.runWithCache(ctx, "git", "ls-files", "--others", "--exclude-standard")
	})
	
	// Execute batch operations
	batch.ExecuteBatch(ctx)
	results, errs := batch.GetResults()
	
	// Check for errors in staged files
	if errs[0] != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "git diff --name-only failed", errs[0])
	}
	
	// Process staged files
	stagedOut := results[0].([]byte)
	files := strings.Split(strings.TrimSpace(string(stagedOut)), "\n")
	
	// Process untracked files (ignore errors for untracked files)
	if errs[1] == nil && len(results[1].([]byte)) > 0 {
		untrackedOut := results[1].([]byte)
		untrackedFiles := strings.Split(strings.TrimSpace(string(untrackedOut)), "\n")
		files = append(files, untrackedFiles...)
	}
	
	// Use the new helper function to process and filter files
	res := c.processAndFilterFiles(files)
	
	if len(res) == 0 {
		return []string{}, nil
	}
	return res, nil
}

// FileStatusSummary 返回详细的文件状态摘要信息
// 使用 git status --porcelain -b 获取完整的状态信息
// Phase 3 Enhancement: Uses cached execution for better performance
func (c *Collector) FileStatusSummary(ctx context.Context) (*FileStatusSummary, error) {
	out, err := c.runWithCache(ctx, "git", "status", "--porcelain", "-b")
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "git status --porcelain -b failed", err)
	}
	
	summary, err := parseGitStatusPorcelain(string(out))
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to parse git status output", err)
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

// ============================================================================
// GitReader Interface Implementation
// ============================================================================

// StagedDiff returns staged changes (equivalent to `git diff --cached`)
// Returns ErrNoDiff if no staged changes are found
// Phase 3 Enhancement: Uses cached execution for better performance
func (c *Collector) StagedDiff(ctx context.Context) (string, error) {
	return c.executeDiffCommand(ctx, "git diff --cached failed", ErrNoDiff, "git", "diff", "--cached", "--no-ext-diff")
}

// UnstagedDiff returns unstaged changes (equivalent to `git diff`)
// Returns empty string if no unstaged changes are found
// Phase 3 Enhancement: Uses cached execution for better performance
func (c *Collector) UnstagedDiff(ctx context.Context) (string, error) {
	return c.executeDiffCommand(ctx, "git diff failed", nil, "git", "diff", "--no-ext-diff")
}

// executeDiffCommand is a helper function to reduce code duplication in diff operations
func (c *Collector) executeDiffCommand(ctx context.Context, errMsg string, emptyErr error, name string, args ...string) (string, error) {
	result, err := c.runWithCache(ctx, name, args...)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, errMsg, err)
	}
	
	resultStr := strings.TrimSpace(string(result))
	if resultStr == "" && emptyErr != nil {
		return "", emptyErr
	}
	
	return resultStr, nil
}

// GitStatus returns git status in porcelain format
// Format: XY filename, where X is index status, Y is worktree status
// Phase 3 Enhancement: Uses cached execution for better performance
func (c *Collector) GitStatus(ctx context.Context) (string, error) {
	return c.executeDiffCommand(ctx, "git status --porcelain failed", nil, "git", "status", "--porcelain")
}

// ============================================================================
// ChangeAnalyzer Interface Implementation
// ============================================================================

// AnalyzeChanges provides a high-level summary of all changes
// Combines staged and unstaged changes into a comprehensive analysis
func (c *Collector) AnalyzeChanges(ctx context.Context) (*ChangesSummary, error) {
	summary, err := c.FileStatusSummary(ctx)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to get file status summary", err)
	}
	
	changes := &ChangesSummary{
		ChangeTypes: make(map[string]int),
		AffectedAreas: []string{},
		UntrackedFiles: []FileStatus{},
	}
	
	// Get untracked files
	untrackedFiles, err := c.UntrackedFiles(ctx)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to get untracked files", err)
	}
	
	// Process untracked files
	for _, untrackedPath := range untrackedFiles {
		untrackedFileStatus := FileStatus{
			Path:        untrackedPath,
			IndexStatus: '?',
			WorkStatus:  '?',
			IsUntracked: true,
		}
		
		// Enhance with metadata
		untrackedFileStatus = c.enhanceFileStatus(untrackedFileStatus)
		changes.UntrackedFiles = append(changes.UntrackedFiles, untrackedFileStatus)
	}
	
	// Analyze all files (tracked + untracked)
	allFiles := append(summary.Files, changes.UntrackedFiles...)
	
	for _, file := range allFiles {
		changes.TotalFiles++
		
		// Check if file is staged
		if file.IndexStatus != ' ' && file.IndexStatus != '?' {
			changes.HasStagedChanges = true
		}
		
		// Check if file has unstaged changes
		if file.WorkStatus != ' ' && file.WorkStatus != 0 {
			changes.HasUnstagedChanges = true
		}
		
		// Check for untracked files
		if file.IsUntracked {
			changes.HasUntrackedFiles = true
		}
		
		// Categorize change types
		switch file.IndexStatus {
		case 'A':
			changes.ChangeTypes["added"]++
		case 'M':
			changes.ChangeTypes["modified"]++
		case 'D':
			changes.ChangeTypes["deleted"]++
		case 'R':
			changes.ChangeTypes["renamed"]++
		case 'C':
			changes.ChangeTypes["copied"]++
		case '?':
			changes.ChangeTypes["untracked"]++
		}
	}
	
	// Set total changed files including untracked
	changes.TotalChangedFiles = len(allFiles)
	
	// Determine change magnitude
	changes.Magnitude = c.calculateChangeMagnitude(changes.TotalChangedFiles)
	
	// Calculate priority
	changes.Priority = c.calculatePriority(changes)
	
	// Determine primary change type
	changes.PrimaryChangeType = determinePrimaryChangeType(changes.ChangeTypes)
	
	// Set suggested prefix
	changes.SuggestedPrefix = c.determineSuggestedPrefix(changes)
	
	// Analyze affected areas
	changes.AffectedAreas = analyzeAffectedAreas(allFiles)
	
	// Sort files by priority
	changes.FilesByPriority = c.sortFilesByPriorityEnhanced(allFiles)
	
	return changes, nil
}

// GetPriorityFiles returns files ordered by importance for commit message generation
// Files are sorted by: change type priority, file type importance, and path
func (c *Collector) GetPriorityFiles(ctx context.Context) ([]FileStatus, error) {
	summary, err := c.FileStatusSummary(ctx)
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "failed to get file status summary", err)
	}
	
	// Use existing sorting logic
	return sortFilesByPriority(summary.Files), nil
}

// ============================================================================
// FileContentProvider Interface Implementation
// ============================================================================

// UntrackedFiles returns list of untracked files that are not ignored
// Files are filtered to exclude build artifacts, dependencies, and binary files
// Phase 3 Enhancement: Uses cached execution and optimized filtering
func (c *Collector) UntrackedFiles(ctx context.Context) ([]string, error) {
	untracked, err := c.runWithCache(ctx, "git", "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, errors.Wrap(errors.ErrTypeGit, "git ls-files --others --exclude-standard failed", err)
	}
	
	files := strings.Split(strings.TrimSpace(string(untracked)), "\n")
	result := c.processAndFilterFiles(files)
	
	return result, nil
}

// processAndFilterFiles is a helper function to process and filter file lists
// Reduces code duplication across multiple methods
func (c *Collector) processAndFilterFiles(files []string) []string {
	var result []string
	
	for _, file := range files {
		if file != "" && !shouldIgnoreFile(file) {
			result = append(result, sanitizeOutput(file))
		}
	}
	
	return optimizeStringSlice(result)
}

// UntrackedFileContent returns the content of an untracked file
// Returns error if file doesn't exist, is binary, or can't be read
// Content is truncated if file is too large (>10KB by default)
func (c *Collector) UntrackedFileContent(ctx context.Context, path string) (string, error) {
	// Security check: sanitize file path
	sanitizedPath := sanitizeOutput(path)
	if sanitizedPath != path {
		return "", errors.New(errors.ErrTypeValidation, fmt.Sprintf("invalid file path: %s", path))
	}
	
	// Check if file should be ignored
	if shouldIgnoreFile(path) {
		return "", errors.New(errors.ErrTypeValidation, fmt.Sprintf("file type not supported: %s", path))
	}
	
	// Use head to limit file size (10KB = 10240 bytes, approximately 640 lines of 16 chars each)
	content, err := c.runner.Run(ctx, "head", "-c", "10240", path)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, fmt.Sprintf("failed to read file %s", path), err)
	}
	
	return string(content), nil
}

// UntrackedFileAsDiff formats an untracked file's content as a diff-like output
// This is essential for including untracked files in commit message generation
func (c *Collector) UntrackedFileAsDiff(ctx context.Context, path string) (string, error) {
	content, err := c.UntrackedFileContent(ctx, path)
	if err != nil {
		return "", err
	}
	
	// Format as diff-like output
	lines := strings.Split(content, "\n")
	var diffLines []string
	
	// Add diff header
	diffLines = append(diffLines, fmt.Sprintf("diff --git a/%s b/%s", path, path))
	diffLines = append(diffLines, "new file mode 100644")
	diffLines = append(diffLines, "index 0000000..1234567")
	diffLines = append(diffLines, "--- /dev/null")
	diffLines = append(diffLines, fmt.Sprintf("+++ b/%s", path))
	
	// Add content lines with + prefix
	for _, line := range lines {
		diffLines = append(diffLines, "+"+line)
	}
	
	return strings.Join(diffLines, "\n"), nil
}

// ============================================================================
// EnhancedDiffProvider Interface Implementation
// ============================================================================

// ComprehensiveDiff returns a complete diff including staged, unstaged, and untracked files
// This is the primary method for getting all changes for commit message generation
func (c *Collector) ComprehensiveDiff(ctx context.Context) (string, error) {
	var diffParts []string
	
	// 1. Get staged diff
	stagedDiff, err := c.StagedDiff(ctx)
	if err != nil && !errors.Is(err, ErrNoDiff) {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get staged diff", err)
	}
	if stagedDiff != "" {
		diffParts = append(diffParts, stagedDiff)
	}
	
	// 2. Get unstaged diff
	unstagedDiff, err := c.UnstagedDiff(ctx)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get unstaged diff", err)
	}
	if unstagedDiff != "" {
		diffParts = append(diffParts, unstagedDiff)
	}
	
	// 3. Get untracked files as diff - THIS IS THE CORE FIX
	untrackedFiles, err := c.UntrackedFiles(ctx)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "failed to get untracked files", err)
	}
	
	// Convert untracked files to diff format
	for _, file := range untrackedFiles {
		fileDiff, err := c.UntrackedFileAsDiff(ctx, file)
		if err != nil {
			// Skip files that can't be read, but log the issue
			continue
		}
		diffParts = append(diffParts, fileDiff)
	}
	
	// 4. Combine all diffs
	combined := strings.Join(diffParts, "\n\n")
	combined = strings.TrimSpace(combined)
	
	if combined == "" {
		// Check if there are any changes at all
		status, err := c.GitStatus(ctx)
		if err != nil {
			return "", errors.Wrap(errors.ErrTypeGit, "git status failed", err)
		}
		if strings.TrimSpace(status) == "" {
			return "", ErrNoDiff
		}
		// Return status if no diff but there are changes
		return status, nil
	}
	
	return combined, nil
}

// CombinedDiff returns staged and unstaged diffs combined (legacy behavior)
func (c *Collector) CombinedDiff(ctx context.Context) (string, error) {
	// --no-ext-diff 避免外部 diff 工具干扰，--cached 获取 staged diff。
	staged, err := c.runner.Run(ctx, "git", "diff", "--cached", "--no-ext-diff")
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "git diff --cached failed", err)
	}

	// 未暂存的改动。
	unstaged, err := c.runner.Run(ctx, "git", "diff", "--no-ext-diff")
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeGit, "git diff failed", err)
	}

	combined := string(staged) + string(unstaged)
	combined = strings.TrimSpace(combined)
	if combined == "" {
		// 可能是新文件删除等导致 diff 为空，检查 git status
		status, err := c.runner.Run(ctx, "git", "status", "--porcelain")
		if err != nil {
			return "", errors.Wrap(errors.ErrTypeGit, "git status --porcelain failed", err)
		}
		statusStr := strings.TrimSpace(string(status))
		if statusStr == "" {
			return "", ErrNoDiff
		}
		return statusStr, nil
	}
	return combined, nil
}

// ============================================================================
// Helper Functions for Analysis
// ============================================================================

// determinePrimaryChangeType determines the most significant change type
func determinePrimaryChangeType(changeTypes map[string]int) string {
	if changeTypes["added"] > 0 {
		return "feat" // New files usually indicate new features
	}
	if changeTypes["deleted"] > 0 {
		return "chore" // Deletions are often cleanup
	}
	if changeTypes["renamed"] > 0 {
		return "refactor" // Renames indicate refactoring
	}
	if changeTypes["modified"] > 0 {
		return "fix" // Modifications could be fixes or improvements
	}
	if changeTypes["untracked"] > 0 {
		return "feat" // New untracked files usually indicate new features
	}
	return "chore" // Default fallback
}

// enhanceFileStatus adds metadata to a FileStatus
func (c *Collector) enhanceFileStatus(status FileStatus) FileStatus {
	// Calculate priority
	status.Priority = getFilePriority(status)
	
	// Determine content type
	status.ContentType = c.determineContentType(status.Path)
	
	// Determine affected area
	status.AffectedArea = c.determineAffectedArea(status.Path)
	
	// For untracked files, we could get file size, but it's optional
	// status.FileSize = c.getFileSize(status.Path)
	
	return status
}

// determineContentType categorizes the file by its content type
func (c *Collector) determineContentType(path string) string {
	// Check for test files first (by path pattern)
	if strings.Contains(path, "test") || strings.Contains(path, "spec") {
		return "test"
	}
	
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".rs", ".rb", ".php":
		return "code"
	case ".json", ".yaml", ".yml", ".xml", ".toml", ".ini", ".conf":
		return "config"
	case ".md", ".txt", ".rst", ".adoc":
		return "docs"
	case ".html", ".css", ".scss", ".less", ".vue", ".jsx", ".tsx":
		return "frontend"
	case ".sql", ".db":
		return "database"
	default:
		return "other"
	}
}

// determineAffectedArea determines the primary affected area
func (c *Collector) determineAffectedArea(path string) string {
	dir := filepath.Dir(path)
	if dir != "." {
		parts := strings.Split(dir, "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return "root"
}

// calculateChangeMagnitude determines the scale of changes
func (c *Collector) calculateChangeMagnitude(totalFiles int) ChangeMagnitude {
	if totalFiles <= 3 {
		return ChangeMagnitudeSmall
	} else if totalFiles <= 10 {
		return ChangeMagnitudeMedium
	}
	return ChangeMagnitudeLarge
}

// calculatePriority calculates overall change priority
func (c *Collector) calculatePriority(changes *ChangesSummary) int {
	priority := 50 // Base priority
	
	// Higher priority for more files
	if changes.TotalChangedFiles > 10 {
		priority += 20
	} else if changes.TotalChangedFiles > 5 {
		priority += 10
	}
	
	// Higher priority for new files
	if changes.ChangeTypes["added"] > 0 || changes.ChangeTypes["untracked"] > 0 {
		priority += 15
	}
	
	// Higher priority for deletions
	if changes.ChangeTypes["deleted"] > 0 {
		priority += 10
	}
	
	// Ensure priority is within bounds
	if priority > 100 {
		priority = 100
	}
	if priority < 1 {
		priority = 1
	}
	
	return priority
}

// determineSuggestedPrefix suggests a commit prefix based on changes
func (c *Collector) determineSuggestedPrefix(changes *ChangesSummary) string {
	if changes.ChangeTypes["added"] > 0 || changes.ChangeTypes["untracked"] > 0 {
		return "feat"
	}
	if changes.ChangeTypes["deleted"] > 0 {
		return "chore"
	}
	if changes.ChangeTypes["renamed"] > 0 {
		return "refactor"
	}
	if changes.ChangeTypes["modified"] > 0 {
		// Check if it's likely a bug fix or improvement
		return "fix"
	}
	return "chore"
}

// sortFilesByPriorityEnhanced sorts files with enhanced priority logic
func (c *Collector) sortFilesByPriorityEnhanced(files []FileStatus) []FileStatus {
	// Enhance all files with metadata
	enhanced := make([]FileStatus, len(files))
	for i, file := range files {
		enhanced[i] = c.enhanceFileStatus(file)
	}
	
	// Sort by priority
	sort.Slice(enhanced, func(i, j int) bool {
		// First by priority (lower number = higher priority)
		if enhanced[i].Priority != enhanced[j].Priority {
			return enhanced[i].Priority < enhanced[j].Priority
		}
		
		// Then by content type importance
		typeOrder := map[string]int{
			"code": 1, "config": 2, "frontend": 3, "docs": 4, "test": 5, "other": 6,
		}
		orderI := typeOrder[enhanced[i].ContentType]
		orderJ := typeOrder[enhanced[j].ContentType]
		if orderI != orderJ {
			return orderI < orderJ
		}
		
		// Finally by path for consistency
		return enhanced[i].Path < enhanced[j].Path
	})
	
	return enhanced
}

// analyzeAffectedAreas identifies the main areas of the codebase that were changed
func analyzeAffectedAreas(files []FileStatus) []string {
	areaMap := make(map[string]bool)
	
	for _, file := range files {
		// Extract directory path as affected area
		dir := filepath.Dir(file.Path)
		if dir != "." {
			// Use first directory level as area
			parts := strings.Split(dir, "/")
			if len(parts) > 0 {
				areaMap[parts[0]] = true
			}
		} else {
			areaMap["root"] = true
		}
	}
	
	var areas []string
	for area := range areaMap {
		areas = append(areas, area)
	}
	
	// Sort for consistent output
	sort.Strings(areas)
	
	return areas
}

// ============================================================================
// Performance and Utility Functions
// ============================================================================

// ExecuteBatchOperations executes multiple git operations concurrently
// This is a convenience method that wraps BatchGitOperations
func (c *Collector) ExecuteBatchOperations(ctx context.Context, operations ...func(context.Context) (interface{}, error)) ([]interface{}, []error) {
	batch := NewBatchGitOperations()
	for _, op := range operations {
		batch.AddOperation(op)
	}
	batch.ExecuteBatch(ctx)
	return batch.GetResults()
}

// GetCacheStats returns cache statistics for monitoring and debugging
func (c *Collector) GetCacheStats() map[string]interface{} {
	c.cache.mutex.RLock()
	defer c.cache.mutex.RUnlock()
	
	stats := map[string]interface{}{
		"cache_size": len(c.cache.cache),
		"cache_ttl":  c.cache.ttl.String(),
	}
	
	// Count expired entries
	expired := 0
	for _, entry := range c.cache.cache {
		if time.Since(entry.timestamp) > c.cache.ttl {
			expired++
		}
	}
	stats["expired_entries"] = expired
	
	return stats
}

// CleanExpiredCache removes expired entries from the cache
func (c *Collector) CleanExpiredCache() int {
	c.cache.mutex.Lock()
	defer c.cache.mutex.Unlock()
	
	cleaned := 0
	for key, entry := range c.cache.cache {
		if time.Since(entry.timestamp) > c.cache.ttl {
			delete(c.cache.cache, key)
			cleaned++
		}
	}
	
	return cleaned
}

// ============================================================================
// Compile-time Interface Verification
// ============================================================================

// Verify that Collector implements all the new focused interfaces
var (
	_ GitReader               = (*Collector)(nil)
	_ ChangeAnalyzer          = (*Collector)(nil)
	_ FileContentProvider     = (*Collector)(nil)
	_ EnhancedDiffProvider    = (*Collector)(nil)
)
