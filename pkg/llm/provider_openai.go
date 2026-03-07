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

// OpenAICompatibleProvider implements OpenAI-compatible LLM API calls.
type OpenAICompatibleProvider struct {
	apiURL     string       // Full API endpoint URL
	apiKey     string       // API Key for authentication
	model      string       // Model name
	httpClient *http.Client // Customizable http.Client for timeout and testing
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

// GetCompletion implements the OpenAI-compatible API call.
func (p *OpenAICompatibleProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	bearerToken, _, err := resolveLLMBearerToken(ctx, p.apiKey)
	if err != nil {
		return "", err
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
	req.Header.Set("Authorization", "Bearer "+bearerToken)

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
			return "", errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, p.apiURL))
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

// GetCompletionStream implements the OpenAI-compatible streaming API call.
func (p *OpenAICompatibleProvider) GetCompletionStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
	contentChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errChan)

		bearerToken, _, err := resolveLLMBearerToken(ctx, p.apiKey)
		if err != nil {
			errChan <- err
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
		req.Header.Set("Authorization", "Bearer "+bearerToken)

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