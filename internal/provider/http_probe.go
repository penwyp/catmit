package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// httpProber 实现HTTP探测功能
type httpProber struct {
	client     *http.Client
	maxRetries int
	timeout    time.Duration
}

// ProberOption 配置选项
type ProberOption func(*httpProber)

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(n int) ProberOption {
	return func(p *httpProber) {
		p.maxRetries = n
	}
}

// WithTimeout 设置单次请求超时
func WithTimeout(d time.Duration) ProberOption {
	return func(p *httpProber) {
		p.timeout = d
	}
}

// NewHTTPProber 创建新的HTTP探测器
func NewHTTPProber(opts ...ProberOption) HTTPProber {
	p := &httpProber{
		maxRetries: 3,
		timeout:    3 * time.Second,
	}

	for _, opt := range opts {
		opt(p)
	}

	p.client = &http.Client{
		Timeout: p.timeout,
	}

	return p
}

// ProbeGitea 探测Gitea API
func (p *httpProber) ProbeGitea(ctx context.Context, baseURL string) ProbeResult {
	url := fmt.Sprintf("%s/api/v1/version", baseURL)
	
	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			// 计算退避时间
			backoff := calculateBackoff(attempt - 1)
			select {
			case <-ctx.Done():
				return ProbeResult{
					IsGitea: false,
					Error:   ctx.Err(),
				}
			case <-time.After(backoff):
				// 继续重试
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return ProbeResult{
				IsGitea: false,
				Error:   err,
			}
		}

		resp, err := p.client.Do(req)
		if err != nil {
			lastErr = err
			// 网络错误，继续重试
			continue
		}
		defer resp.Body.Close()

		// 服务器错误（5xx），继续重试
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			continue
		}

		// 其他非200状态码，不重试
		if resp.StatusCode != http.StatusOK {
			return ProbeResult{
				IsGitea: false,
			}
		}

		// 解析响应
		var versionInfo struct {
			Version string `json:"version"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
			// JSON解析错误，不是Gitea
			return ProbeResult{
				IsGitea: false,
			}
		}

		// 成功检测到Gitea
		return ProbeResult{
			IsGitea: true,
			Version: versionInfo.Version,
		}
	}

	// 所有重试都失败
	if lastErr != nil {
		return ProbeResult{
			IsGitea: false,
			Error:   lastErr,
		}
	}

	return ProbeResult{
		IsGitea: false,
		Error:   ErrProbeTimeout,
	}
}

// calculateBackoff 计算指数退避时间
func calculateBackoff(attempt int) time.Duration {
	base := time.Second
	maxBackoff := 4 * time.Second

	backoff := base * (1 << attempt) // 2^attempt
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	return backoff
}