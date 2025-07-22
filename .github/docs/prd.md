# **Product Requirements Document (PRD)**

**Product Name:** **catmit**
**Author:** —
**Last Updated:**  6 July 2025
**Status:** Draft / v0.2 (feature-locked for MVP)

---

## 1  Executive Summary

`catmit` is a cross-platform Golang CLI/TUI that auto-generates high-quality, convention-compliant Git commit messages.
It analyzes recent repository history and pending changes, crafts a rich prompt, invokes DeepSeek’s LLM, and returns a polished commit message. Users can review, edit, or—via `--yes`—commit instantly. Progress feedback, multi-language output, and a configurable timeout make the tool production-friendly and scriptable.

---

## 2  Goals & Success Metrics

| Goal                                 | Metric                                         | Target (MVP)        |
| ------------------------------------ | ---------------------------------------------- | ------------------- |
| Reduce commit-message authoring time | Median time per commit (manual vs. catmit) | ↓ ≥ 80 %            |
| Improve message quality              | % messages passing Conventional Commit linter  | ≥ 95 %              |
| Adoption                             | Installs within first 30 days                  | 500+ internal users |
| Reliability                          | Failed LLM calls                               | < 1 %               |

---

## 3  Target Users

| Persona                | Pain Point                               | Value Delivered                    |
| ---------------------- | ---------------------------------------- | ---------------------------------- |
| **Backend Dev** (Go)   | Context switching to write good messages | 1-click, high-quality commits      |
| **Project Maintainer** | Inconsistent history                     | Enforced style & bilingual support |
| **Release Engineer**   | Changelog generation errors              | Always lint-compliant messages     |

---

## 4  User Stories

1. *As a developer* I want to run `catmit` without arguments to generate a commit message in English so I can commit faster.
2. *As a bilingual developer* I want to specify `--lang zh` so the message is produced in Chinese.
3. *As a power user* I want `--yes` to skip the UI so my pre-commit hook can auto-commit.
4. *As a cautious user* I want an interactive preview with Accept / Edit / Cancel so I stay in control.
5. *As a DevOps engineer* I need a `--timeout 10` flag so my CI job never hangs.

---

## 5  Functional Requirements

| ID   | Requirement                                                                                   |
| ---- | --------------------------------------------------------------------------------------------- |
| F-1  | Collect last **10** commits on current branch.                                                |
| F-2  | Collect staged **and** unstaged diffs, including untracked files.                             |
| F-3  | Accept optional seed text (`catmit "feat: seed"`).                                        |
| F-4  | Compose a single prompt containing: seed text, diff digest, log digest, language instruction. |
| F-5  | Read API key from `CATMIT_LLM_API_KEY`; invoke DeepSeek chat API.                               |
| F-6  | Support `--lang/-l` (ISO 639-1), default `en`.                                                |
| F-7  | Support `--timeout/-t` (int s), default `20`.                                                 |
| F-8  | Display progress stages with Charmbracelet spinner/progress.                                  |
| F-9  | Interactive TUI confirmation (Accept, Edit, Cancel).                                          |
| F-10 | `--yes/-y` bypasses TUI and commits immediately.                                              |
| F-11 | `--dry-run` prints message but never commits.                                                 |
| F-12 | On Accept/-y, run `git commit -m "<msg>"`.                                                    |
| F-13 | Return non-zero exit code on error or timeout.                                                |

---

## 6  Non-Functional Requirements

| Category          | Requirement                                                                            |
| ----------------- | -------------------------------------------------------------------------------------- |
| **Performance**   | End-to-end latency ≤ 3 s (p95) for 500-line diff; LLM call may consume most of budget. |
| **Reliability**   | Graceful degradation: on LLM failure, print error; never corrupt repo state.           |
| **Portability**   | Static binaries for macOS & Linux (amd64/arm64).                                       |
| **Security**      | Never log diff content unless `--verbose`; env vars masked.                            |
| **Usability**     | Zero config for happy path; rich TUI conforming to WCAG contrast guidelines.           |
| **Extensibility** | Pluggable LLM provider interface for v2.                                               |

---

## 7  CLI Specification

```text
catmit [OPTIONS] [SEED_TEXT]

Options:
  -l, --lang <code>       Commit message language (default "en")
  -t, --timeout <sec>     API timeout seconds (default 20)
  -y, --yes               Skip confirmation; commit immediately
      --dry-run           Show message, do not commit
      --verbose           Log extra diagnostics
  -h, --help              Show help
```

---

## 8  UX / TUI Flow

1. **Collecting diff…** ▶ progress bar
2. **Crafting prompt…** ▶ spinner
3. **Querying DeepSeek…** ▶ spinner (timeout governed)
4. **Review Screen**

```
┌ Commit Preview (EN) ─────────────────────────────┐
│ feat(api): validate JWT expiry in middleware     │
│                                                 │
│   • returns 401 on expired tokens               │
│   • adds unit tests (auth_test.go)              │
│                                                 │
│ [A] Accept  [E] Edit  [C] Cancel                │
└──────────────────────────────────────────────────┘
```

Editing opens an in-place textarea (Charmbracelet Textinput).

---

## 9  System Architecture

```
┌──────────┐   git(1)   ┌────────────┐   HTTP    ┌──────────────┐
│ catmit│──────────▶│ Prompt Core │──────────▶│ DeepSeek API │
└──────────┘            └────────────┘◀──────────┤ (chat)       │
   ▲   │spinner/progress│     │json             └──────────────┘
   │   ▼                ▼TUI
   │ git commit -m    review / edit
   └────────────── repository ────────────────
```

*Core packages*:

* `collector` (git ops)
* `prompt` (template & truncation)
* `client` (DeepSeek)
* `ui` (Bubble Tea models)
* `cmd` (cobra / flag parsing)

---

## 10  API Contract (DeepSeek v1)

```http
POST /v1/chat/completions
Headers: Authorization: Bearer $CATMIT_LLM_API_KEY
Body: {
  "model": "deepseek-chat",
  "messages": [{"role":"user","content": "<prompt>"}],
  "max_tokens": 128,
  "temperature": 0.7
}
```

*Timeout* controlled by Go `http.Client` with `context.WithTimeout`.

---

## 11  Error Handling

| Scenario         | Behaviour                                                 |
| ---------------- | --------------------------------------------------------- |
| No changes | Print "nothing to commit" warning, exit 0. (No staged, unstaged, or untracked files) |
| Timeout reached  | Cancel request, print “Timeout (N s) exceeded”, exit 124. |
| DeepSeek 4xx/5xx | Print status & body, exit 1.                              |
| Git commit fails | Propagate git’s exit code.                                |

---

## 12  Build & Deployment

| Step         | Details                                                    |
| ------------ | ---------------------------------------------------------- |
| CI           | Go 1.22, `go vet`, `golangci-lint run`, `go test ./...`.   |
| Build        | `CGO_ENABLED=0 go build -ldflags "-s -w" -o catmit .`  |
| Release      | GitHub Actions goreleaser → upload macOS & Linux binaries. |
| Homebrew Tap | `brew tap org/catmit && brew install catmit`.      |

---

## 13  Metrics & Monitoring

* CLI emits anonymized telemetry (opt-in): generation duration, success/fail.
* Track adoption via release download counts.
* Grafana dashboard fed by GitHub webhook & optional telemetry endpoint.

---

## 14  Risks & Mitigations

| Risk                             | Impact            | Mitigation                                             |
| -------------------------------- | ----------------- | ------------------------------------------------------ |
| DeepSeek outage                  | Blocking commits  | Offline fallback: open `$EDITOR`.                      |
| Large diffs overflow token limit | Truncated context | Use heuristic diff summarizer & aggressive truncation. |
| Users distrust auto-commit       | Low adoption      | Default interactive mode; `--yes` opt-in.              |
| Language quality for non-EN/zh   | Poor messages     | Add tests & prompt engineering; allow user edit step.  |

---

## 15  Out of Scope (MVP)

* Multiple LLM providers (OpenAI, Claude)
* Windows binaries
* Signed commits (`git commit -S`)
* Conventional-Commits check/generation beyond message text

---

## 16  Roadmap & Milestones

| Date           | Milestone           | Deliverable                                 |
| -------------- | ------------------- | ------------------------------------------- |
| **2025-07-15** | MVP code complete   | F-1 … F-13                                  |
| **07-18**      | Internal dog-food   | CLI installers, feedback survey             |
| **07-25**      | Public v0.1 release | Homebrew tap + docs                         |
| **Q3**         | v0.2                | Provider plug-in framework, Windows support |
| **Q4**         | v1.0                | Signed commits, enterprise telemetry        |

---

## 17  Acceptance Criteria

1. Running `catmit` with staged changes produces a valid, lint-passing English message in ≤ 3 s (p95) on a 500-line diff.
2. `catmit -l zh -y` commits Chinese message with no confirmation UI.
3. Timeout flag aborts request after specified seconds with exit 124.
4. Tool exits gracefully when no diff is present.
5. Binaries run on macOS 13/14 (arm64) and Ubuntu 22.04 (amd64).