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

// AzureOpenAIProvider implements Azure OpenAI API calls.
type AzureOpenAIProvider struct {
	baseURL    string       // Azure OpenAI resource base URL (e.g., https://myresource.openai.azure.com)
	apiKey     string       // API Key for authentication
	deployment string       // Deployment name (model deployment)
	apiVersion string       // API version (defaults to 2024-10-21)
	httpClient *http.Client // Customizable http.Client for timeout and testing
}

// NewAzureOpenAIProvider creates an Azure OpenAI Provider.
// Reads configuration from environment variables, reusing existing CATMIT_LLM_* variables.
func NewAzureOpenAIProvider() *AzureOpenAIProvider {
	baseURL := os.Getenv("CATMIT_LLM_API_URL")
	if baseURL == "" {
		baseURL = "https://myresource.openai.azure.com" // Default placeholder
	}

	apiKey := os.Getenv("CATMIT_LLM_API_KEY")

	deployment := os.Getenv("CATMIT_LLM_MODEL")
	if deployment == "" {
		deployment = "gpt-4" // Default deployment name
	}

	apiVersion := "2024-10-21" // Latest GA version, hardcoded

	return &AzureOpenAIProvider{
		baseURL:    baseURL,
		apiKey:     apiKey,
		deployment: deployment,
		apiVersion: apiVersion,
		httpClient: &http.Client{
			// No Timeout set, fully relying on context for timeout and cancellation
		},
	}
}

// GetCompletion implements the Azure OpenAI API call.
func (p *AzureOpenAIProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Check if API Key is set
	if p.apiKey == "" {
		return "", errors.ErrLLMAPIKey
	}

	// Build Azure OpenAI API endpoint
	// Remove trailing slash from baseURL if present
	baseURL := strings.TrimRight(p.baseURL, "/")
	apiURL := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", 
		baseURL, p.deployment, p.apiVersion)

	// Build request body, separating system and user messages
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	reqBody := chatRequest{
		Model:               p.deployment, // Azure uses deployment name
		Messages:            messages,
		MaxTokens:           128,
		Temperature:         0.7,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to marshal request", err)
	}

	// Build HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(data))
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to create request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// Azure uses api-key header instead of Authorization: Bearer
	if p.apiKey != "" {
		req.Header.Set("api-key", p.apiKey)
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
			return "", errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, apiURL))
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

// GetCompletionStream implements the Azure OpenAI streaming API call.
func (p *AzureOpenAIProvider) GetCompletionStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
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

		// Build Azure OpenAI API endpoint
		// Remove trailing slash from baseURL if present
		baseURL := strings.TrimRight(p.baseURL, "/")
		apiURL := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", 
			baseURL, p.deployment, p.apiVersion)

		// Build request body, separating system and user messages
		messages := []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}

		reqBody := chatRequest{
			Model:               p.deployment, // Azure uses deployment name
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(data))
		if err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to create request", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		// Azure uses api-key header instead of Authorization: Bearer
		if p.apiKey != "" {
			req.Header.Set("api-key", p.apiKey)
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
				errChan <- errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, apiURL))
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