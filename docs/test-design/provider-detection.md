# Provider检测测试策略设计

**版本**: v1.0  
**生成日期**: 2025-01-17  
**基于文档**: devplan-pr.md

## 测试目标

设计全面覆盖Provider检测功能的测试用例，确保：
1. 正确解析各种格式的Git remote URL
2. 准确识别GitHub、Gitea等Provider类型
3. 可靠地进行HTTP探测和验证
4. 优雅地处理错误和边界情况

## 测试范围

### 1. URL解析测试
- **标准HTTPS格式**: `https://github.com/owner/repo.git`
- **SSH格式**: `git@github.com:owner/repo.git`
- **带端口的SSH**: `ssh://git@gitea.com:2222/owner/repo.git`
- **无.git后缀**: `https://github.com/owner/repo`
- **自定义域名**: `https://git.company.com/owner/repo.git`
- **复杂路径**: `https://gitlab.com/group/subgroup/repo.git`

### 2. Provider识别测试
- **GitHub识别**: github.com域名的各种变体
- **Gitea识别**: 通过HTTP探测API端点
- **GitLab识别**: gitlab.com及自建GitLab
- **未知Provider**: 无法识别时的降级处理

### 3. HTTP探测测试
- **成功探测**: 200状态码，正确的响应内容
- **重试机制**: 网络错误时的指数退避重试
- **超时处理**: 单次请求和总体超时控制
- **认证处理**: 处理401/403等认证错误
- **SSL证书**: 自签名证书的处理

### 4. 边界情况测试
- **空URL**: 处理空字符串输入
- **无效URL**: 格式错误的URL
- **无网络**: 网络不可达时的处理
- **DNS解析失败**: 域名无法解析
- **重定向**: HTTP 301/302重定向处理

## 测试用例详细设计

### URL解析测试用例

```go
// TestParseGitRemoteURL
testCases := []struct {
    name     string
    input    string
    expected RemoteInfo
    wantErr  bool
}{
    {
        name:  "GitHub HTTPS with .git",
        input: "https://github.com/owner/repo.git",
        expected: RemoteInfo{
            Provider: "github",
            Host:     "github.com",
            Owner:    "owner",
            Repo:     "repo",
            Protocol: "https",
        },
    },
    {
        name:  "GitHub SSH",
        input: "git@github.com:owner/repo.git",
        expected: RemoteInfo{
            Provider: "github",
            Host:     "github.com",
            Owner:    "owner",
            Repo:     "repo",
            Protocol: "ssh",
        },
    },
    {
        name:  "Gitea with custom port",
        input: "ssh://git@gitea.company.com:2222/owner/repo.git",
        expected: RemoteInfo{
            Provider: "unknown", // 需要HTTP探测确认
            Host:     "gitea.company.com",
            Port:     2222,
            Owner:    "owner",
            Repo:     "repo",
            Protocol: "ssh",
        },
    },
    {
        name:  "GitLab with subgroups",
        input: "https://gitlab.com/group/subgroup/repo.git",
        expected: RemoteInfo{
            Provider: "gitlab",
            Host:     "gitlab.com",
            Owner:    "group/subgroup",
            Repo:     "repo",
            Protocol: "https",
        },
    },
    {
        name:    "Invalid URL",
        input:   "not-a-url",
        wantErr: true,
    },
}
```

### HTTP探测测试用例

```go
// TestHTTPProbe
testCases := []struct {
    name           string
    mockResponses  []MockResponse
    expectedResult ProbeResult
    expectedRetries int
}{
    {
        name: "Gitea API detected",
        mockResponses: []MockResponse{
            {
                StatusCode: 200,
                Body:       `{"version":"1.21.0"}`,
                Headers:    map[string]string{"Content-Type": "application/json"},
            },
        },
        expectedResult: ProbeResult{
            IsGitea: true,
            Version: "1.21.0",
        },
        expectedRetries: 0,
    },
    {
        name: "Retry on network error",
        mockResponses: []MockResponse{
            {Error: &net.OpError{Op: "dial"}},
            {Error: &net.OpError{Op: "dial"}},
            {StatusCode: 200, Body: `{"version":"1.21.0"}`},
        },
        expectedResult: ProbeResult{
            IsGitea: true,
            Version: "1.21.0",
        },
        expectedRetries: 2,
    },
    {
        name: "Timeout after max retries",
        mockResponses: []MockResponse{
            {Error: context.DeadlineExceeded},
            {Error: context.DeadlineExceeded},
            {Error: context.DeadlineExceeded},
        },
        expectedResult: ProbeResult{
            Error: ErrProbeTimeout,
        },
        expectedRetries: 3,
    },
}
```

### 集成测试场景

```go
// TestProviderDetection_Integration
scenarios := []struct {
    name           string
    gitRemoteURL   string
    httpEndpoint   string
    expectedProvider string
}{
    {
        name:           "GitHub public repo",
        gitRemoteURL:   "https://github.com/golang/go.git",
        expectedProvider: "github",
    },
    {
        name:           "Gitea with API",
        gitRemoteURL:   "https://gitea.com/gitea/gitea.git",
        httpEndpoint:   "https://gitea.com/api/v1/version",
        expectedProvider: "gitea",
    },
    {
        name:           "Self-hosted unknown",
        gitRemoteURL:   "https://git.internal.com/team/project.git",
        expectedProvider: "unknown",
    },
}
```

## Mock策略

### 1. HTTP Client Mock
使用 `httpmock` 或自定义 RoundTripper 实现：
- 模拟各种HTTP响应状态
- 控制响应延迟和超时
- 记录重试次数和请求详情

### 2. Git Command Mock
使用接口抽象Git操作：
- Mock `git remote -v` 输出
- 控制不同的remote配置场景

### 3. 时间控制
使用可注入的时钟接口：
- 控制重试间隔
- 测试超时行为

## 测试执行计划

### 阶段1: 单元测试（Red Phase）
1. 编写URL解析测试 - 确保所有测试失败
2. 编写HTTP探测测试 - 包含重试和超时测试
3. 运行测试确认全部失败

### 阶段2: 实现（Green Phase）
1. 实现最小可行的URL解析器
2. 实现带重试的HTTP探测器
3. 确保所有测试通过

### 阶段3: 重构（Refactor Phase）
1. 提取通用Provider接口
2. 优化错误处理和日志
3. 保持测试通过

## 性能测试考虑

1. **并发探测**: 测试同时探测多个Provider的性能
2. **缓存效果**: 测试Provider信息缓存的命中率
3. **超时设置**: 验证超时配置的合理性

## 测试数据管理

1. **测试fixtures**: 
   - 各种URL格式样本
   - 模拟的API响应数据
   
2. **环境隔离**:
   - 不依赖外部网络
   - 使用本地Mock服务器

## 覆盖率目标

- URL解析模块: ≥ 95%
- HTTP探测模块: ≥ 90%
- 集成测试: ≥ 85%
- 整体覆盖率: ≥ 90%

## 注意事项

1. 测试必须可重复执行
2. 避免测试之间的依赖
3. 使用表驱动测试提高可维护性
4. 为每个测试场景添加清晰的描述
5. 错误信息要有助于调试