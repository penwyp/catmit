### catmit v2 — “一键 Commit/Push/PR” 设计稿（完整版）

> **重点补充**：
>
> 1. *Provider* 检测链路与命令模板
> 2. 旗标间 **依赖 / 互斥** 行为
> 3. 全量边缘与错误处理

---

## 1 · CLI 旗标与默认

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
            │
(--push) ────┤
            │
            ▼
    (--push-remote)*
```

\* 带 \* 的旗标 **只有** 在父旗标开启时解析；否则仅提示 “flag ignored” 并继续执行（返回码 0）。

---

## 2 · Provider 检测

### 2.1 检测顺序

1. **本地映射** `~/.config/catmit/providers.yaml`

   ```yaml
   github: [github.acme.com]
   gitlab: [gitlab-internal.local]
   gitea:  [git.pingcap.net]
   ```
2. **域名正则**

   * `*github.*`   → `github`
   * `*gitlab.*`   → `gitlab`
   * `*gitea.*`   → `gitea`
3. **HTTP / SSH title probe**

   * `HEAD https://host/`，若响应头含 `X-Gitea-Version` → `gitea`
   * `X-Gitlab-…`                               → `gitlab`
4. **未知** → 报错：
   `ERR_UNKNOWN_PROVIDER (host) — add host to ~/.config/catmit/providers.yaml`

### 2.2 结果结构

```json
{
  "provider": "github|gitlab|gitea",
  "host": "git.xxx.com",
  "owner": "pingkai",
  "repo": "tms",
  "forkOwner": "yunpeng.wu",    // 若不同
  "branch": "feature-xyz"
}
```

---

## 3 · 命令模板 & REST 回退

> 变量： `{t}` 标题，`{b}` 正文，`{branch}` 当前分支，`{base}` 目标分支
> `{{draft}}` 在 `--pr-draft`=true 且 provider 支持时替换为对应 CLI 旗标

| Provider   | CLI 检测 (顺序)      | CLI 模板                                                                                                            | REST Fallback                              |
| ---------- | ---------------- | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------ |
| **GitHub** | `gh --version`   | `gh pr create --title '{t}' --body '{b}' {{--draft}} --base {base} --head {owner}:{branch}`                       | `POST /repos/{owner}/{repo}/pulls` REST v3 |
| **GitLab** | `glab --version` | `glab mr create --title '{t}' --description '{b}' {{--draft}} --source-branch {branch} --target-branch {base}`    | `POST /projects/:id/merge_requests`        |
| **Gitea**  | `tea --version`  | `tea pr create --repo {owner}/{repo} --title '{t}' --description '{b}' --base {base} --head {forkOwner}:{branch}` | `POST /repos/{owner}/{repo}/pulls`         |

*CLI 缺失*   → 友好报错。
*CLI 已安装但* `login` 状态异常 → 进入 REST（含 token 提示）。

---

## 4 · 伪代码（详细版）

```pseudo
flags ← parseFlags()

branch ← git.currentBranch()
title  ← git.lastCommitTitle()
body   ← git.lastCommitBody(full=true)   # %B 保留换行

# STEP A. Push
if flags.push:
    ok ← git.push(flags.pushRemote, branch)
    if !ok: fatal("push failed – check remote or use --debug")

# STEP B. PR / MR
if flags.pr:
    meta ← git.parseRemote(flags.prRemote)
    provider ← detectProvider(meta.host)      # section 2

    ensureBranchIsPushed(meta.remote, branch) # fatal if not pushed & --push=false
    ensureCLIOrRESTReady(provider)            # may download CLI

    cmd ← buildCmd(provider, meta,
                   title, body,
                   base = flags.prBase or providerDefaultBase(provider, meta),
                   draft = flags.prDraft)
    if flags.dryRun:
        print(cmd); exit 0

    out, err ← exec(cmd, timeout=flags.timeout)
    if err:                                     # CLI failed
        url, err2 ← invokeREST(provider, meta, title, body, ...)
        if err2: fatal(err2)
        print(url)
    else:
        print(extractURL(out))
```

---

## 5 · 主要错误情境 & 友好消息

| 场景                    | 反馈示例                                                                                       |
| --------------------- | ------------------------------------------------------------------------------------------ |
| 未 push 分支且未给 `--push` | `branch 'feat/x' not on remote; use -p/--push or push manually`                            |
| CLI 未登录               | `gh auth not found – run 'gh auth login' or set GITHUB_TOKEN`                              |
| REST 401              | `token unauthorized (401). set correct <PROVIDER>_*_TOKEN env`                             |
| REST 5xx              | `provider gitlab returned 500 – retry later or contact admin`                              |
| Unknown provider      | `cannot detect provider for host git.dev.local; add it to ~/.config/catmit/providers.yaml` |
| Draft flag on Gitea   | `INFO: gitea does not support draft; creating regular PR`                                  |
| Timeout               | `network timeout (20s). increase with --timeout`                                           |
| Download CLI fail     | `auto-download gh failed (offline?). falling back to REST`                                 |

---

## 6 · 调用示例

```bash
# 最常用：commit → push(origin) → PR(origin)
catmit -p -c

# push 到 personal，PR 发到 upstream
catmit -p -R personal -c -P upstream

# 只推 origin，不建 PR
catmit -p

# 创建 GitLab 草稿 MR，目标 release-1.1
catmit -c --pr-draft -b release-1.1
```

---

> 以上设计确保 **功能分层、旗标自洽、错误友好**。后续如需支持 Bitbucket、Azure DevOps，只需在 **Provider 检测表 + cmd 模板** 中新增条目即可横向扩展。

