package client

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

// LLMProvider 定义 LLM 服务提供商的通用接口
// 支持不同的 LLM API（OpenAI 兼容和非兼容）
type LLMProvider interface {
	GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Client 负责与 LLM API 进行交互。
// 该结构体通过注入的 LLMProvider 支持多种 LLM 服务商。
// English log messages should be emitted by callers; 此处仅返回错误。
//
// 注意：所有公共方法都应接受 context.Context 以便调用方控制取消与超时。
//
// Example:
//
//	c := client.NewClient(logger)
//	msg, err := c.GetCommitMessage(ctx, "<system>", "<user>")
//
// commit message 在成功时返回，错误由调用方处理。
//
// 未来可使用 go generate 生成接口 mock，以便单元测试其它模块。
// ------------------------------------------------------------------------------------
//
//go:generate mockgen -source=client.go -destination=../mocks/client_mock.go -package=mocks
type Client struct {
	provider LLMProvider // LLM 服务提供商
	logger   *zap.Logger // 结构化日志记录器
}

// OpenAICompatibleProvider 实现 OpenAI 兼容的 LLM API 调用
type OpenAICompatibleProvider struct {
	apiURL     string       // 完整的 API 端点 URL
	apiKey     string       // 鉴权所需的 API Key
	model      string       // 模型名称
	httpClient *http.Client // 可注入自定义 http.Client，用于超时与测试
}

// NewClient 创建一个 LLM Client。
// 所有超时控制通过传入的 context.Context 实现，确保信号处理的即时响应。
func NewClient(logger *zap.Logger) *Client {
	provider := NewOpenAICompatibleProvider()
	return &Client{
		provider: provider,
		logger:   logger,
	}
}

// NewClientWithProvider 创建一个使用指定 Provider 的 Client。
func NewClientWithProvider(provider LLMProvider, logger *zap.Logger) *Client {
	return &Client{
		provider: provider,
		logger:   logger,
	}
}

// NewOpenAICompatibleProvider 创建一个 OpenAI 兼容的 Provider
// 从环境变量读取配置，支持 DeepSeek, Volcengine 等 OpenAI 兼容 API
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
			// 不设置 Timeout，完全依赖 context 控制超时和取消
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

// chatResponse 对应 DeepSeek Chat API 的完整响应格式。
// 包含所有 API 返回的字段，确保与实际响应结构匹配。
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

// GetCompletion 实现 OpenAI 兼容的 API 调用
func (p *OpenAICompatibleProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// 检查 API Key 是否设置
	if p.apiKey == "" {
		return "", errors.ErrLLMAPIKey
	}
	
	// 构建请求体，使用 system 和 user 消息分离
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

	// 构建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, bytes.NewReader(data))
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to create request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	// 发送请求
	resp, err := p.httpClient.Do(req)
	if err != nil {
		// 如果是 context 取消或超时，返回适当的错误类型
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.ErrLLMTimeout
		}
		if strings.Contains(err.Error(), "timeout") {
			return "", errors.ErrLLMTimeout
		}
		// 其他网络错误
		return "", errors.WrapRetryable(errors.ErrTypeLLM, "network request failed", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 读取响应体以便错误处理和解析。
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to read response", err)
	}

	// 非 200 统一处理为错误输出，包含状态码但不包含响应体以防泄露敏感信息。
	if resp.StatusCode != http.StatusOK {
		// 处理特定的状态码
		switch resp.StatusCode {
		case http.StatusTooManyRequests:
			return "", errors.ErrLLMRateLimit
		case http.StatusUnauthorized:
			return "", errors.New(errors.ErrTypeAuth, "API authentication failed").WithSuggestion("检查您的 API Key 是否正确")
		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable:
			return "", errors.NewRetryable(errors.ErrTypeLLM, fmt.Sprintf("API server error: status %d", resp.StatusCode))
		default:
			return "", errors.New(errors.ErrTypeLLM, fmt.Sprintf("API error: status %d", resp.StatusCode))
		}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", errors.Wrap(errors.ErrTypeLLM, "failed to parse response", err).WithSuggestion("API 响应格式可能已变更，请检查 API 文档")
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.ErrLLMResponse
	}

	// 验证响应内容完整性
	if chatResp.Choices[0].Message.Content == "" {
		return "", errors.New(errors.ErrTypeLLM, "invalid response: empty message content")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// GetCommitMessage 调用 LLM API 生成 commit message。
// systemPrompt 包含角色定义、任务说明和格式规则。
// userPrompt 包含上下文数据（分支、文件、提交历史、diff）。
// 成功返回 message 字符串，失败返回错误。
func (c *Client) GetCommitMessage(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	// 详细的调试日志记录
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
	
	// 委托给 Provider 执行实际调用
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

