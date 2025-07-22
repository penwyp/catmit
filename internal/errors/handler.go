package errors

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Handler is the error handler interface
type Handler interface {
	Handle(err error) error
	HandleWithRetry(ctx context.Context, err error, operation func() error) error
}

// DefaultHandler is the default error handler implementation
type DefaultHandler struct {
	MaxRetries    int
	RetryInterval time.Duration
	Verbose       bool
}

// NewHandler creates a new error handler
func NewHandler(verbose bool) Handler {
	return &DefaultHandler{
		MaxRetries:    3,
		RetryInterval: time.Second,
		Verbose:       verbose,
	}
}

// Handle processes the error
func (h *DefaultHandler) Handle(err error) error {
	if err == nil {
		return nil
	}

	// Convert to CatmitError for more information
	var catmitErr *CatmitError
	if !As(err, &catmitErr) {
		// If not a CatmitError, try to infer type from error content
		catmitErr = h.inferErrorType(err)
	}

	// Format and output the error
	h.printError(catmitErr)

	return catmitErr
}

// HandleWithRetry processes the error and supports retry
func (h *DefaultHandler) HandleWithRetry(ctx context.Context, err error, operation func() error) error {
	if err == nil || operation == nil {
		return h.Handle(err)
	}

	// Check if error is retryable
	if !IsRetryable(err) {
		return h.Handle(err)
	}

	// Execute retry logic
	var lastErr error
	for i := 0; i < h.MaxRetries; i++ {
		if i > 0 {
			// Wait before retrying
			select {
			case <-ctx.Done():
				return h.Handle(ctx.Err())
			case <-time.After(h.RetryInterval * time.Duration(i)):
				// Exponential backoff
			}

			if h.Verbose {
				fmt.Printf("🔄 Retrying %d/%d...\n", i+1, h.MaxRetries)
			}
		}

		lastErr = operation()
		if lastErr == nil {
			return nil
		}

		// If the new error is not retryable, return immediately
		if !IsRetryable(lastErr) {
			return h.Handle(lastErr)
		}
	}

	// All retries failed
	return h.Handle(WrapRetryable(ErrTypeNetwork, fmt.Sprintf("Operation failed after %d retries", h.MaxRetries), lastErr))
}

// inferErrorType infers the error type based on error content
func (h *DefaultHandler) inferErrorType(err error) *CatmitError {
	errMsg := strings.ToLower(err.Error())

	// Git related errors
	if strings.Contains(errMsg, "git") || strings.Contains(errMsg, "repository") || strings.Contains(errMsg, "nothing to commit") {
		if strings.Contains(errMsg, "not a git repository") {
			return Wrap(ErrTypeGit, "Not a Git repository", err).WithSuggestion("Please run this command inside a Git repository")
		}
		if strings.Contains(errMsg, "no changes") || strings.Contains(errMsg, "nothing to commit") {
			return Wrap(ErrTypeGit, "No changes to commit", err).WithSuggestion("Make some changes before committing")
		}
		return Wrap(ErrTypeGit, "Git operation failed", err)
	}

	// Network related errors
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") {
		return WrapRetryable(ErrTypeTimeout, "Operation timed out", err).WithSuggestion("Check your network connection or increase the timeout")
	}
	if strings.Contains(errMsg, "connection") || strings.Contains(errMsg, "network") {
		return WrapRetryable(ErrTypeNetwork, "Network error", err).WithSuggestion("Check your network connection and retry")
	}

	// Authentication related errors
	if strings.Contains(errMsg, "auth") || strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "forbidden") {
		return Wrap(ErrTypeAuth, "Authentication failed", err).WithSuggestion("Check your credentials or log in again")
	}

	// API related errors
	if strings.Contains(errMsg, "api") || strings.Contains(errMsg, "rate limit") {
		if strings.Contains(errMsg, "rate limit") {
			return WrapRetryable(ErrTypeLLM, "API rate limit", err).WithSuggestion("Retry later or upgrade your API plan")
		}
		return Wrap(ErrTypeLLM, "API error", err)
	}

	// Default error
	return Wrap(ErrTypeUnknown, err.Error(), err)
}

// printError prints the formatted error message
func (h *DefaultHandler) printError(err *CatmitError) {
	// Define styles
	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	suggestionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11"))

	// Build error message
	var parts []string

	// Error icon and main message
	icon := h.getErrorIcon(err.Type)
	parts = append(parts, fmt.Sprintf("%s %s", icon, errorStyle.Render(err.Error())))

	// Suggestion
	if err.Suggestion != "" {
		parts = append(parts, suggestionStyle.Render(fmt.Sprintf("💡 %s", err.Suggestion)))
	}

	// Details (only in verbose mode)
	if h.Verbose && err.Cause != nil {
		parts = append(parts, fmt.Sprintf("   Cause: %v", err.Cause))
		if err.Retryable {
			parts = append(parts, "   ℹ️  This error is retryable")
		}
	}

	// Output to stderr
	fmt.Fprintln(os.Stderr, strings.Join(parts, "\n"))
}

// getErrorIcon returns the icon based on error type
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

// HandleFatal handles fatal errors and exits
func HandleFatal(err error) {
	if err == nil {
		return
	}

	handler := NewHandler(false)
	_ = handler.Handle(err)

	// Determine exit code based on error type
	exitCode := 1
	if IsRetryable(err) {
		exitCode = 124 // Timeout exit code
	}

	os.Exit(exitCode)
}
