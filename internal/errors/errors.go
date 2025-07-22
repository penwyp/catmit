package errors

import (
	"errors"
	"fmt"
)

// ErrorType 定义错误类型
type ErrorType int

const (
	// ErrTypeUnknown 未知错误
	ErrTypeUnknown ErrorType = iota
	// ErrTypeGit Git 相关错误
	ErrTypeGit
	// ErrTypeProvider Provider 相关错误
	ErrTypeProvider
	// ErrTypePR PR 创建相关错误
	ErrTypePR
	// ErrTypeConfig 配置相关错误
	ErrTypeConfig
	// ErrTypeNetwork 网络相关错误
	ErrTypeNetwork
	// ErrTypeAuth 认证相关错误
	ErrTypeAuth
	// ErrTypeTimeout 超时错误
	ErrTypeTimeout
	// ErrTypeValidation 验证错误
	ErrTypeValidation
	// ErrTypeLLM LLM API 相关错误
	ErrTypeLLM
	// ErrTypeExternal 外部命令/工具相关错误
	ErrTypeExternal
)

// CatmitError 统一错误结构
type CatmitError struct {
	Type       ErrorType
	Message    string
	Cause      error
	Retryable  bool
	Suggestion string
}

// Error 实现 error 接口
func (e *CatmitError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap 支持 errors.Is 和 errors.As
func (e *CatmitError) Unwrap() error {
	return e.Cause
}

// WithSuggestion 添加解决建议
func (e *CatmitError) WithSuggestion(suggestion string) *CatmitError {
	e.Suggestion = suggestion
	return e
}

// IsRetryable 检查错误是否可重试
func (e *CatmitError) IsRetryable() bool {
	return e.Retryable
}

// New 创建新的 CatmitError
func New(errType ErrorType, message string) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
		Retryable: false,
	}
}

// Newf 创建格式化的新 CatmitError
func Newf(errType ErrorType, format string, args ...interface{}) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   fmt.Sprintf(format, args...),
		Retryable: false,
	}
}

// Wrap 包装已有错误
func Wrap(errType ErrorType, message string, cause error) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
		Cause:     cause,
		Retryable: false,
	}
}

// Wrapf 格式化包装已有错误
func Wrapf(errType ErrorType, format string, cause error, args ...interface{}) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   fmt.Sprintf(format, args...),
		Cause:     cause,
		Retryable: false,
	}
}

// NewRetryable 创建可重试错误
func NewRetryable(errType ErrorType, message string) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
		Retryable: true,
	}
}

// WrapRetryable 包装可重试错误
func WrapRetryable(errType ErrorType, message string, cause error) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
		Cause:     cause,
		Retryable: true,
	}
}

// 预定义的常见错误
var (
	// Git 相关错误
	ErrNoGitRepo       = New(ErrTypeGit, "current directory is not a Git repository").WithSuggestion("Please run this command in a Git repository")
	ErrNoStagedChanges = New(ErrTypeGit, "no staged changes").WithSuggestion("Use 'git add' to stage your changes")
	ErrNoBranch        = New(ErrTypeGit, "unable to get current branch").WithSuggestion("Make sure you are on a valid Git branch")
	ErrGitCommand      = New(ErrTypeGit, "Git command execution failed")

	// Provider 相关错误
	ErrProviderNotSupported = New(ErrTypeProvider, "unsupported Git provider").WithSuggestion("Currently supports GitHub, GitLab, Bitbucket and Gitea")
	ErrProviderDetection    = New(ErrTypeProvider, "unable to detect Git provider").WithSuggestion("Check if your remote repository URL is correct")
	ErrProviderConfig       = New(ErrTypeConfig, "provider configuration error").WithSuggestion("Check ~/.config/catmit/providers.yaml configuration file")

	// PR 相关错误
	ErrPRAlreadyExists = New(ErrTypePR, "pull request already exists").WithSuggestion("Visit the existing PR or use a different branch")
	ErrPRCreation      = New(ErrTypePR, "failed to create pull request")
	ErrCLINotInstalled = New(ErrTypePR, "required CLI tool is not installed").WithSuggestion("Please install the appropriate CLI tool (gh/glab/tea)")
	ErrCLINotAuthed    = New(ErrTypeAuth, "CLI tool is not authenticated").WithSuggestion("Run the appropriate auth command (gh auth login, etc.)")

	// 配置相关错误
	ErrConfigNotFound = New(ErrTypeConfig, "configuration file not found")
	ErrConfigParse    = New(ErrTypeConfig, "failed to parse configuration file").WithSuggestion("Check if the configuration file format is correct")
	ErrConfigWrite    = New(ErrTypeConfig, "failed to write configuration file")
	ErrInvalidConfig  = New(ErrTypeConfig, "invalid configuration").WithSuggestion("Refer to the configuration examples in the documentation")

	// 网络相关错误
	ErrNetworkTimeout = NewRetryable(ErrTypeTimeout, "network request timeout").WithSuggestion("Check your network connection and retry")
	ErrNetworkFailed  = NewRetryable(ErrTypeNetwork, "network request failed").WithSuggestion("Check your network connection or try again later")

	// LLM 相关错误
	ErrLLMAPIKey    = New(ErrTypeLLM, "API Key not set").WithSuggestion("Set the environment variable CATMIT_LLM_API_KEY")
	ErrLLMRateLimit = NewRetryable(ErrTypeLLM, "API rate limit exceeded").WithSuggestion("Try again later or upgrade your API plan")
	ErrLLMResponse  = New(ErrTypeLLM, "invalid LLM response format")
	ErrLLMTimeout   = NewRetryable(ErrTypeTimeout, "LLM request timeout").WithSuggestion("Increase timeout duration or try again later")

	// 验证错误
	ErrInvalidInput     = New(ErrTypeValidation, "invalid input parameters")
	ErrMissingParameter = New(ErrTypeValidation, "missing required parameter")
)

// Is 检查是否为特定错误
func Is(err error, target error) bool {
	return errors.Is(err, target)
}

// As 尝试转换为特定错误类型
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// GetType 获取错误类型
func GetType(err error) ErrorType {
	var catmitErr *CatmitError
	if errors.As(err, &catmitErr) {
		// 如果是超时类型，直接返回
		if catmitErr.Type == ErrTypeTimeout {
			return catmitErr.Type
		}
		// 递归检查 Cause 链
		if catmitErr.Cause != nil {
			causeType := GetType(catmitErr.Cause)
			// 如果 Cause 是超时类型，返回超时
			if causeType == ErrTypeTimeout {
				return ErrTypeTimeout
			}
		}
		return catmitErr.Type
	}
	return ErrTypeUnknown
}

// IsRetryable 检查错误是否可重试
func IsRetryable(err error) bool {
	var catmitErr *CatmitError
	if errors.As(err, &catmitErr) {
		return catmitErr.IsRetryable()
	}
	return false
}

// GetSuggestion 获取错误建议
func GetSuggestion(err error) string {
	var catmitErr *CatmitError
	if errors.As(err, &catmitErr) {
		return catmitErr.Suggestion
	}
	return ""
}

// FormatError 格式化错误输出
func FormatError(err error) string {
	var catmitErr *CatmitError
	if !errors.As(err, &catmitErr) {
		return err.Error()
	}

	msg := catmitErr.Error()
	if catmitErr.Suggestion != "" {
		msg += fmt.Sprintf("\n💡 %s", catmitErr.Suggestion)
	}

	return msg
}
