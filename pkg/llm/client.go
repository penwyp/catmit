package llm

import (
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
		apiURL:     apiURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{
			// No Timeout set, fully relying on context for timeout and cancellation
		},
	}
}

// chatRequest defines the request body structure for DeepSeek Chat API.
// Private struct used only for serialization.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
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

	reqBody := chatRequest{
		Model:       p.model,
		Messages:    messages,
		MaxTokens:   128,
		Temperature: 0.7,
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
		// Handle specific status codes
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return "", errors.ErrInvalidInput
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
