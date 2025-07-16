package errors

// Exit codes for different error types
const (
	ExitCodeSuccess              = 0
	ExitCodeGenericError         = 1
	ExitCodeCLINotInstalled      = 2
	ExitCodeCLINotAuthenticated  = 3
	ExitCodePRAlreadyExists      = 4
	ExitCodeNetworkError         = 5
	ExitCodePermissionDenied     = 6
	ExitCodeUnsupportedProvider  = 7
	ExitCodeGitError             = 8
	ExitCodeTimeout              = 124 // Standard timeout exit code
)

// PRError 包含PR操作的错误信息
type PRError struct {
	Message    string // 用户友好的错误消息
	Details    string // 详细的错误信息（可选）
	Suggestion string // 建议的解决方案
	ExitCode   int    // 退出码
	IsRetryable bool  // 是否可以重试
}