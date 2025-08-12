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

// OllamaProvider implements Ollama local API calls.
type OllamaProvider struct {
	apiURL     string       // API endpoint URL
	model      string       // Model name
	httpClient *http.Client // Customizable http.Client for timeout and testing
}

// NewOllamaProvider creates an Ollama Provider.
// Reads configuration from environment variables.
func NewOllamaProvider() *OllamaProvider {
	apiURL := os.Getenv("CATMIT_LLM_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:11434/api/chat"
	}

	model := os.Getenv("CATMIT_LLM_MODEL")
	if model == "" {
		model = "llama2"
	}

	return &OllamaProvider{
		apiURL: apiURL,
		model:  model,
		httpClient: &http.Client{
			// No Timeout set, fully relying on context for timeout and cancellation
		},
	}
}

// ollamaMessage represents a message in Ollama format
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaRequest defines the Ollama-specific request format
type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  *ollamaOptions  `json:"options,omitempty"`
}

// ollamaOptions contains optional parameters for Ollama
type ollamaOptions struct {
	NumPredict  int     `json:"num_predict,omitempty"`  // Max tokens to generate
	Temperature float64 `json:"temperature,omitempty"`
}

// ollamaResponse represents the non-streaming response from Ollama
type ollamaResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason,omitempty"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalCount          int    `json:"eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
}

// ollamaStreamResponse represents a streaming response from Ollama
type ollamaStreamResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done          bool   `json:"done"`
	DoneReason    string `json:"done_reason,omitempty"`
	TotalDuration int64  `json:"total_duration,omitempty"`
	EvalCount     int    `json:"eval_count,omitempty"`
}

// GetCompletion implements the Ollama API call.
func (p *OllamaProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Build messages array
	messages := []ollamaMessage{}
	
	// Add system message if provided
	if systemPrompt != "" {
		messages = append(messages, ollamaMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	
	// Add user message
	messages = append(messages, ollamaMessage{
		Role:    "user",
		Content: userPrompt,
	})

	reqBody := ollamaRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   false,
		Options: &ollamaOptions{
			NumPredict:  128,
			Temperature: 0.7,
		},
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
		// Check if Ollama is not running
		if strings.Contains(err.Error(), "connection refused") {
			return "", errors.New(errors.ErrTypeLLM, "Ollama server is not running. Please start Ollama with 'ollama serve'")
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
		case http.StatusNotFound:
			return "", errors.New(errors.ErrTypeLLM, fmt.Sprintf("model '%s' not found. Please pull the model with 'ollama pull %s'", p.model, p.model))
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
			return "", errors.NewRetryable(errors.ErrTypeLLM, fmt.Sprintf("Ollama server error: status %d", resp.StatusCode))
		default:
			return "", errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, p.apiURL))
		}
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(bodyBytes, &ollamaResp); err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to parse response", err).WithSuggestion("API response format may have changed, please check Ollama documentation")
	}

	// Validate response content
	if ollamaResp.Message.Content == "" {
		return "", errors.New(errors.ErrTypeLLM, "invalid response: empty message content")
	}

	return ollamaResp.Message.Content, nil
}

// GetCompletionStream implements the Ollama streaming API call.
func (p *OllamaProvider) GetCompletionStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
	contentChan := make(chan string, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errChan)

		// Build messages array
		messages := []ollamaMessage{}
		
		// Add system message if provided
		if systemPrompt != "" {
			messages = append(messages, ollamaMessage{
				Role:    "system",
				Content: systemPrompt,
			})
		}
		
		// Add user message
		messages = append(messages, ollamaMessage{
			Role:    "user",
			Content: userPrompt,
		})

		reqBody := ollamaRequest{
			Model:    p.model,
			Messages: messages,
			Stream:   true,
			Options: &ollamaOptions{
				NumPredict:  128,
				Temperature: 0.7,
			},
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
			// Check if Ollama is not running
			if strings.Contains(err.Error(), "connection refused") {
				errChan <- errors.New(errors.ErrTypeLLM, "Ollama server is not running. Please start Ollama with 'ollama serve'")
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
			case http.StatusNotFound:
				errChan <- errors.New(errors.ErrTypeLLM, fmt.Sprintf("model '%s' not found. Please pull the model with 'ollama pull %s'", p.model, p.model))
			case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
				errChan <- errors.NewRetryable(errors.ErrTypeLLM, fmt.Sprintf("Ollama server error: status %d", resp.StatusCode))
			default:
				errChan <- errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, p.apiURL))
			}
			return
		}

		// Read newline-delimited JSON stream
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			// Parse JSON response
			var streamResp ollamaStreamResponse
			if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
				// Skip invalid chunks
				continue
			}

			// Send content if available
			if streamResp.Message.Content != "" {
				select {
				case contentChan <- streamResp.Message.Content:
				case <-ctx.Done():
					return
				}
			}

			// Check if streaming is done
			if streamResp.Done {
				break
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to read stream", err)
			return
		}
	}()

	return contentChan, errChan
}