package errors

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Handler 错误处理器接口
type Handler interface {
	Handle(err error) error
	HandleWithRetry(ctx context.Context, err error, operation func() error) error
}

// DefaultHandler 默认错误处理器
type DefaultHandler struct {
	MaxRetries    int
	RetryInterval time.Duration
	Verbose       bool
}

// NewHandler 创建新的错误处理器
func NewHandler(verbose bool) Handler {
	return &DefaultHandler{
		MaxRetries:    3,
		RetryInterval: time.Second,
		Verbose:       verbose,
	}
}

// Handle 处理错误
func (h *DefaultHandler) Handle(err error) error {
	if err == nil {
		return nil
	}

	// 转换为 CatmitError 以获取更多信息
	var catmitErr *CatmitError
	if !As(err, &catmitErr) {
		// 如果不是 CatmitError，尝试根据错误内容推断类型
		catmitErr = h.inferErrorType(err)
	}

	// 格式化并输出错误
	h.printError(catmitErr)

	return catmitErr
}

// HandleWithRetry 处理错误并支持重试
func (h *DefaultHandler) HandleWithRetry(ctx context.Context, err error, operation func() error) error {
	if err == nil || operation == nil {
		return h.Handle(err)
	}

	// 检查是否可重试
	if !IsRetryable(err) {
		return h.Handle(err)
	}

	// 执行重试逻辑
	var lastErr error
	for i := 0; i < h.MaxRetries; i++ {
		if i > 0 {
			// 等待后重试
			select {
			case <-ctx.Done():
				return h.Handle(ctx.Err())
			case <-time.After(h.RetryInterval * time.Duration(i)):
				// 指数退避
			}

			if h.Verbose {
				fmt.Printf("🔄 重试 %d/%d...\n", i+1, h.MaxRetries)
			}
		}

		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		// 如果新错误不可重试，立即返回
		if !IsRetryable(lastErr) {
			return h.Handle(lastErr)
		}
	}

	// 所有重试都失败
	return h.Handle(WrapRetryable(ErrTypeNetwork, fmt.Sprintf("操作在 %d 次重试后失败", h.MaxRetries), lastErr))
}

// inferErrorType 根据错误内容推断错误类型
func (h *DefaultHandler) inferErrorType(err error) *CatmitError {
	errMsg := strings.ToLower(err.Error())

	// Git 相关错误
	if strings.Contains(errMsg, "git") || strings.Contains(errMsg, "repository") || strings.Contains(errMsg, "nothing to commit") {
		if strings.Contains(errMsg, "not a git repository") {
			return Wrap(ErrTypeGit, "不是 Git 仓库", err).WithSuggestion("请在 Git 仓库中运行此命令")
		}
		if strings.Contains(errMsg, "no changes") || strings.Contains(errMsg, "nothing to commit") {
			return Wrap(ErrTypeGit, "没有需要提交的更改", err).WithSuggestion("先进行一些更改再提交")
		}
		return Wrap(ErrTypeGit, "Git 操作失败", err)
	}

	// 网络相关错误
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
		return WrapRetryable(ErrTypeTimeout, "操作超时", err).WithSuggestion("检查网络连接或增加超时时间")
	}
	if strings.Contains(errMsg, "connection") || strings.Contains(errMsg, "network") {
		return WrapRetryable(ErrTypeNetwork, "网络错误", err).WithSuggestion("检查网络连接并重试")
	}

	// 认证相关错误
	if strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "forbidden") {
		return Wrap(ErrTypeAuth, "认证失败", err).WithSuggestion("检查您的凭据或重新登录")
	}

	// API 相关错误
	if strings.Contains(errMsg, "api") || strings.Contains(errMsg, "rate limit") {
		if strings.Contains(errMsg, "rate limit") {
			return WrapRetryable(ErrTypeLLM, "API 速率限制", err).WithSuggestion("稍后重试或升级您的 API 套餐")
		}
		return Wrap(ErrTypeLLM, "API 错误", err)
	}

	// 默认错误
	return Wrap(ErrTypeUnknown, err.Error(), err)
}

// printError 打印格式化的错误信息
func (h *DefaultHandler) printError(err *CatmitError) {
	// 定义样式
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	suggestionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11"))

	// 构建错误消息
	var parts []string

	// 错误图标和主消息
	icon := h.getErrorIcon(err.Type)
	parts = append(parts, fmt.Sprintf("%s %s", icon, errorStyle.Render(err.Error())))

	// 建议
	if err.Suggestion != "" {
		parts = append(parts, suggestionStyle.Render(fmt.Sprintf("💡 %s", err.Suggestion)))
	}

	// 详细信息（仅在 verbose 模式下）
	if h.Verbose && err.Cause != nil {
		parts = append(parts, fmt.Sprintf("   原因: %v", err.Cause))
		if err.Retryable {
			parts = append(parts, "   ℹ️  此错误可重试")
		}
	}

	// 输出到 stderr
	fmt.Fprintln(os.Stderr, strings.Join(parts, "\n"))
}

// getErrorIcon 根据错误类型返回图标
func (h *DefaultHandler) getErrorIcon(errType ErrorType) string {
	switch errType {
	case ErrTypeGit:
		return "🔧"
	case ErrTypeProvider:
		return "🔗"
	case ErrTypePR:
		return "📝"
	case ErrTypeConfig:
		return "⚙️"
	case ErrTypeNetwork:
		return "🌐"
	case ErrTypeAuth:
		return "🔐"
	case ErrTypeTimeout:
		return "⏱️"
	case ErrTypeValidation:
		return "✅"
	case ErrTypeLLM:
		return "🤖"
	default:
		return "❌"
	}
}

// HandleFatal 处理致命错误并退出
func HandleFatal(err error) {
	if err == nil {
		return
	}

	handler := NewHandler(false)
	_ = handler.Handle(err)

	// 根据错误类型确定退出码
	exitCode := 1
	if IsRetryable(err) {
		exitCode = 124 // 超时退出码
	}

	os.Exit(exitCode)
}
