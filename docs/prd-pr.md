### catmit v2 — "一键 Commit/Push/PR" 设计稿（完整版）

> **版本**: v2.0
> **更新日期**: 2025-01-17
> **状态**: 设计阶段

---

## 1 · CLI 旗标与默认值

| 旗标                   | 缩写   | 默认            | 说明                                    |
| -------------------- | ---- | ------------- | ------------------------------------- |
| **Commit 域**         |      |               |                                       |
| `--stage-all`        |      | `true`        | 当暂存区为空时 `git add -A`                  |
| `--lang`             | `-l` | `en`          | 生成语种                                  |
| `--yes`              | `-y` | `false`       | 跳过人工确认                                |
| **Push 域**           |      |               |                                       |
| `--push`             | `-p` | `false`       | 成功 commit 后执行 `git push`              |
| `--push-remote`      | `-R` | `origin`      | `push` 目的 remote，仅 `--push` 有效        |
| **PR/MR 域**          |      |               |                                       |
| `--pr`               | `-c` | `false`       | 创建 PR / MR                            |
| `--pr-remote`        | `-P` | `origin`      | 用哪条 remote 判定 provider / owner / repo |
| `--pr-base`          | `-b` | provider 默认分支 | 目标分支                                  |
| `--pr-draft`         |      | `false`       | 草稿 PR/MR（若支持）                         |
| `--pr-provider`      |      | `auto`        | 手动指定 provider (github/gitea/auto)       |
| **运行控制**             |      |               |                                       |
| `--dry-run`          |      | `false`       | 只打印动作                                 |
| `--debug`            |      | `false`       | 详细日志                                  |
| `--timeout`          | `-t` | `20`          | 网络 / CLI 调用秒数                         |
| `--version` `--help` |      | –             | 信息                                    |

### 1.1 旗标依赖 / 互斥规则

```
            ┌─── --pr (-c) ───┐
            │                 ▼
            │        (--pr-remote)*
            │        (--pr-base)*
            │        (--pr-draft)*
            │        (--pr-provider)*
            │
(--push) ────┤
            │
            ▼
    (--push-remote)*
```

\* 带 \* 的旗标 **只有** 在父旗标开启时解析；否则仅提示 "flag ignored" 并继续执行（返回码 0）。

---

## 2 · Provider 检测

### 2.1 检测流程

当 `--pr-provider=auto` 或未指定时，按以下顺序检测：

1. **解析 git remote URL**
   - 支持 HTTPS: `https://github.com/owner/repo.git`
   - 支持 SSH: `git@github.com:owner/repo.git`
   - 支持带端口: `ssh://git@github.com:22/owner/repo.git`

2. **本地映射配置** `~/.config/catmit/providers.yaml`
   ```yaml
   # 自定义域名到 provider 的映射
   custom_mappings:
     github.company.com: github
     git.mycompany.net: gitea
   
   # 默认设置
   defaults:
     pr_base: main
     pr_draft: false
   ```

3. **域名模式匹配**
   - `*github.*` → `github`
   - `*gitea.*` → `gitea`
   - 注意：需要处理子域名情况，如 `git.github.com`

4. **HTTP/HTTPS 探测**（作为最后手段）
   - `HEAD https://host/` 或 `GET https://host/api/v1/version`
   - 检查响应头：
     - `X-GitHub-*` → `github`
     - `X-Gitea-Version` → `gitea`
   - 超时时间：3秒

5. **未知 provider 处理**
   - 报错并提示用户使用 `--pr-provider` 手动指定
   - 或添加映射到 `~/.config/catmit/providers.yaml`

### 2.2 Provider 信息结构

```json
{
  "provider": "github|gitea",
  "host": "github.com",
  "owner": "penwyp",
  "repo": "catmit",
  "forkOwner": "contributor",    // 若是 fork 仓库
  "branch": "feature-xyz",
  "defaultBase": "main"           // provider 的默认主分支
}
```

---

## 3 · CLI 工具管理

### 3.1 支持的 CLI 工具

| Provider | CLI 工具 | 最低版本要求 | 版本检测命令 | 认证检测命令 |
|----------|---------|------------|------------|------------|
| GitHub   | gh      | 2.0.0      | `gh --version` | `gh auth status` |
| Gitea    | tea     | 0.9.0      | `tea --version` | `tea login list` |

### 3.2 CLI 检测与处理

1. **检测顺序**
   - 检查 CLI 是否已安装（通过 `which` 或 `command -v`）
   - 检查版本是否满足最低要求
   - 检查认证状态（可选，仅在 `auth status` 命令中执行）

2. **未安装处理**
   ```
   Error: GitHub CLI (gh) is not installed
   
   To create pull requests on GitHub, please install 'gh':
   → macOS:    brew install gh
   → Windows:  winget install --id GitHub.cli
   → Linux:    See https://github.com/cli/cli#installation
   
   After installation, run: gh auth login
   ```

3. **版本过低处理**
   ```
   Error: GitHub CLI version too old
   
   Current version: 1.14.0
   Required version: >=2.0.0
   
   Please upgrade 'gh' to continue.
   ```

### 3.3 命令模板

| Provider | CLI 模板 | 变量说明 |
|----------|---------|---------|
| **GitHub** | `gh pr create --title '{title}' --body '{body}' {{--draft}} --base {base} --head {owner}:{branch}` | `{{--draft}}` 仅在 `--pr-draft=true` 时插入 |
| **Gitea** | `tea pr create --repo {owner}/{repo} --title '{title}' --description '{body}' --base {base} --head {forkOwner}:{branch}` | Gitea 不支持 draft PR |

> 注意：`{title}` 和 `{body}` 需要进行 shell 转义处理，防止注入攻击

---

## 4 · 认证管理

### 4.1 Auth Status 命令

新增 `catmit auth status` 命令，用于检查认证状态：

```bash
$ catmit auth status

Repository: github.com/penwyp/catmit
Provider: GitHub (auto-detected)

CLI Status:
┌─────────┬───────────┬─────────┬──────────────┬────────────────┐
│ CLI     │ Installed │ Version │ Min Required │ Authenticated  │
├─────────┼───────────┼─────────┼──────────────┼────────────────┤
│ gh      │ ✓         │ 2.40.1  │ 2.0.0        │ ✓              │
└─────────┴───────────┴─────────┴──────────────┴────────────────┘

✓ Ready to create pull requests
```

### 4.2 认证原则

- **不主动处理认证**：让用户通过各 CLI 工具自行完成认证
- **友好的错误提示**：当认证失败时，提供清晰的解决方案
- **支持多种认证方式**：
  - CLI 工具自带的认证（推荐）
  - 环境变量（如 `GITHUB_TOKEN`）
  - SSH 密钥（自动）

---

## 5 · 配置文件管理

### 5.1 配置文件位置

- 主配置：`~/.config/catmit/providers.yaml`
- 创建时机：首次运行需要 provider 检测时自动创建
- 权限：600（仅用户可读写）

### 5.2 默认配置模板

```yaml
# catmit provider configuration
# Generated at: 2025-01-17

# Custom domain to provider mappings
custom_mappings:
  # Example:
  # github.company.com: github
  # git.mycompany.net: gitea

# Default settings for PR creation
defaults:
  pr_base: main
  pr_draft: false

# CLI tool settings (optional)
cli:
  github:
    command: gh
    min_version: "2.0.0"
  gitea:
    command: tea
    min_version: "0.9.0"
```

---

## 6 · PR 创建流程

### 6.1 完整流程

```pseudo
flags ← parseFlags()

# STEP 1: Commit (existing flow)
if hasChanges:
    generateCommitMessage()
    if !flags.dryRun && (flags.yes || userConfirms):
        git.commit()

# STEP 2: Push
if flags.push || flags.pr:  # PR 隐式需要 push
    needsPush ← git.needsPush(flags.pushRemote)
    if needsPush:
        if flags.dryRun:
            print("Would push to {pushRemote}")
        else:
            git.push(flags.pushRemote)

# STEP 3: Create PR
if flags.pr:
    # 3.1 解析 provider
    if flags.prProvider != "auto":
        provider ← flags.prProvider
    else:
        provider ← detectProvider(flags.prRemote)
    
    # 3.2 检查 CLI 工具
    cliInfo ← checkCLI(provider)
    if !cliInfo.installed:
        fatal("CLI not installed", withInstallGuide)
    if !cliInfo.versionOK:
        fatal("CLI version too old", withUpgradeGuide)
    
    # 3.3 准备 PR 信息
    lastCommit ← git.lastCommit()
    prTitle ← lastCommit.subject
    prBody ← buildPRBody(lastCommit, flags)
    
    # 3.4 构建并执行命令
    cmd ← buildPRCommand(provider, prTitle, prBody, flags)
    if flags.dryRun:
        print(cmd)
    else:
        url ← exec(cmd)
        print("✓ Pull request created: {url}")
```

### 6.2 PR Body 生成策略

1. **基础内容**：使用最后一次 commit 的完整信息
2. **增强内容**（如果可用）：
   - 关联的 Issue（从 commit message 中提取 #123）
   - 变更摘要（文件数、增删行数）
   - PR 模板（如果仓库有 `.github/PULL_REQUEST_TEMPLATE.md`）

### 6.3 分支管理

1. **自动推送当前分支**：如果本地分支未推送，自动推送到 `--push-remote`
2. **Fork 工作流支持**：
   - 检测是否为 fork（比较 remote URL）
   - 自动使用正确的 head 格式：`forkOwner:branch`
3. **目标分支选择**：
   - 优先使用 `--pr-base` 指定的分支
   - 其次使用 provider 的默认分支（通过 API 或配置获取）
   - 最后回退到 `main` 或 `master`

---

## 7 · 错误处理

### 7.1 错误类型与用户提示

| 错误场景 | 用户提示 | 建议操作 |
|---------|---------|---------|
| 未推送分支 | `Branch 'feat-x' not pushed to remote 'origin'` | 添加 `-p` 参数或手动 push |
| CLI 未安装 | `GitHub CLI (gh) is not installed` | 提供安装指南链接 |
| CLI 未认证 | `GitHub CLI not authenticated` | 运行 `gh auth login` |
| Provider 未知 | `Cannot detect provider for 'git.company.com'` | 使用 `--pr-provider` 或配置映射 |
| PR 已存在 | `Pull request already exists: <url>` | 显示现有 PR 链接 |
| 网络超时 | `Network timeout (20s) when creating PR` | 建议增加 `--timeout` |
| 无权限 | `Permission denied: cannot create PR` | 检查仓库权限和认证 |

### 7.2 优雅降级

1. **CLI 降级**：当 CLI 工具不可用时，提示用户：
   - 手动创建 PR 的 Web URL
   - 必要的命令行步骤
   
2. **Provider 降级**：当无法自动检测时：
   - 提示可能的 provider 选项
   - 建议使用 `--pr-provider` 明确指定

---

## 8 · 使用示例

### 8.1 基础用法

```bash
# 最常用：commit → push → PR（全自动）
catmit -p -c

# 指定目标分支
catmit -p -c -b release/1.0

# 创建草稿 PR
catmit -p -c --pr-draft

# 手动指定 provider（当自动检测失败时）
catmit -p -c --pr-provider gitea
```

### 8.2 高级用法

```bash
# Fork 工作流：push 到个人 fork，PR 到上游
catmit -p -R fork -c -P upstream

# 仅创建 PR（已有 commit 和 push）
catmit -c

# 检查认证状态
catmit auth status

# 调试模式（查看将执行的命令）
catmit -p -c --dry-run --debug
```

### 8.3 配置自定义 provider

```bash
# 编辑配置文件
vim ~/.config/catmit/providers.yaml

# 添加映射
custom_mappings:
  github.company.com: github
  
# 使用
catmit -p -c  # 将自动识别 github.company.com 为 GitHub
```

---

## 9 · 实现计划

### Phase 1：MVP 版本（GitHub + Gitea）
- [x] 扩展现有 `--create-pr` 功能
- [ ] 实现 provider 自动检测
- [ ] 添加 CLI 版本检查
- [ ] 实现 `auth status` 命令
- [ ] 自动创建配置文件

### Phase 2：体验优化
- [ ] PR 模板支持
- [ ] 更智能的分支推送策略
- [ ] TUI 模式下的 PR 预览
- [ ] 批量 PR 创建支持

### Phase 3：生态扩展
- [ ] GitLab 支持
- [ ] Bitbucket 支持
- [ ] 自定义 PR 创建脚本
- [ ] Web UI 集成

---

## 10 · 技术决策记录

1. **为什么不自动下载 CLI？**
   - 安全考虑：避免下载执行未知二进制
   - 用户控制：让用户选择安装方式
   - 简化实现：减少维护成本

2. **为什么第一阶段只支持 GitHub 和 Gitea？**
   - GitHub：最广泛使用，gh CLI 成熟
   - Gitea：开源友好，适合私有部署
   - GitLab：CLI 工具较新，API 复杂，延后支持

3. **为什么使用 YAML 配置？**
   - 人类可读：便于手动编辑
   - 结构清晰：支持嵌套配置
   - 生态统一：Git 相关工具多用 YAML

---

> 本设计确保 **功能完整、体验流畅、错误友好**。通过渐进式实现，在保证核心功能稳定的同时，为未来扩展预留空间。
