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
)

// AnthropicProvider implements Anthropic Claude API calls.
type AnthropicProvider struct {
	apiURL     string       // API endpoint URL
	apiKey     string       // API Key for authentication
	model      string       // Model name
	httpClient *http.Client // Customizable http.Client for timeout and testing
}

// NewAnthropicProvider creates an Anthropic Claude Provider.
// Reads configuration from environment variables.
func NewAnthropicProvider() *AnthropicProvider {
	apiURL := os.Getenv("CATMIT_LLM_API_URL")
	if apiURL == "" {
		apiURL = "https://api.anthropic.com/v1/messages"
	}

	apiKey := os.Getenv("CATMIT_LLM_API_KEY")

	model := os.Getenv("CATMIT_LLM_MODEL")
	if model == "" {
		model = "claude-3-sonnet-20240229"
	}

	return &AnthropicProvider{
		apiURL: apiURL,
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			// No Timeout set, fully relying on context for timeout and cancellation
		},
	}
}

// anthropicRequest defines the Anthropic-specific request format
type anthropicRequest struct {
	Model       string            `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string            `json:"system,omitempty"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature float64           `json:"temperature"`
	Stream      bool              `json:"stream,omitempty"`
}

// anthropicMessage represents a message in Anthropic format
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse represents the Anthropic API response
type anthropicResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Content      []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// anthropicStreamEvent represents a streaming event from Anthropic
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	ContentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content_block"`
}

// GetCompletion implements the Anthropic API call.
func (p *AnthropicProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Check if API Key is set
	if p.apiKey == "" {
		return "", errors.ErrLLMAPIKey
	}

	// Build request body - Anthropic uses separate system field
	messages := []anthropicMessage{
		{Role: "user", Content: userPrompt},
	}

	reqBody := anthropicRequest{
		Model:       p.model,
		Messages:    messages,
		System:      systemPrompt,
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
	req.Header.Set("anthropic-version", "2023-06-01")
	if p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
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

	// Non-200 responses are handled as errors
	if resp.StatusCode != http.StatusOK {
		// Log response body for debugging (only in debug mode)
		errorBody := string(bodyBytes)
		if len(errorBody) > 500 {
			errorBody = errorBody[:500] + "..."
		}
		
		// Handle specific status codes
		switch resp.StatusCode {
		case http.StatusBadRequest:
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
			return "", errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, p.apiURL))
		}
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(bodyBytes, &anthropicResp); err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to parse response", err).WithSuggestion("API response format may have changed, please check API documentation")
	}

	if len(anthropicResp.Content) == 0 {
		return "", errors.ErrLLMResponse
	}

	// Extract text from the first content block
	for _, content := range anthropicResp.Content {
		if content.Type == "text" && content.Text != "" {
			return content.Text, nil
		}
	}

	return "", errors.New(errors.ErrTypeLLM, "invalid response: no text content found")
}

// GetCompletionStream implements the Anthropic streaming API call.
func (p *AnthropicProvider) GetCompletionStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
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

		// Build request body - Anthropic uses separate system field
		messages := []anthropicMessage{
			{Role: "user", Content: userPrompt},
		}

		reqBody := anthropicRequest{
			Model:       p.model,
			Messages:    messages,
			System:      systemPrompt,
			MaxTokens:   128,
			Temperature: 0.7,
			Stream:      true,
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
		req.Header.Set("anthropic-version", "2023-06-01")
		if p.apiKey != "" {
			req.Header.Set("x-api-key", p.apiKey)
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
				errChan <- errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, p.apiURL))
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
				
				// Parse JSON event
				var event anthropicStreamEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					// Skip invalid chunks
					continue
				}

				// Extract content from different event types
				if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
					select {
					case contentChan <- event.Delta.Text:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return contentChan, errChan
}