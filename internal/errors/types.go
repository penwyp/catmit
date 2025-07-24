package errors

// Exit codes for different error types
const (
	ExitCodeSuccess             = 0
	ExitCodeGenericError        = 1
	ExitCodeCLINotInstalled     = 2
	ExitCodeCLINotAuthenticated = 3
	ExitCodePRAlreadyExists     = 4
	ExitCodeNetworkError        = 5
	ExitCodePermissionDenied    = 6
	ExitCodeUnsupportedProvider = 7
	ExitCodeGitError            = 8
	ExitCodeTimeout             = 124 // Standard timeout exit code
)

// PRError contains PR operation error information
type PRError struct {
	Message     string // User-friendly error message
	Details     string // Detailed error information (optional)
	Suggestion  string // Suggested solution
	ExitCode    int    // Exit code
	IsRetryable bool   // Whether it can be retried
}
