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

// Wrap 包装已有错误
func Wrap(errType ErrorType, message string, cause error) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
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
	ErrNoGitRepo       = New(ErrTypeGit, "当前目录不是 Git 仓库").WithSuggestion("请在 Git 仓库中运行此命令")
	ErrNoStagedChanges = New(ErrTypeGit, "没有暂存的更改").WithSuggestion("使用 'git add' 暂存您的更改")
	ErrNoBranch        = New(ErrTypeGit, "无法获取当前分支").WithSuggestion("确保您在有效的 Git 分支上")
	ErrGitCommand      = New(ErrTypeGit, "Git 命令执行失败")
	
	// Provider 相关错误
	ErrProviderNotSupported = New(ErrTypeProvider, "不支持的 Git 提供商").WithSuggestion("当前支持 GitHub、GitLab、Bitbucket 和 Gitea")
	ErrProviderDetection    = New(ErrTypeProvider, "无法检测 Git 提供商").WithSuggestion("检查您的远程仓库 URL 是否正确")
	ErrProviderConfig       = New(ErrTypeConfig, "Provider 配置错误").WithSuggestion("检查 ~/.config/catmit/providers.yaml 配置文件")
	
	// PR 相关错误
	ErrPRAlreadyExists = New(ErrTypePR, "PR 已存在").WithSuggestion("访问现有 PR 或使用不同的分支")
	ErrPRCreation      = New(ErrTypePR, "创建 PR 失败")
	ErrCLINotInstalled = New(ErrTypePR, "所需的 CLI 工具未安装").WithSuggestion("请安装相应的 CLI 工具（gh/glab/tea 等）")
	ErrCLINotAuthed    = New(ErrTypeAuth, "CLI 工具未认证").WithSuggestion("运行相应的认证命令（gh auth login 等）")
	
	// 配置相关错误
	ErrConfigNotFound   = New(ErrTypeConfig, "配置文件不存在")
	ErrConfigParse      = New(ErrTypeConfig, "配置文件解析失败").WithSuggestion("检查配置文件格式是否正确")
	ErrConfigWrite      = New(ErrTypeConfig, "配置文件写入失败")
	ErrInvalidConfig    = New(ErrTypeConfig, "配置无效").WithSuggestion("参考文档中的配置示例")
	
	// 网络相关错误
	ErrNetworkTimeout = NewRetryable(ErrTypeTimeout, "网络请求超时").WithSuggestion("检查网络连接并重试")
	ErrNetworkFailed  = NewRetryable(ErrTypeNetwork, "网络请求失败").WithSuggestion("检查网络连接或稍后重试")
	
	// LLM 相关错误
	ErrLLMAPIKey      = New(ErrTypeLLM, "未设置 API Key").WithSuggestion("设置环境变量 CATMIT_LLM_API_KEY")
	ErrLLMRateLimit   = NewRetryable(ErrTypeLLM, "API 速率限制").WithSuggestion("稍后重试或升级您的 API 套餐")
	ErrLLMResponse    = New(ErrTypeLLM, "LLM 响应格式错误")
	ErrLLMTimeout     = NewRetryable(ErrTypeTimeout, "LLM 请求超时").WithSuggestion("增加超时时间或稍后重试")
	
	// 验证错误
	ErrInvalidInput     = New(ErrTypeValidation, "输入参数无效")
	ErrMissingParameter = New(ErrTypeValidation, "缺少必需参数")
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