# PR Workflow Optimization: Early PR Existence Check

## 目标概述

优化 `catmit --pr` 工作流，在生成提交信息之前先检查 PR 是否已存在，避免不必要的 LLM API 调用。

## 背景分析

### 当前工作流问题
1. **流程顺序**：生成提交信息 → 提交 → 推送 → 创建 PR
2. **资源浪费**：如果 PR 已存在，前面的 LLM 调用和用户交互都是无效的
3. **用户体验**：用户需要等待整个流程完成才能知道 PR 已存在

### 期望工作流
1. **优化顺序**：检查 PR 是否存在 → (如果不存在) → 生成提交信息 → 提交 → 推送 → 创建 PR
2. **快速反馈**：立即告知用户 PR 状态
3. **资源节约**：避免不必要的 LLM API 调用

## 技术调研

### 现有代码结构
```
internal/
├── workflow/
│   └── workflow.go          # 主工作流逻辑
├── pr/
│   ├── creator.go          # PR 创建接口和实现
│   ├── github_creator.go   # GitHub PR 创建
│   └── gitlab_creator.go   # GitLab PR 创建
├── ui/
│   └── main_model.go       # TUI 主模型
└── errors/
    └── errors.go           # 包含 ErrPRAlreadyExists
```

### 关键接口和方法
- `Workflow.Run()`: 主工作流入口
- `PRCreator.Create()`: PR 创建方法
- `MainModel.Update()`: TUI 状态更新
- `ErrPRAlreadyExists`: PR 已存在错误类型

## 详细任务清单

### 阶段 1：PR 存在性检查功能实现

#### 1.1 定义 PR 检查接口
**文件**: `internal/pr/creator.go`
```go
// 添加新方法到 Creator 接口
type Creator interface {
    Create(ctx context.Context, options CreateOptions) (string, error)
    // 新增：检查 PR 是否存在
    CheckExists(ctx context.Context, options CreateOptions) (exists bool, prURL string, err error)
}
```

**验收标准**：
- [ ] 接口定义清晰，参数和返回值合理
- [ ] 文档注释完整，说明各参数含义
- [ ] 考虑了错误处理场景

#### 1.2 实现 GitHub PR 检查
**文件**: `internal/pr/github_creator.go`
```go
func (g *githubCreator) CheckExists(ctx context.Context, options CreateOptions) (bool, string, error) {
    // 使用 gh pr list --head <branch> --json url,state
    // 解析结果，判断是否有 open 状态的 PR
}
```

**技术要点**：
- [ ] 使用 `gh pr list` 命令查询 PR
- [ ] 支持指定分支名查询
- [ ] 解析 JSON 输出获取 PR URL
- [ ] 处理多个 PR 的情况（返回第一个 open 的）
- [ ] 超时和错误处理

#### 1.3 实现 GitLab PR 检查
**文件**: `internal/pr/gitlab_creator.go`
```go
func (g *gitlabCreator) CheckExists(ctx context.Context, options CreateOptions) (bool, string, error) {
    // 使用 glab mr list --source-branch <branch>
    // 解析结果，判断是否有 open 状态的 MR
}
```

**技术要点**：
- [ ] 使用 `glab mr list` 命令查询 MR
- [ ] 解析输出格式（可能需要 --json 参数）
- [ ] 处理 GitLab 特有的 MR 状态
- [ ] 错误处理和降级策略

#### 1.4 实现通用 Git PR 检查（fallback）
**文件**: `internal/pr/git_creator.go`
```go
func (g *gitCreator) CheckExists(ctx context.Context, options CreateOptions) (bool, string, error) {
    // 返回 false, "", nil 因为纯 Git 不支持 PR
    return false, "", nil
}
```

### 阶段 2：工作流集成

#### 2.1 修改主工作流逻辑
**文件**: `internal/workflow/workflow.go`

**修改 Run 方法**：
```go
func (w *Workflow) Run(ctx context.Context) error {
    // 新增：如果启用了 PR 创建，先检查 PR 是否存在
    if w.config.CreatePR {
        exists, prURL, err := w.checkPRExists(ctx)
        if err != nil {
            return fmt.Errorf("checking PR existence: %w", err)
        }
        if exists {
            fmt.Printf("PR already exists: %s\n", prURL)
            return nil
        }
    }
    
    // 继续原有流程...
}
```

**新增辅助方法**：
```go
func (w *Workflow) checkPRExists(ctx context.Context) (bool, string, error) {
    // 1. 获取当前分支名
    // 2. 获取远程仓库信息
    // 3. 调用 PRCreator.CheckExists
    // 4. 返回结果
}
```

**测试场景**：
- [ ] PR 不存在时继续正常流程
- [ ] PR 存在时显示 URL 并退出
- [ ] 检查失败时的错误处理
- [ ] 不同 provider 的兼容性

#### 2.2 更新自动模式工作流
**文件**: `internal/workflow/workflow.go`

**修改 runAutomatic 方法**：
```go
func (w *Workflow) runAutomatic(ctx context.Context) error {
    // 在生成提交信息前检查 PR
    if w.config.CreatePR {
        // 复用 checkPRExists 逻辑
    }
    
    // 继续原有流程...
}
```

#### 2.3 更新交互模式工作流
**文件**: `internal/workflow/workflow.go`

**修改 runInteractive 方法**：
- [ ] 在 UI 初始化时传递 PR 检查需求
- [ ] 处理 PR 已存在的 UI 显示

### 阶段 3：UI 模型更新

#### 3.1 添加 PR 检查加载状态
**文件**: `internal/ui/main_model.go`

**新增加载阶段**：
```go
const (
    LoadingPRCheck      LoadingStage = "pr_check"
    LoadingChanges      LoadingStage = "changes"
    LoadingHistory      LoadingStage = "history"
    LoadingGeneration   LoadingStage = "generation"
)
```

**更新状态消息**：
```go
var loadingMessages = map[LoadingStage]string{
    LoadingPRCheck:    "Checking if PR already exists...",
    // 其他消息...
}
```

#### 3.2 处理 PR 存在场景
**修改 Update 方法**：
```go
case LoadingPhase:
    switch m.loadingStage {
    case LoadingPRCheck:
        // 处理 PR 检查结果
        if msg.PRExists {
            m.phase = DonePhase
            m.finalMessage = fmt.Sprintf("PR already exists: %s", msg.PRURL)
            return m, tea.Quit
        }
        // 继续下一阶段
    }
```

**新增消息类型**：
```go
type PRCheckResult struct {
    Exists bool
    URL    string
    Error  error
}
```

### 阶段 4：异步加载优化

#### 4.1 并行化 PR 检查
**文件**: `internal/ui/loading_phase.go`

```go
func (m MainModel) loadDataAsync() tea.Cmd {
    return func() tea.Msg {
        var wg sync.WaitGroup
        results := make(chan interface{}, 4)
        
        // PR 检查（如果需要）
        if m.options.CreatePR {
            wg.Add(1)
            go func() {
                defer wg.Done()
                // 执行 PR 检查
            }()
        }
        
        // 其他加载任务...
    }
}
```

### 阶段 5：错误处理和边界情况

#### 5.1 网络错误处理
- [ ] PR 检查超时处理（设置合理的超时时间）
- [ ] 网络不可用时的降级策略
- [ ] CLI 工具不可用时的处理

#### 5.2 权限和认证问题
- [ ] 未认证时的错误提示
- [ ] 权限不足时的处理
- [ ] Token 过期的处理

#### 5.3 特殊场景处理
- [ ] 多个 PR 存在时的处理
- [ ] PR 处于不同状态（draft, closed, merged）的处理
- [ ] 分支名包含特殊字符的处理

### 阶段 6：测试覆盖

#### 6.1 单元测试
**文件**: `internal/pr/*_test.go`
- [ ] `CheckExists` 方法的单元测试
- [ ] 模拟不同的命令输出
- [ ] 错误场景测试

#### 6.2 集成测试
**文件**: `internal/workflow/workflow_test.go`
- [ ] PR 存在时的工作流测试
- [ ] PR 不存在时的工作流测试
- [ ] 各种错误场景的测试

#### 6.3 E2E 测试
**文件**: `test/e2e/pr_test.go`
- [ ] 完整的 PR 检查流程测试
- [ ] 不同 provider 的测试
- [ ] UI 交互测试

### 阶段 7：文档更新

#### 7.1 更新 README
- [ ] 更新工作流说明
- [ ] 添加新行为的描述
- [ ] 更新示例

#### 7.2 更新 CLAUDE.md
- [ ] 添加新的架构说明
- [ ] 更新接口文档
- [ ] 添加测试指南

## 实现顺序建议

1. **第一步**：实现 PR 检查接口和具体实现（1.1-1.4）
2. **第二步**：在非交互模式下集成 PR 检查（2.1-2.2）
3. **第三步**：更新交互模式和 UI（2.3, 3.1-3.2）
4. **第四步**：优化性能和错误处理（4.1, 5.1-5.3）
5. **第五步**：完善测试覆盖（6.1-6.3）
6. **第六步**：更新文档（7.1-7.2）

## 验收标准

### 功能验收
- [ ] `catmit --pr` 在 PR 存在时立即显示 PR URL 并退出
- [ ] PR 不存在时按原流程执行
- [ ] 支持 GitHub 和 GitLab
- [ ] 错误处理优雅，提示信息清晰

### 性能验收
- [ ] PR 检查在 2 秒内完成
- [ ] 不影响原有功能的性能
- [ ] 网络请求有合理的超时设置

### 代码质量
- [ ] 测试覆盖率 > 85%
- [ ] 通过所有 lint 检查
- [ ] 接口设计符合现有架构
- [ ] 错误处理完善

## 风险和注意事项

1. **API 限制**：频繁的 PR 查询可能触发 API 限制
2. **兼容性**：不同版本的 gh/glab CLI 输出格式可能不同
3. **性能影响**：新增的检查步骤会略微增加启动时间
4. **用户体验**：需要清晰地向用户展示正在进行的操作

