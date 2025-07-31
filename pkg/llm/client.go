package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
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
//go:generate mockgen -source=llm.go -destination=../mocks/client_mock.go -package=mocks
type Client struct {
	provider LLMProvider // LLM service provider
	logger   *zap.Logger // Structured logger
}

// OpenAICompatibleProvider implements OpenAI-compatible LLM API calls.
type OpenAICompatibleProvider struct {
	apiURL     string       // Full API endpoint URL
	apiKey     string       // API Key for authentication
	model      string       // Model name
	httpClient *http.Client // Customizable http.Client for timeout and testing
}

// NewClient creates a new LLM Client.
// All timeout control is implemented via the passed context.Context to ensure immediate signal handling.
func NewClient(logger *zap.Logger) *Client {
	provider := NewOpenAICompatibleProvider()
	return &Client{
		provider: provider,
		logger:   logger,
	}
}

// NewClientWithProvider creates a Client using the specified Provider.
func NewClientWithProvider(provider LLMProvider, logger *zap.Logger) *Client {
	return &Client{
		provider: provider,
		logger:   logger,
	}
}

// NewOpenAICompatibleProvider creates an OpenAI-compatible Provider.
// Reads configuration from environment variables, supports DeepSeek, Volcengine, and other OpenAI-compatible APIs.
func NewOpenAICompatibleProvider() *OpenAICompatibleProvider {
	apiURL := os.Getenv("CATMIT_LLM_API_URL")
	if apiURL == "" {
		apiURL = "https://api.deepseek.com/v1/chat/completions"
	}

	apiKey := os.Getenv("CATMIT_LLM_API_KEY")

	model := os.Getenv("CATMIT_LLM_MODEL")
	if model == "" {
		model = "deepseek-chat"
	}

	return &OpenAICompatibleProvider{
		apiURL: apiURL,
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			// No Timeout set, fully relying on context for timeout and cancellation
		},
	}
}

// chatRequest defines the request body structure for DeepSeek Chat API.
// Private struct used only for serialization.
type chatRequest struct {
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	MaxTokens           int           `json:"max_tokens,omitempty"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
	Temperature         float64       `json:"temperature"`
	Stream              bool          `json:"stream,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse corresponds to the full response format of DeepSeek Chat API.
// Includes all fields returned by the API to ensure compatibility with the actual response structure.
type chatResponse struct {
	ID                string `json:"id"`
	Object            string `json:"object"`
	Created           int64  `json:"created"`
	Model             string `json:"model"`
	SystemFingerprint string `json:"system_fingerprint"`
	Choices           []struct {
		Index        int         `json:"index"`
		Message      chatMessage `json:"message"`
		LogProbs     interface{} `json:"logprobs"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// streamResponse represents a streaming response chunk from the API
type streamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int       `json:"index"`
		Delta        deltaMessage `json:"delta"`
		FinishReason *string   `json:"finish_reason"`
	} `json:"choices"`
}

type deltaMessage struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// maskAPIKey masks the API key for logging purposes
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "***" + apiKey[len(apiKey)-4:]
}

// truncateForLog truncates content for logging with UTF-8 awareness
func truncateForLog(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	// Ensure we don't cut in the middle of a UTF-8 character
	truncated := content[:maxLen]
	for len(truncated) > 0 && !isValidUTF8Start(truncated[len(truncated)-1]) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + "..."
}

// isValidUTF8Start checks if a byte can be the start of a UTF-8 character
func isValidUTF8Start(b byte) bool {
	return (b&0x80) == 0 || (b&0xC0) == 0xC0
}

// GetCompletion implements the OpenAI-compatible API call.
func (p *OpenAICompatibleProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Check if API Key is set
	if p.apiKey == "" {
		return "", errors.ErrLLMAPIKey
	}

	// Build request body, separating system and user messages
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	// Different APIs may have different requirements for max_tokens
	// Volcengine Ark doesn't allow both max_tokens and max_completion_tokens
	reqBody := chatRequest{
		Model:               p.model,
		Messages:            messages,
		MaxTokens:           128,
		Temperature:         0.7,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to marshal request", err)
	}

	// Build HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(data))
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to create request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	// Send request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		// If context is cancelled or deadline exceeded, return appropriate error type
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.ErrLLMTimeout
		}
		if strings.Contains(err.Error(), "timeout") {
			return "", errors.ErrLLMTimeout
		}
		// Other network errors
		return "", errors.WrapRetryable(errors.ErrTypeLLM, "network request failed", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body for error handling and parsing.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to read response", err)
	}

	// Non-200 responses are handled as errors, including status code but not response body to avoid leaking sensitive info.
	if resp.StatusCode != http.StatusOK {
		// Log response body for debugging (only in debug mode)
		errorBody := string(bodyBytes)
		if len(errorBody) > 500 {
			errorBody = errorBody[:500] + "..."
		}
		
		// Handle specific status codes
		switch resp.StatusCode {
		case http.StatusBadRequest:
			// Return a new error with the response body for debugging
			return "", errors.New(errors.ErrTypeValidation, fmt.Sprintf("invalid input parameters: %s", errorBody))
		case http.StatusUnauthorized:
			return "", errors.ErrLLMAPIKey
		case http.StatusForbidden:
			return "", errors.ErrLLMAPIKey
		case http.StatusTooManyRequests:
			return "", errors.ErrLLMRateLimit
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
			return "", errors.NewRetryable(errors.ErrTypeLLM, fmt.Sprintf("API server error: status %d", resp.StatusCode))
		default:
			return "", errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d", resp.StatusCode))
		}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to parse response", err).WithSuggestion("API response format may have changed, please check API documentation")
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.ErrLLMResponse
	}

	// Validate response content integrity
	if chatResp.Choices[0].Message.Content == "" {
		return "", errors.New(errors.ErrTypeLLM, "invalid response: empty message content")
	}

	return chatResp.Choices[0].Message.Content, nil
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
				zap.String("api_url", oaiProvider.apiURL),
				zap.String("api_key_masked", maskAPIKey(oaiProvider.apiKey)),
				zap.String("model", oaiProvider.model),
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
				zap.String("api_url", oaiProvider.apiURL),
				zap.String("api_key_masked", maskAPIKey(oaiProvider.apiKey)),
				zap.String("model", oaiProvider.model),
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

// GetCompletionStream implements the OpenAI-compatible streaming API call.
func (p *OpenAICompatibleProvider) GetCompletionStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
	contentChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errChan)

		// Check if API Key is set
		if p.apiKey == "" {
			errChan <- errors.ErrLLMAPIKey
			return
		}

		// Build request body, separating system and user messages
		messages := []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}

		// Different APIs may have different requirements for max_tokens
		// Volcengine Ark doesn't allow both max_tokens and max_completion_tokens
		reqBody := chatRequest{
			Model:               p.model,
			Messages:            messages,
			MaxTokens:           128,
			Temperature:         0.7,
			Stream:              true,
		}

		data, err := json.Marshal(reqBody)
		if err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to marshal request", err)
			return
		}

		// Build HTTP request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(data))
		if err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to create request", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		if p.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		}

		// Send request
		resp, err := p.httpClient.Do(req)
		if err != nil {
			// If context is cancelled or deadline exceeded, return appropriate error type
			if ctx.Err() == context.DeadlineExceeded {
				errChan <- errors.ErrLLMTimeout
				return
			}
			if strings.Contains(err.Error(), "timeout") {
				errChan <- errors.ErrLLMTimeout
				return
			}
			// Other network errors
			errChan <- errors.WrapRetryable(errors.ErrTypeLLM, "network request failed", err)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		// Non-200 responses are handled as errors
		if resp.StatusCode != http.StatusOK {
			// Read error body for debugging
			bodyBytes, _ := io.ReadAll(resp.Body)
			errorBody := string(bodyBytes)
			if len(errorBody) > 500 {
				errorBody = errorBody[:500] + "..."
			}
			
			// Handle specific status codes
			switch resp.StatusCode {
			case http.StatusBadRequest:
				// Return a new error with the response body for debugging
				errChan <- errors.New(errors.ErrTypeValidation, fmt.Sprintf("invalid input parameters: %s", errorBody))
			case http.StatusUnauthorized:
				errChan <- errors.ErrLLMAPIKey
			case http.StatusForbidden:
				errChan <- errors.ErrLLMAPIKey
			case http.StatusTooManyRequests:
				errChan <- errors.ErrLLMRateLimit
			case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
				errChan <- errors.NewRetryable(errors.ErrTypeLLM, fmt.Sprintf("API server error: status %d", resp.StatusCode))
			default:
				errChan <- errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d", resp.StatusCode))
			}
			return
		}

		// Read SSE stream
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to read stream", err)
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Parse SSE format
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				
				// Check for stream end
				if data == "[DONE]" {
					break
				}

				// Parse JSON
				var chunk streamResponse
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					// Skip invalid chunks
					continue
				}

				// Extract content from chunk
				if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
					select {
					case contentChan <- chunk.Choices[0].Delta.Content:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return contentChan, errChan
}
