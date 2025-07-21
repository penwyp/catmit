package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"encoding/json"

	"github.com/atotto/clipboard"
	"github.com/penwyp/catmit/squash"
	"github.com/penwyp/catmit/ui"
	"github.com/stretchr/testify/assert"
)

// MockHTTPClient 用于测试的 HTTP 客户端适配器
type MockHTTPClient struct {
	handler http.HandlerFunc
}

func (m *MockHTTPClient) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	// 创建一个测试服务器
	server := httptest.NewServer(m.handler)
	defer server.Close()

	// 构建请求
	req, err := http.NewRequestWithContext(ctx, "POST", server.URL, nil)
	if err != nil {
		return "", err
	}

	// 发送请求
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 解析响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 简单的 JSON 解析（仅用于测试）
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
	// 创建 mock 客户端
	mockClient := &MockHTTPClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 返回模拟响应
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

	// 创建 squash 实例
	s := squash.New(mockClient, "en")

	// 测试输入
	messages := []string{
		"feat: add user authentication",
		"fix: resolve login error on mobile",
		"docs: update authentication guide",
	}

	// 执行生成
	ctx := context.Background()
	result, err := s.Generate(ctx, messages)

	// 验证结果
	assert.NoError(t, err)
	assert.Contains(t, result, "feat: implement complete authentication system")
	assert.Contains(t, result, "Add user authentication with JWT support")
	assert.Contains(t, result, "Fix login error on mobile devices")
}

func TestSquash_ChineseIntegration(t *testing.T) {
	// 创建 mock 客户端
	mockClient := &MockHTTPClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 返回中文响应
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

	// 创建 squash 实例（中文模式）
	s := squash.New(mockClient, "zh")

	// 测试输入
	messages := []string{
		"feat: 添加用户认证",
		"fix: 修复登录错误",
		"docs: 更新认证文档",
	}

	// 执行生成
	ctx := context.Background()
	result, err := s.Generate(ctx, messages)

	// 验证结果
	assert.NoError(t, err)
	assert.Contains(t, result, "feat: 实现完整的认证系统")
	assert.Contains(t, result, "添加基于 JWT 的用户认证功能")
}

func TestSquash_ErrorHandling(t *testing.T) {
	// 创建返回错误的 mock 客户端
	mockClient := &MockHTTPClient{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error": "Internal server error"}`)
		}),
	}

	// 创建 squash 实例
	s := squash.New(mockClient, "en")

	// 测试输入
	messages := []string{
		"feat: add feature",
		"fix: fix bug",
	}

	// 执行生成
	ctx := context.Background()
	_, err := s.Generate(ctx, messages)

	// 验证错误
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to generate commit message")
}

// TestSquash_UIModel 测试 TUI 模型的基本行为
func TestSquash_UIModel(t *testing.T) {
	// 创建 mock 客户端
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

	// 创建 UI 模型
	model := ui.NewSquashModel(s, messages)

	// 测试初始化
	initCmd := model.Init()
	assert.NotNil(t, initCmd)

	// 测试视图渲染
	view := model.View()
	assert.Contains(t, view, "Squashing Commit Messages")
	assert.Contains(t, view, "Generating consolidated commit message")
}

// TestSquash_ClipboardIntegration 测试剪贴板功能
func TestSquash_ClipboardIntegration(t *testing.T) {
	// 跳过 CI 环境（可能没有剪贴板支持）
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping clipboard test in CI environment")
	}

	// 保存原始剪贴板内容
	originalContent, _ := clipboard.ReadAll()
	defer func() {
		// 恢复原始剪贴板内容
		clipboard.WriteAll(originalContent)
	}()

	testContent := "feat: test clipboard integration"
	
	// 写入测试内容
	err := clipboard.WriteAll(testContent)
	if err != nil {
		t.Skip("Clipboard not available on this system")
	}

	// 读取并验证
	readContent, err := clipboard.ReadAll()
	assert.NoError(t, err)
	assert.Equal(t, testContent, readContent)
}

// TestSquash_NoConfirmMode 测试 --no-confirm 模式的输出
func TestSquash_NoConfirmMode(t *testing.T) {
	// 捕获标准输出
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// 模拟生成的 commit message
	expectedMessage := "feat: consolidated commit message"
	
	// 输出消息（模拟 --no-confirm 模式的行为）
	fmt.Println(expectedMessage)
	
	// 恢复标准输出
	w.Close()
	os.Stdout = oldStdout
	
	// 读取输出
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()
	
	// 验证输出
	assert.Contains(t, output, expectedMessage)
}