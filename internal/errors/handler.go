package errors

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

// ErrorHandler 错误处理器
type ErrorHandler struct {
	// 可以添加配置选项
}

// NewErrorHandler 创建新的错误处理器
func NewErrorHandler() *ErrorHandler {
	return &ErrorHandler{}
}

// HandlePRError 处理PR相关错误，返回结构化的错误信息
func (h *ErrorHandler) HandlePRError(err error) PRError {
	if err == nil {
		return PRError{ExitCode: ExitCodeSuccess}
	}

	errStr := err.Error()

	// CLI未安装
	if strings.Contains(errStr, "gh is not installed") {
		return PRError{
			Message:    "GitHub CLI (gh) is not installed",
			Suggestion: "Install with:\n  brew install gh\n  https://github.com/cli/cli#installation",
			ExitCode:   ExitCodeCLINotInstalled,
		}
	}
	if strings.Contains(errStr, "tea is not installed") {
		return PRError{
			Message:    "Gitea CLI (tea) is not installed",
			Suggestion: "Install with:\n  go install gitea.com/gitea/tea@latest\n  https://gitea.com/gitea/tea",
			ExitCode:   ExitCodeCLINotInstalled,
		}
	}

	// CLI未认证
	if strings.Contains(errStr, "gh is not authenticated") {
		return PRError{
			Message:    "GitHub CLI (gh) is not authenticated",
			Suggestion: "Run: gh auth login",
			ExitCode:   ExitCodeCLINotAuthenticated,
		}
	}
	if strings.Contains(errStr, "tea is not authenticated") {
		return PRError{
			Message:    "Gitea CLI (tea) is not authenticated",
			Suggestion: "Run: tea login add",
			ExitCode:   ExitCodeCLINotAuthenticated,
		}
	}

	// PR已存在
	if strings.Contains(errStr, "already exists") {
		return PRError{
			Message:    "A pull request already exists for this branch",
			Suggestion: "View existing PRs with:\n  gh pr list (GitHub)\n  tea pr list (Gitea)",
			ExitCode:   ExitCodePRAlreadyExists,
		}
	}

	// 网络错误
	if strings.Contains(errStr, "no such host") || 
	   strings.Contains(errStr, "connection refused") ||
	   strings.Contains(errStr, "timeout") ||
	   strings.Contains(errStr, "dial tcp") {
		return PRError{
			Message:     "Network error occurred",
			Details:     errStr,
			Suggestion:  "Check your internet connection and try again",
			ExitCode:    ExitCodeNetworkError,
			IsRetryable: true,
		}
	}

	// 权限错误
	if strings.Contains(errStr, "403") || strings.Contains(errStr, "Permission denied") {
		return PRError{
			Message:    "Permission denied",
			Details:    errStr,
			Suggestion: "Check repository permissions and authentication status",
			ExitCode:   ExitCodePermissionDenied,
		}
	}

	// 不支持的Provider
	if strings.Contains(errStr, "unsupported provider") {
		provider := extractProvider(errStr)
		// 特殊处理一些provider的大小写
		providerTitle := provider
		switch strings.ToLower(provider) {
		case "github":
			providerTitle = "GitHub"
		case "gitlab":
			providerTitle = "GitLab"
		case "gitea":
			providerTitle = "Gitea"
		default:
			providerTitle = strings.Title(provider)
		}
		return PRError{
			Message:    fmt.Sprintf("%s is not supported yet", providerTitle),
			Suggestion: "Supported providers: GitHub, Gitea",
			ExitCode:   ExitCodeUnsupportedProvider,
		}
	}

	// Git错误
	if strings.Contains(errStr, "remote") && strings.Contains(errStr, "not found") {
		remoteName := extractRemoteName(errStr)
		return PRError{
			Message:    fmt.Sprintf("Git remote '%s' not found", remoteName),
			Suggestion: fmt.Sprintf("Run: git remote add %s <url>", remoteName),
			ExitCode:   ExitCodeGitError,
		}
	}

	// 默认错误
	return PRError{
		Message:  fmt.Sprintf("Error: %s", errStr),
		ExitCode: ExitCodeGenericError,
	}
}

// FormatError 格式化错误信息为用户友好的输出
func (h *ErrorHandler) FormatError(prError PRError) string {
	var sb strings.Builder

	// 错误消息（红色）
	sb.WriteString(color.RedString("Error: %s\n", prError.Message))

	// 详细信息（如果有）
	if prError.Details != "" {
		sb.WriteString(color.YellowString("Details: %s\n", prError.Details))
	}

	// 建议（如果有）
	if prError.Suggestion != "" {
		sb.WriteString("\n")
		sb.WriteString(prError.Suggestion)
		sb.WriteString("\n")
	}

	return sb.String()
}

// IsRetryableError 判断错误是否可以重试
func (h *ErrorHandler) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	
	// 网络相关错误可以重试
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"no such host",
		"temporary failure",
		"EOF",
		"connection reset",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	// 认证和权限错误不应重试
	nonRetryablePatterns := []string{
		"401",
		"403",
		"Unauthorized",
		"Forbidden",
		"already exists",
		"not found",
		"permission denied",
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	return false
}

// WrapError 包装错误，添加上下文信息
func (h *ErrorHandler) WrapError(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// 辅助函数

func extractProvider(errStr string) string {
	// 从 "unsupported provider: gitlab" 中提取 "gitlab"
	parts := strings.Split(errStr, ":")
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[1])
	}
	return "unknown"
}

func extractRemoteName(errStr string) string {
	// 从 "remote 'origin' not found" 中提取 "origin"
	start := strings.Index(errStr, "'")
	end := strings.LastIndex(errStr, "'")
	if start != -1 && end != -1 && start < end {
		return errStr[start+1 : end]
	}
	return "origin"
}