package errors

import (
	"errors"
	"fmt"
)

// ErrorType defines the type of error
type ErrorType int

const (
	// ErrTypeUnknown unknown error
	ErrTypeUnknown ErrorType = iota
	// ErrTypeGit Git related error
	ErrTypeGit
	// ErrTypeProvider provider related error
	ErrTypeProvider
	// ErrTypePR pull request creation related error
	ErrTypePR
	// ErrTypeConfig configuration related error
	ErrTypeConfig
	// ErrTypeNetwork network related error
	ErrTypeNetwork
	// ErrTypeAuth authentication related error
	ErrTypeAuth
	// ErrTypeTimeout timeout error
	ErrTypeTimeout
	// ErrTypeValidation validation error
	ErrTypeValidation
	// ErrTypeLLM LLM API related error
	ErrTypeLLM
	// ErrTypeExternal external command/tool related error
	ErrTypeExternal
)

// CatmitError is the unified error structure
type CatmitError struct {
	Type       ErrorType
	Message    string
	Cause      error
	Retryable  bool
	Suggestion string
}

// Error implements the error interface
func (e *CatmitError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap supports errors.Is and errors.As
func (e *CatmitError) Unwrap() error {
	return e.Cause
}

// WithSuggestion adds a suggestion for resolving the error
func (e *CatmitError) WithSuggestion(suggestion string) *CatmitError {
	e.Suggestion = suggestion
	return e
}

// IsRetryable checks if the error is retryable
func (e *CatmitError) IsRetryable() bool {
	return e.Retryable
}

// New creates a new CatmitError
func New(errType ErrorType, message string) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
		Retryable: false,
	}
}

// Newf creates a new CatmitError with formatted message
func Newf(errType ErrorType, format string, args ...interface{}) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   fmt.Sprintf(format, args...),
		Retryable: false,
	}
}

// Wrap wraps an existing error
func Wrap(errType ErrorType, message string, cause error) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
		Cause:     cause,
		Retryable: false,
	}
}

// Wrapf wraps an existing error with formatted message
func Wrapf(errType ErrorType, format string, cause error, args ...interface{}) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   fmt.Sprintf(format, args...),
		Cause:     cause,
		Retryable: false,
	}
}

// NewRetryable creates a retryable error
func NewRetryable(errType ErrorType, message string) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
		Retryable: true,
	}
}

// WrapRetryable wraps a retryable error
func WrapRetryable(errType ErrorType, message string, cause error) *CatmitError {
	return &CatmitError{
		Type:      errType,
		Message:   message,
		Cause:     cause,
		Retryable: true,
	}
}

// Predefined common errors
var (
	// Git related errors
	ErrNoGitRepo       = New(ErrTypeGit, "current directory is not a Git repository").WithSuggestion("Please run this command in a Git repository")
	ErrNoStagedChanges = New(ErrTypeGit, "no staged changes")
	ErrNoBranch        = New(ErrTypeGit, "unable to get current branch").WithSuggestion("Make sure you are on a valid Git branch")
	ErrGitCommand      = New(ErrTypeGit, "Git command execution failed")

	// Provider related errors
	ErrProviderNotSupported = New(ErrTypeProvider, "unsupported Git provider").WithSuggestion("Currently supports GitHub, GitLab, Bitbucket and Gitea")
	ErrProviderDetection    = New(ErrTypeProvider, "unable to detect Git provider").WithSuggestion("Check if your remote repository URL is correct")
	ErrProviderConfig       = New(ErrTypeConfig, "provider configuration error").WithSuggestion("Check ~/.config/catmit/providers.yaml configuration file")

	// PR related errors
	ErrPRAlreadyExists = New(ErrTypePR, "pull request already exists").WithSuggestion("Visit the existing PR or use a different branch")
	ErrPRCreation      = New(ErrTypePR, "failed to create pull request")
	ErrCLINotInstalled = New(ErrTypePR, "required CLI tool is not installed").WithSuggestion("Please install the appropriate CLI tool (gh/glab/tea)")
	ErrCLINotAuthed    = New(ErrTypeAuth, "CLI tool is not authenticated").WithSuggestion("Run the appropriate auth command (gh auth login, etc.)")

	// Configuration related errors
	ErrConfigNotFound = New(ErrTypeConfig, "configuration file not found")
	ErrConfigParse    = New(ErrTypeConfig, "failed to parse configuration file").WithSuggestion("Check if the configuration file format is correct")
	ErrConfigWrite    = New(ErrTypeConfig, "failed to write configuration file")
	ErrInvalidConfig  = New(ErrTypeConfig, "invalid configuration").WithSuggestion("Refer to the configuration examples in the documentation")

	// Network related errors
	ErrNetworkTimeout = NewRetryable(ErrTypeTimeout, "network request timeout").WithSuggestion("Check your network connection and retry")
	ErrNetworkFailed  = NewRetryable(ErrTypeNetwork, "network request failed").WithSuggestion("Check your network connection or try again later")

	// LLM related errors
	ErrLLMAPIKey    = New(ErrTypeLLM, "API Key not set").WithSuggestion("Set the environment variable CATMIT_LLM_API_KEY")
	ErrLLMRateLimit = NewRetryable(ErrTypeLLM, "API rate limit exceeded").WithSuggestion("Try again later or upgrade your API plan")
	ErrLLMResponse  = New(ErrTypeLLM, "invalid LLM response format")
	ErrLLMTimeout   = NewRetryable(ErrTypeTimeout, "LLM request timeout").WithSuggestion("Increase timeout duration or try again later")

	// Validation errors
	ErrInvalidInput     = New(ErrTypeValidation, "invalid input parameters")
	ErrMissingParameter = New(ErrTypeValidation, "missing required parameter")
)

// Is checks if the error is a specific error
func Is(err error, target error) bool {
	return errors.Is(err, target)
}

// As tries to convert to a specific error type
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

// GetType gets the error type
func GetType(err error) ErrorType {
	var catmitErr *CatmitError
	if errors.As(err, &catmitErr) {
		// If it is a timeout type, return directly
		if catmitErr.Type == ErrTypeTimeout {
			return catmitErr.Type
		}
		// Recursively check the Cause chain
		if catmitErr.Cause != nil {
			causeType := GetType(catmitErr.Cause)
			// If the Cause is a timeout type, return timeout
			if causeType == ErrTypeTimeout {
				return ErrTypeTimeout
			}
		}
		return catmitErr.Type
	}
	return ErrTypeUnknown
}

// IsRetryable checks if the error is retryable
func IsRetryable(err error) bool {
	var catmitErr *CatmitError
	if errors.As(err, &catmitErr) {
		return catmitErr.IsRetryable()
	}
	return false
}


// FormatError formats the error output
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
