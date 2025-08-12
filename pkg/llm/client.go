package llm

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"
)

// LLMProvider defines the common interface for LLM service providers.
// Supports different LLM APIs (OpenAI-compatible and non-compatible).
type LLMProvider interface {
	GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	GetCompletionStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error)
}

// Client is responsible for interacting with the LLM API.
// This struct supports multiple LLM providers via injected LLMProvider.
// English log messages should be emitted by callers; only errors are returned here.
//
// Note: All public methods should accept context.Context to allow callers to control cancellation and timeout.
//
// Example:
//
//	c := llm.NewClient(logger)
//	msg, err := c.GetCommitMessage(ctx, "<system>", "<user>")
//
// The commit message is returned on success, errors are handled by the caller.
//
// In the future, go generate can be used to generate interface mocks for unit testing other modules.
// ------------------------------------------------------------------------------------
//
//go:generate mockgen -source=client.go -destination=../mocks/client_mock.go -package=mocks
type Client struct {
	provider LLMProvider // LLM service provider
	logger   *zap.Logger // Structured logger
}

// NewClient creates a new LLM Client.
// All timeout control is implemented via the passed context.Context to ensure immediate signal handling.
// Provider is automatically selected based on CATMIT_LLM_PROVIDER environment variable.
func NewClient(logger *zap.Logger) *Client {
	providerType := os.Getenv("CATMIT_LLM_PROVIDER")
	
	var provider LLMProvider
	switch providerType {
	case "azure":
		provider = NewAzureOpenAIProvider()
	case "cli":
		// CLI provider requires CATMIT_LLM_CLI_TOOL to be set
		// NewCLIProvider will panic if the tool is not found or not executable
		provider = NewCLIProvider(logger)
	default:
		// Default to OpenAI-compatible for backward compatibility
		provider = NewOpenAICompatibleProvider()
	}
	
	return &Client{
		provider: provider,
		logger:   logger,
	}
}


// GetCommitMessage calls the LLM API to generate a commit message.
// systemPrompt contains role definition, task description, and formatting rules.
// userPrompt contains context data (branch, files, commit history, diff).
// Returns the message string on success, or an error on failure.
func (c *Client) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Detailed debug logging
	if c.logger != nil {
		// Get provider details for logging
		if oaiProvider, ok := c.provider.(*OpenAICompatibleProvider); ok {
			c.logger.Debug("LLM API Request Details",
				zap.String("provider", "openai-compatible"),
				zap.String("api_url", oaiProvider.apiURL),
				zap.String("api_key_masked", maskAPIKey(oaiProvider.apiKey)),
				zap.String("model", oaiProvider.model),
				zap.String("system_prompt_preview", truncateForLog(systemPrompt, 100)),
				zap.String("user_prompt_preview", truncateForLog(userPrompt, 100)),
				zap.Int("system_prompt_length", len(systemPrompt)),
				zap.Int("user_prompt_length", len(userPrompt)))
		} else if azureProvider, ok := c.provider.(*AzureOpenAIProvider); ok {
			c.logger.Debug("LLM API Request Details",
				zap.String("provider", "azure"),
				zap.String("base_url", azureProvider.baseURL),
				zap.String("api_key_masked", maskAPIKey(azureProvider.apiKey)),
				zap.String("deployment", azureProvider.deployment),
				zap.String("api_version", azureProvider.apiVersion),
				zap.String("system_prompt_preview", truncateForLog(systemPrompt, 100)),
				zap.String("user_prompt_preview", truncateForLog(userPrompt, 100)),
				zap.Int("system_prompt_length", len(systemPrompt)),
				zap.Int("user_prompt_length", len(userPrompt)))
		} else {
			c.logger.Debug("LLM API Request",
				zap.String("system_prompt_preview", truncateForLog(systemPrompt, 100)),
				zap.String("user_prompt_preview", truncateForLog(userPrompt, 100)),
				zap.Int("system_prompt_length", len(systemPrompt)),
				zap.Int("user_prompt_length", len(userPrompt)))
		}
	}

	// Delegate actual call to Provider
	result, err := c.provider.GetCompletion(ctx, systemPrompt, userPrompt)

	if c.logger != nil {
		if err != nil {
			c.logger.Debug("LLM API Error",
				zap.Error(err),
				zap.String("error_type", fmt.Sprintf("%T", err)))
		} else {
			c.logger.Debug("LLM API Success",
				zap.Int("response_length", len(result)),
				zap.String("response_preview", truncateForLog(result, 100)))
		}
	}

	return result, err
}

// GetCommitMessageStream calls the LLM API to generate a commit message with streaming.
// Returns channels for content chunks and errors.
func (c *Client) GetCommitMessageStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
	// Detailed debug logging
	if c.logger != nil {
		// Get provider details for logging
		if oaiProvider, ok := c.provider.(*OpenAICompatibleProvider); ok {
			c.logger.Debug("LLM API Stream Request Details",
				zap.String("provider", "openai-compatible"),
				zap.String("api_url", oaiProvider.apiURL),
				zap.String("api_key_masked", maskAPIKey(oaiProvider.apiKey)),
				zap.String("model", oaiProvider.model),
				zap.String("system_prompt_preview", truncateForLog(systemPrompt, 100)),
				zap.String("user_prompt_preview", truncateForLog(userPrompt, 100)),
				zap.Int("system_prompt_length", len(systemPrompt)),
				zap.Int("user_prompt_length", len(userPrompt)))
		} else if azureProvider, ok := c.provider.(*AzureOpenAIProvider); ok {
			c.logger.Debug("LLM API Stream Request Details",
				zap.String("provider", "azure"),
				zap.String("base_url", azureProvider.baseURL),
				zap.String("api_key_masked", maskAPIKey(azureProvider.apiKey)),
				zap.String("deployment", azureProvider.deployment),
				zap.String("api_version", azureProvider.apiVersion),
				zap.String("system_prompt_preview", truncateForLog(systemPrompt, 100)),
				zap.String("user_prompt_preview", truncateForLog(userPrompt, 100)),
				zap.Int("system_prompt_length", len(systemPrompt)),
				zap.Int("user_prompt_length", len(userPrompt)))
		} else {
			c.logger.Debug("LLM API Stream Request",
				zap.String("system_prompt_preview", truncateForLog(systemPrompt, 100)),
				zap.String("user_prompt_preview", truncateForLog(userPrompt, 100)),
				zap.Int("system_prompt_length", len(systemPrompt)),
				zap.Int("user_prompt_length", len(userPrompt)))
		}
	}

	// Delegate actual call to Provider
	return c.provider.GetCompletionStream(ctx, systemPrompt, userPrompt)
}