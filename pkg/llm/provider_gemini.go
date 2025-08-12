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

// GeminiProvider implements Google AI Studio (Gemini) API calls.
type GeminiProvider struct {
	apiURL     string       // API endpoint URL
	apiKey     string       // API Key for authentication
	model      string       // Model name
	httpClient *http.Client // Customizable http.Client for timeout and testing
}

// NewGeminiProvider creates a Google Gemini Provider.
// Reads configuration from environment variables.
func NewGeminiProvider() *GeminiProvider {
	apiKey := os.Getenv("CATMIT_LLM_API_KEY")

	model := os.Getenv("CATMIT_LLM_MODEL")
	if model == "" {
		model = "gemini-pro"
	}

	// Build API URL based on model
	apiURL := os.Getenv("CATMIT_LLM_API_URL")
	if apiURL == "" {
		apiURL = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	}

	return &GeminiProvider{
		apiURL: apiURL,
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			// No Timeout set, fully relying on context for timeout and cancellation
		},
	}
}

// geminiRequest defines the Gemini-specific request format
type geminiRequest struct {
	Contents         []geminiContent    `json:"contents"`
	GenerationConfig *generationConfig  `json:"generationConfig,omitempty"`
}

// geminiContent represents content in Gemini format
type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

// geminiPart represents a part of content
type geminiPart struct {
	Text string `json:"text"`
}

// generationConfig contains generation parameters
type generationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

// geminiResponse represents the Gemini API response
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
		Index        int    `json:"index"`
	} `json:"candidates"`
	PromptFeedback struct {
		SafetyRatings []struct {
			Category    string `json:"category"`
			Probability string `json:"probability"`
		} `json:"safetyRatings"`
	} `json:"promptFeedback"`
}

// geminiStreamChunk represents a streaming chunk from Gemini
type geminiStreamChunk struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

// GetCompletion implements the Gemini API call.
func (p *GeminiProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// Check if API Key is set
	if p.apiKey == "" {
		return "", errors.ErrLLMAPIKey
	}

	// Build request body - Gemini combines system and user prompts
	combinedPrompt := systemPrompt
	if systemPrompt != "" && userPrompt != "" {
		combinedPrompt = systemPrompt + "\n\n" + userPrompt
	} else if userPrompt != "" {
		combinedPrompt = userPrompt
	}

	contents := []geminiContent{
		{
			Role: "user",
			Parts: []geminiPart{
				{Text: combinedPrompt},
			},
		},
	}

	reqBody := geminiRequest{
		Contents: contents,
		GenerationConfig: &generationConfig{
			MaxOutputTokens: 128,
			Temperature:     0.7,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to marshal request", err)
	}

	// Build URL with API key
	url := p.apiURL
	if p.apiKey != "" {
		if strings.Contains(url, "?") {
			url += "&key=" + p.apiKey
		} else {
			url += "?key=" + p.apiKey
		}
	}

	// Build HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
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
			return "", errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, url))
		}
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to parse response", err).WithSuggestion("API response format may have changed, please check API documentation")
	}

	if len(geminiResp.Candidates) == 0 {
		return "", errors.ErrLLMResponse
	}

	// Extract text from the first candidate
	if len(geminiResp.Candidates[0].Content.Parts) > 0 {
		var textParts []string
		for _, part := range geminiResp.Candidates[0].Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		}
		if len(textParts) > 0 {
			return strings.Join(textParts, ""), nil
		}
	}

	return "", errors.New(errors.ErrTypeLLM, "invalid response: no text content found")
}

// GetCompletionStream implements the Gemini streaming API call.
func (p *GeminiProvider) GetCompletionStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
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

		// Build request body - Gemini combines system and user prompts
		combinedPrompt := systemPrompt
		if systemPrompt != "" && userPrompt != "" {
			combinedPrompt = systemPrompt + "\n\n" + userPrompt
		} else if userPrompt != "" {
			combinedPrompt = userPrompt
		}

		contents := []geminiContent{
			{
				Role: "user",
				Parts: []geminiPart{
					{Text: combinedPrompt},
				},
			},
		}

		reqBody := geminiRequest{
			Contents: contents,
			GenerationConfig: &generationConfig{
				MaxOutputTokens: 128,
				Temperature:     0.7,
			},
		}

		data, err := json.Marshal(reqBody)
		if err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to marshal request", err)
			return
		}

		// Build URL with API key and streaming parameter
		url := p.apiURL
		// Replace generateContent with streamGenerateContent for streaming
		url = strings.Replace(url, ":generateContent", ":streamGenerateContent", 1)
		if p.apiKey != "" {
			if strings.Contains(url, "?") {
				url += "&key=" + p.apiKey
			} else {
				url += "?key=" + p.apiKey
			}
		}
		url += "&alt=sse"

		// Build HTTP request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to create request", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

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
				errChan <- errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d, endpoint: %s", resp.StatusCode, url))
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
				
				// Parse JSON chunk
				var chunk geminiStreamChunk
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					// Skip invalid chunks
					continue
				}

				// Extract text from parts
				if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
					for _, part := range chunk.Candidates[0].Content.Parts {
						if part.Text != "" {
							select {
							case contentChan <- part.Text:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}
	}()

	return contentChan, errChan
}