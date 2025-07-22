# catmit PR功能 Phase 2 开发计划

**版本**: v2.0  
**生成日期**: 2025-01-17  
**基于分析**: Phase 1 代码实现评估

---

## 第二阶段目标概述

Phase 1 已经实现了 PR 创建的核心功能，但存在以下不足需要在 Phase 2 解决：

1. **Provider 自动检测不完整** - 仅支持硬编码的域名模式，缺少 HTTP 探测和本地配置映射
2. **用户体验待优化** - 错误提示不够友好，缺少详细的安装指南和进度提示
3. **功能缺失** - 未实现配置文件管理、多 Provider 支持、PR 模板支持等
4. **测试覆盖不足** - 部分关键路径缺少测试，E2E 测试需要完善
5. **架构问题** - 命令行参数与 PRD 设计不一致，部分模块耦合度高

Phase 2 将聚焦于**完善自动检测、优化用户体验、扩展功能集、提升测试覆盖**。

---

## 核心功能点列表

### 1. Provider 检测增强（优化）
- [ ] 实现基于配置文件的自定义映射
- [ ] 完善 HTTP 探测机制（带重试和超时控制）
- [ ] 支持更多 Provider 模式识别（GitLab、Bitbucket）
- [ ] 改进未知 Provider 的处理流程

### 2. 配置文件管理（新增）
- [ ] 实现 `~/.config/catmit/providers.yaml` 的创建和管理
- [ ] 支持原子写入和并发保护
- [ ] 提供默认配置模板
- [ ] 实现配置热加载

### 3. CLI 参数对齐（优化）
- [ ] 将 `--create-pr` 改为 `-c/--pr`
- [ ] 新增 `--pr-remote`、`--pr-base`、`--pr-draft`、`--pr-provider` 参数
- [ ] 实现参数依赖关系验证
- [ ] 保持向后兼容性

### 4. 错误处理优化（优化）
- [ ] 实现统一的错误处理框架
- [ ] 提供友好的错误提示和解决方案
- [ ] 支持错误分类（可重试/不可重试）
- [ ] 改进 CLI 未安装时的引导

### 5. TUI 交互增强（新增）
- [ ] PR 创建前的预览界面
- [ ] 实时显示 PR 创建进度
- [ ] 支持交互式参数修改
- [ ] 优化错误显示方式

### 6. 多 Provider 支持（新增）
- [ ] 完整实现 Gitea 支持（tea CLI）
- [ ] 准备 GitLab 支持的基础架构
- [ ] 统一不同 Provider 的差异处理

### 7. PR 模板支持（新增）
- [ ] 检测仓库的 PR 模板文件
- [ ] 自动填充模板变量
- [ ] 支持自定义模板

### 8. Fork 工作流支持（新增）
- [ ] 检测 Fork 关系
- [ ] 自动设置正确的 head/base
- [ ] 支持跨仓库 PR

### 9. 测试覆盖提升（优化）
- [ ] 补充 HTTP 探测的单元测试
- [ ] 完善配置管理的测试
- [ ] 增加更多 E2E 场景
- [ ] 性能测试和压力测试

### 10. 文档完善（新增）
- [ ] 用户使用指南
- [ ] 配置示例
- [ ] 故障排查手册
- [ ] 贡献者指南

---

## 技术方案

### 1. Provider 检测架构重构

```go
// 新的 Provider 检测流程
type ProviderResolver struct {
    configManager  *config.Manager
    httpProber     *provider.HTTPProber
    patternMatcher *provider.PatternMatcher
}

func (r *ProviderResolver) Resolve(ctx context.Context, remoteURL string) (*provider.Info, error) {
    // 1. 解析 URL
    info, err := provider.ParseURL(remoteURL)
    
    // 2. 检查本地配置映射
    if mapped := r.configManager.GetMapping(info.Host); mapped != "" {
        info.Provider = mapped
        return info, nil
    }
    
    // 3. 模式匹配
    if matched := r.patternMatcher.Match(info.Host); matched != "" {
        info.Provider = matched
        return info, nil
    }
    
    // 4. HTTP 探测（带重试）
    if probeResult := r.httpProber.Probe(ctx, info.GetHTTPURL()); probeResult.Success {
        info.Provider = probeResult.Provider
        return info, nil
    }
    
    // 5. 返回 unknown，提示用户配置
    return info, ErrUnknownProvider
}
```

### 2. 配置文件结构

```yaml
# ~/.config/catmit/providers.yaml
version: 1

# 自定义域名映射
mappings:
  github.company.com: github
  git.mycompany.net: gitea
  gitlab.internal.com: gitlab

# 默认设置
defaults:
  pr_base: main
  pr_draft: false
  
# Provider 特定配置
providers:
  github:
    cli_command: gh
    min_version: "2.0.0"
  gitea:
    cli_command: tea
    min_version: "0.8.0"
  gitlab:
    cli_command: glab
    min_version: "0.15.0"

# HTTP 探测配置
http_probe:
  timeout: 3s
  retry_count: 3
  retry_delay: 1s
```

### 3. 错误处理框架

```go
// 统一错误类型
type PRError struct {
    Code       ErrorCode
    Message    string
    Details    string
    Suggestion string
    Retryable  bool
}

// 错误处理器
type ErrorHandler struct {
    logger *zap.Logger
}

func (h *ErrorHandler) Handle(err error) *PRError {
    switch {
    case errors.Is(err, ErrCLINotInstalled):
        return &PRError{
            Code:       ErrorCodeCLINotInstalled,
            Message:    "GitHub CLI (gh) is not installed",
            Suggestion: h.getInstallGuide("gh"),
            Retryable:  false,
        }
    case errors.Is(err, ErrPRAlreadyExists):
        return &PRError{
            Code:       ErrorCodePRExists,
            Message:    "Pull request already exists",
            Details:    extractPRURL(err),
            Retryable:  false,
        }
    // ... 更多错误类型
    }
}
```

### 4. TUI 组件设计

```go
// PR 预览模型
type PRPreviewModel struct {
    title      string
    body       string
    base       string
    draft      bool
    provider   provider.Info
    
    // UI 状态
    focusIndex int
    editing    bool
}

// 实现 tea.Model 接口
func (m PRPreviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // 处理用户输入，支持编辑各字段
}

func (m PRPreviewModel) View() string {
    // 渲染预览界面
    return fmt.Sprintf(`
┌─ Pull Request Preview ─────────────────┐
│ Provider: %s                           │
│ Base: %s                               │
│ Draft: %v                              │
├────────────────────────────────────────┤
│ Title:                                 │
│ %s                                     │
├────────────────────────────────────────┤
│ Description:                           │
│ %s                                     │
└────────────────────────────────────────┘

[Enter] Create PR  [e] Edit  [Esc] Cancel
`, m.provider.Provider, m.base, m.draft, m.title, m.body)
}
```

---

## 依赖与前置条件

### 技术依赖
- Go 1.21+ （使用新的错误处理特性）
- 现有的测试框架：testify, gomock
- TUI 框架：Bubble Tea (已在使用)

### 前置任务
1. 完成 Phase 1 的所有测试验证
2. 收集用户反馈和问题报告
3. 确定需要支持的 Provider 优先级

### 资源需求
- 开发人员：2 人
- 测试环境：GitHub、Gitea 实例
- CI/CD 环境更新

---

## 时间排期与里程碑

### Week 1-2: 基础设施改进
- [ ] Provider 检测架构重构
- [ ] 配置文件管理实现
- [ ] 错误处理框架搭建
- [ ] 单元测试补充

**验收标准**: 
- Provider 自动检测成功率 > 95%
- 配置文件操作 100% 原子性
- 错误处理覆盖所有已知场景

### Week 3-4: 功能扩展
- [ ] CLI 参数对齐实现
- [ ] Gitea 完整支持
- [ ] PR 模板支持
- [ ] Fork 工作流实现

**验收标准**:
- 所有 PRD 定义的参数正常工作
- Gitea PR 创建成功率 > 90%
- 支持主流 PR 模板格式

### Week 5: 用户体验优化
- [ ] TUI 交互组件开发
- [ ] 错误提示优化
- [ ] 安装引导完善
- [ ] 性能优化

**验收标准**:
- TUI 响应时间 < 100ms
- 用户满意度评分 > 4.5/5
- 无阻塞性 UX 问题

### Week 6: 测试与文档
- [ ] E2E 测试完善
- [ ] 性能测试执行
- [ ] 用户文档编写
- [ ] 发布准备

**验收标准**:
- 测试覆盖率 > 85%
- 文档覆盖所有功能
- 通过发布检查清单

---

## 风险与应对方案

### 风险 1: Provider API 变更
**描述**: GitHub/Gitea 等平台的 API 或 CLI 工具可能发生不兼容变更  
**影响**: 高  
**概率**: 中  
**应对方案**:
- 实现版本兼容性检查
- 维护多版本 CLI 支持矩阵
- 建立 API 变更监控机制

### 风险 2: 并发配置文件访问冲突
**描述**: 多个 catmit 实例同时修改配置文件可能导致数据损坏  
**影响**: 中  
**概率**: 低  
**应对方案**:
- 使用文件锁机制
- 实现配置文件备份
- 添加损坏检测和自动修复

### 风险 3: 网络不稳定导致探测失败
**描述**: HTTP 探测在网络环境差时可能频繁失败  
**影响**: 中  
**概率**: 高  
**应对方案**:
- 实现智能重试机制
- 提供离线模式支持
- 缓存探测结果

### 风险 4: 复杂 Git 工作流支持不足
**描述**: 某些企业使用的复杂 Git 工作流可能不被支持  
**影响**: 低  
**概率**: 中  
**应对方案**:
- 提供工作流配置选项
- 支持自定义脚本钩子
- 文档中说明限制

### 风险 5: 性能问题
**描述**: Provider 检测和 PR 创建可能在某些情况下很慢  
**影响**: 低  
**概率**: 中  
**应对方案**:
- 实现并行检测
- 添加进度指示器
- 优化关键路径

---

## 验收标准

### 功能验收
1. **Provider 检测**
   - [ ] 支持所有 PRD 定义的检测方式
   - [ ] 未知 Provider 有清晰的配置指引
   - [ ] HTTP 探测成功率 > 95%

2. **CLI 集成**
   - [ ] GitHub CLI 完整功能支持
   - [ ] Gitea CLI 基本功能支持
   - [ ] CLI 版本兼容性验证通过

3. **用户体验**
   - [ ] 错误信息清晰且可操作
   - [ ] TUI 交互流畅无卡顿
   - [ ] 配置过程简单直观

### 技术验收
1. **代码质量**
   - [ ] 测试覆盖率 > 85%
   - [ ] 无 golint 严重警告
   - [ ] 通过 race detector 检查

2. **性能指标**
   - [ ] Provider 检测 < 3s
   - [ ] PR 创建（不含网络）< 500ms
   - [ ] 内存使用 < 50MB

3. **兼容性**
   - [ ] 支持 Go 1.19+
   - [ ] 支持 macOS/Linux/Windows
   - [ ] 向后兼容 Phase 1 用法

---

## 后续规划（Phase 3）

1. **更多 Provider 支持**
   - GitLab 完整实现
   - Bitbucket 支持
   - 自托管 Git 服务支持

2. **高级功能**
   - 批量 PR 创建
   - PR 依赖管理
   - 自动冲突解决

3. **企业功能**
   - SAML/OAuth 集成
   - 审计日志
   - 合规性检查

4. **生态系统**
   - VS Code 插件
   - IntelliJ 插件
   - Web UI

---

## 附录：技术债务清单

从 Phase 1 继承的技术债务：

1. **命令结构不一致**
   - 当前：`--create-pr` 作为布尔标志
   - 目标：`--pr` 带子选项

2. **Provider 检测硬编码**
   - 当前：仅检查域名包含关键字
   - 目标：灵活的检测策略

3. **错误处理分散**
   - 当前：各模块独立处理错误
   - 目标：统一错误处理层

4. **测试 Mock 重复**
   - 当前：每个测试文件都定义 Mock
   - 目标：共享 Mock 定义

5. **配置管理缺失**
   - 当前：无持久化配置
   - 目标：完整配置管理系统

这些技术债务将在 Phase 2 中逐步解决，确保代码质量和可维护性。

---

**开发计划 Phase 2 生成完成** ✅
