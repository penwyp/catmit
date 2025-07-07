package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client 负责与 DeepSeek Chat API 进行交互。
// 该结构体通过注入的 http.Client 支持超时与测试时的伪造服务器。
// English log messages should be emitted by callers; 此处仅返回错误。
//
// 注意：所有公共方法都应接受 context.Context 以便调用方控制取消与超时。
//
// Example:
//
//	c := client.NewClient("https://api.deepseek.com", os.Getenv("DEEPSEEK_API_KEY"), 20*time.Second)
//	msg, err := c.GetCommitMessage(ctx, "<prompt>")
//
// commit message 在成功时返回，错误由调用方处理。
//
// 未来可使用 go generate 生成接口 mock，以便单元测试其它模块。
// ------------------------------------------------------------------------------------
//
//go:generate mockgen -source=client.go -destination=../mocks/client_mock.go -package=mocks
type Client struct {
	baseURL    string       // DeepSeek API 基础地址，例如 https://api.deepseek.com
	apiKey     string       // 鉴权所需的 API Key
	httpClient *http.Client // 可注入自定义 http.Client，用于超时与测试
}

// NewClient 创建一个 DeepSeek Client。
// timeout 用于设置 http.Client 的全局超时，以防请求阻塞。
func NewClient(baseURL, apiKey string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// chatRequest 定义了 DeepSeek Chat API 请求体结构。
// 私有结构体仅用于序列化。
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

// chatResponse 对应 DeepSeek Chat API 的响应格式。
type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// GetCommitMessage 调用 DeepSeek API 生成 commit message。
// systemPrompt 包含角色定义、任务说明和格式规则。
// userPrompt 包含上下文数据（分支、文件、提交历史、diff）。
// 成功返回 message 字符串，失败返回错误。
func (c *Client) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// 构建请求体，使用 system 和 user 消息分离
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	
	reqBody := chatRequest{
		Model:       "deepseek-chat",
		Messages:    messages,
		MaxTokens:   128,
		Temperature: 0.7,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// 如果是 context 取消或超时，直接返回原始错误以便调用方区分。
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应体以便错误处理和解析。
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// 非 200 统一处理为错误输出，包含状态码但不包含响应体以防泄露敏感信息。
	if resp.StatusCode != http.StatusOK {
		// 只记录状态码，不记录响应体内容以防泄露 API 密钥等敏感信息
		return "", fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("invalid response: empty choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// GetCommitMessageLegacy 调用 DeepSeek API 生成 commit message（旧版本兼容）。
// prompt 为经过模板渲染后的完整提示文本。
// 成功返回 message 字符串，失败返回错误。
// 已废弃：建议使用 GetCommitMessage(systemPrompt, userPrompt) 替代。
func (c *Client) GetCommitMessageLegacy(ctx context.Context, prompt string) (string, error) {
	// 构建请求体
	reqBody := chatRequest{
		Model:       "deepseek-chat",
		Messages:    []chatMessage{{Role: "user", Content: prompt}},
		MaxTokens:   128,
		Temperature: 0.7,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// 如果是 context 取消或超时，直接返回原始错误以便调用方区分。
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应体以便错误处理和解析。
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// 非 200 统一处理为错误输出，包含状态码但不包含响应体以防泄露敏感信息。
	if resp.StatusCode != http.StatusOK {
		// 只记录状态码，不记录响应体内容以防泄露 API 密钥等敏感信息
		return "", fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("invalid response: empty choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
