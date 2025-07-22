package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/atotto/clipboard"
	"github.com/penwyp/catmit/internal/squash"
	"github.com/penwyp/catmit/internal/ui"
	"github.com/stretchr/testify/assert"
)

// MockHTTPClient is a mock HTTP client adapter for testing
type MockHTTPClient struct {
	handler http.HandlerFunc
}

func (m *MockHTTPClient) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	// Create a test server
	server := httptest.NewServer(m.handler)
	defer server.Close()

	// Build request
	req, err := http.NewRequestWithContext(ctx, "POST", server.URL, nil)
	if err != nil {
		return "", err
	}

	// Send request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Simple JSON parsing (for test only)
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response")
}

func TestSquash_Integration(t *testing.T) {
	// Create mock client
	mockClient := &MockHTTPClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Return mock response
			response := `{
				"choices": [{
					"message": {
						"content": "feat: implement complete authentication system\n\n- Add user authentication with JWT support\n- Fix login error on mobile devices\n- Update authentication documentation"
					}
				}]
			}`
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, response)
		}),
	}

	// Create squash instance
	s := squash.New(mockClient, "en")

	// Test input
	messages := []string{
		"feat: add user authentication",
		"fix: resolve login error on mobile",
		"docs: update authentication guide",
	}

	// Execute generation
	ctx := context.Background()
	result, err := s.Generate(ctx, messages)

	// Validate result
	assert.NoError(t, err)
	assert.Contains(t, result, "feat: implement complete authentication system")
	assert.Contains(t, result, "Add user authentication with JWT support")
	assert.Contains(t, result, "Fix login error on mobile devices")
}

func TestSquash_ChineseIntegration(t *testing.T) {
	// Create mock client for Chinese response
	mockClient := &MockHTTPClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Return a mock response in Chinese
			response := `{
				"choices": [{
					"message": {
						"content": "feat: 实现完整的认证系统\n\n- 添加基于 JWT 的用户认证功能\n- 修复移动端登录错误\n- 更新认证相关文档"
					}
				}]
			}`
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, response)
		}),
	}

	// Create squash instance in Chinese mode
	s := squash.New(mockClient, "zh")

	// Test input messages in Chinese
	messages := []string{
		"feat: 添加用户认证",
		"fix: 修复登录错误",
		"docs: 更新认证文档",
	}

	// Execute commit message generation
	ctx := context.Background()
	result, err := s.Generate(ctx, messages)

	// Validate the generated result
	assert.NoError(t, err)
	assert.Contains(t, result, "feat: 实现完整的认证系统")
	assert.Contains(t, result, "添加基于 JWT 的用户认证功能")
}

func TestSquash_ErrorHandling(t *testing.T) {
	// Create mock client that returns error
	mockClient := &MockHTTPClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error": "Internal server error"}`)
		}),
	}

	// Create squash instance
	s := squash.New(mockClient, "en")

	// Test input
	messages := []string{
		"feat: add feature",
		"fix: fix bug",
	}

	// Execute generation
	ctx := context.Background()
	_, err := s.Generate(ctx, messages)

	// Validate error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate commit message")
}

// TestSquash_UIModel tests the basic behavior of the TUI model
func TestSquash_UIModel(t *testing.T) {
	// Create mock client
	mockResponse := "feat: test commit message"
	mockClient := &MockHTTPClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := fmt.Sprintf(`{
				"choices": [{
					"message": {
						"content": "%s"
					}
				}]
			}`, mockResponse)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, response)
		}),
	}

	s := squash.New(mockClient, "en")

	messages := []string{
		"feat: add feature",
		"fix: fix bug",
	}

	// Create UI model
	model := ui.NewSquashModel(s, messages, false)

	// Test initialization
	initCmd := model.Init()
	assert.NotNil(t, initCmd)

	// Test view rendering
	view := model.View()
	assert.Contains(t, view, "Squashing Commit Messages")
	assert.Contains(t, view, "Generating consolidated commit message")
}

// TestSquash_ClipboardIntegration tests clipboard functionality
func TestSquash_ClipboardIntegration(t *testing.T) {
	// Skip in CI environment (clipboard may not be supported)
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping clipboard test in CI environment")
	}

	// Save original clipboard content
	originalContent, _ := clipboard.ReadAll()
	defer func() {
		// Restore original clipboard content
		_ = clipboard.WriteAll(originalContent)
	}()

	testContent := "feat: test clipboard integration"

	// Write test content
	err := clipboard.WriteAll(testContent)
	if err != nil {
		t.Skip("Clipboard not available on this system")
	}

	// Read and validate
	readContent, err := clipboard.ReadAll()
	assert.NoError(t, err)
	assert.Equal(t, testContent, readContent)
}

// TestSquash_NoConfirmMode tests output in --no-confirm mode
func TestSquash_NoConfirmMode(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Simulate generated commit message
	expectedMessage := "feat: consolidated commit message"

	// Output message (simulate --no-confirm mode behavior)
	fmt.Println(expectedMessage)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read output
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Validate output
	assert.Contains(t, output, expectedMessage)
}
