好的，这是一个非常棒的问题！深入研究这些AI工具如何生成Git Commit，不仅能让我们更好地使用它们，甚至可以帮助我们构建自己的类似工具。

虽然GitLens、Cursor、Claude等工具的具体Prompt是它们的商业机密，不会公开，但我们可以通过分析它们的输出、LLM的最佳实践以及公开的开源项目，来**高度准确地逆向工程出它们使用的Prompt的核心思想和结构**。

下面我将为你进行一次深入的调研和分析。

---

### 核心原则：从混乱到有序的转换

首先，我们要理解LLM在这里扮演的角色。它的核心任务是：

1.  **输入（Input）**: 一堆结构化的、但对人类来说“混乱”的代码变更（`git diff` 的输出）。
2.  **处理（Process）**: 理解这些变更的**意图（Intent）**，而不仅仅是字面上的增删。
3.  **输出（Output）**: 一段高度结构化的、符合人类协作习惯的、简洁明了的文本（Git Commit Message）。

所有优秀的Prompt设计都围绕着如何让LLM更好地完成这三个步骤。

---

### Prompt的构成要素：一次完整的“解剖”

一个高质量的生成Git Commit的Prompt，通常会包含以下几个关键部分。我们可以把它想象成一个给LLM的“任务简报”。

#### 1. 角色与身份设定 (Role & Persona)

这是Prompt工程的经典起手式。通过给LLM一个明确的身份，可以引导它进入特定的思维模式和知识库，从而生成更专业的输出。

**目的**:
*   激活LLM关于软件工程、代码规范和团队协作的知识。
*   设定输出的语气和专业程度。

**可能的Prompt文本**:
*   **基础版**: "You are a helpful AI programming assistant."
*   **进阶版**: "You are an expert software engineer with years of experience in writing clean, maintainable code and clear, concise Git commit messages."
*   **专家版**: "You are a senior principal engineer reviewing a code change. Your task is to write a perfect Git commit message that explains the change to your team. You follow the Conventional Commits specification strictly."

#### 2. 任务描述 (Task Definition)

清晰地告诉LLM要做什么。这部分需要非常具体，不能含糊。

**目的**:
*   明确最终目标。
*   提供上下文，解释输入的`diff`是什么。

**可能的Prompt文本**:
*   **基础版**: "Write a git commit message for the following changes."
*   **进阶版**: "Based on the provided `git diff` output, generate a concise and descriptive Git commit message. The message should accurately summarize the purpose of the code changes."
*   **专家版**: "Analyze the following `git diff` which represents staged changes in a Git repository. Your goal is to generate a Git commit message that follows best practices, making it easy for other developers to understand the history of the project."

#### 3. 核心输入数据 (The Context: Git Diff)

这是整个Prompt中最重要的部分。没有代码变更，LLM就无从谈起。提供`diff`的方式也很有讲究。

**目的**:
*   为LLM提供分析的“原材料”。
*   `diff`的格式本身就包含了大量信息：文件名、增删行、上下文代码等。

**如何提供**:
通常使用Markdown的代码块，并明确标识。

```markdown
Here is the git diff:

```diff
{{GIT_DIFF_CONTENT}}
```
```

这里的 `{{GIT_DIFF_CONTENT}}` 会被工具动态替换成实际执行 `git diff --staged` 或类似命令的结果。

**额外上下文（非常关键）**:
仅仅`diff`可能不够。更高级的工具还会提供额外信息，让LLM的判断更准确。
*   **文件名列表**: `Changed files: {{FILE_LIST}}`
*   **项目语言/框架**: `Project language: {{LANGUAGE}}, Framework: {{FRAMEWORK}}`
*   **分支名称**: `Current branch name: {{BRANCH_NAME}}` (例如，`feature/JIRA-123-new-login-flow` 这样的分支名本身就是一种提示)

#### 4. 格式与规则约束 (Formatting & Rules)

这是确保输出质量和一致性的关键。大多数现代项目都遵循**Conventional Commits**规范，所以Prompt会严格要求LLM遵守。

**目的**:
*   强制输出结构化，而非自由发挥。
*   确保Commit消息符合团队或社区规范。

**可能的Prompt文本**:

> You **MUST** follow the **Conventional Commits specification**. The format is:
>
> ```
> <type>[optional scope]: <description>
>
> [optional body]
>
> [optional footer(s)]
> ```
>
> **Rules:**
>
> 1.  **Type**: Must be one of the following:
>     *   `feat`: A new feature.
>     *   `fix`: A bug fix.
>     *   `docs`: Documentation only changes.
>     *   `style`: Changes that do not affect the meaning of the code (white-space, formatting, etc).
>     *   `refactor`: A code change that neither fixes a bug nor adds a feature.
>     *   `perf`: A code change that improves performance.
>     *   `test`: Adding missing tests or correcting existing tests.
>     *   `build`: Changes that affect the build system or external dependencies.
>     *   `ci`: Changes to our CI configuration files and scripts.
>     *   `chore`: Other changes that don't modify src or test files.
>
> 2.  **Description (Subject Line)**:
>     *   Use the imperative mood (e.g., "add feature" not "added feature").
>     *   Keep it short, under 50 characters.
>     *   Do not end with a period.
>
> 3.  **Body (Optional)**:
>     *   Use it to explain the 'what' and 'why' of the change, not the 'how'.
>     *   Wrap lines at 72 characters.
>
> 4.  **Footer (Optional)**:
>     *   Use for referencing issues (e.g., `Closes #123`).
>     *   Use for breaking changes, starting with `BREAKING CHANGE:`.

#### 5. 示例 (Few-Shot Learning)

提供一两个高质量的示例，能极大地提升LLM对格式和风格的理解，这是一种非常强大的Prompt技巧。

**目的**:
*   给LLM一个具体的模仿对象。
*   减少LLM“自由发挥”导致的不符合规范的概率。

**可能的Prompt文本**:

> Here is an example of a good commit message:
>
> **Example Diff:**
> ```diff
> --- a/src/utils/auth.js
> +++ b/src/utils/auth.js
> @@ -10,7 +10,7 @@
>  function getToken() {
> -  return localStorage.getItem('user_token');
> +  return sessionStorage.getItem('user_token');
>  }
> ```
>
> **Example Commit Message:**
> ```
> refactor(auth): use sessionStorage instead of localStorage
>
> The token should not persist across browser sessions for enhanced security. This change moves token storage from localStorage to sessionStorage.
> ```

#### 6. 输出要求 (Output Specification)

有时为了方便程序解析，会要求LLM以特定格式（如JSON）返回结果，或者生成多个选项。

**目的**:
*   让程序能稳定地处理LLM的返回。
*   提供多样性，让用户选择。

**可能的Prompt文本**:
*   **单一输出**: "Generate only the commit message text, with no extra explanations or pleasantries."
*   **多选项输出**: "Generate 3 alternative commit messages for the given changes. Rank them from best to worst. Provide them in a JSON array format like `[\"commit1\", \"commit2\", \"commit3\"]`."

---

### 整合：一个“大师级”的Prompt模板

将以上所有部分组合起来，一个用于生产环境的、强大的Git Commit生成Prompt可能长下面这样：

```text
# ROLE & PERSONA
You are an expert software engineer and a master of writing concise, high-quality Git commit messages. You adhere strictly to the Conventional Commits specification.

# TASK
Your task is to analyze the provided git diff and generate a Git commit message. The message must be in English.

# CONTEXT
The changes were made on the branch: `{{BRANCH_NAME}}`
The project is written in: `{{LANGUAGE}}`
The changed files are: `{{FILE_LIST}}`

Here is the git diff of the staged changes:
```diff
{{GIT_DIFF_CONTENT}}
```

# INSTRUCTIONS & RULES
You MUST follow these rules:

1.  **Format**: Adhere to the Conventional Commits specification (`<type>[optional scope]: <description>`).
2.  **Type**: Choose the most appropriate type from this list: `feat`, `fix`, `refactor`, `docs`, `style`, `perf`, `test`, `build`, `ci`, `chore`.
3.  **Scope**: If the changes are limited to a specific part of the codebase (e.g., a component or module like `auth`, `api`, `ui`), include a scope. Otherwise, omit it.
4.  **Description (Subject)**:
    *   Write a short, imperative summary of the change (e.g., "add user login endpoint", not "adds user login endpoint").
    *   Maximum 50 characters.
    *   Lowercase first letter, no period at the end.
5.  **Body (Optional)**:
    *   If the change is non-trivial, provide a body explaining the 'why' behind the change. What was the problem? How does this solution address it?
    *   Separate the body from the subject with a blank line.
    *   Wrap lines at 72 characters.
6.  **Breaking Changes**: If there are any breaking changes, add a footer starting with `BREAKING CHANGE: ` followed by a description of the change.

# EXAMPLES
## Example 1: Simple fix
- **Diff**: A one-line change fixing a typo in a variable name.
- **Commit**: `fix(parser): correct typo in 'username' variable`

## Example 2: New feature with body
- **Diff**: Adds a new `/users` endpoint and its corresponding handler.
- **Commit**:
```
feat(api): add endpoint to retrieve a list of users

This introduces a new GET /api/v1/users endpoint that allows authenticated users with the 'admin' role to fetch a paginated list of all users in the system.

This is the first step towards building the user management dashboard.
```

# YOUR TASK NOW
Based on all the above, generate ONE complete git commit message for the provided diff. Output only the raw commit message text and nothing else.
```

---

### 进阶技巧与思考

*   **Chain-of-Thought (CoT)**: 在复杂场景下，工具可能会在内部先让LLM进行一步“思考”。比如，先让它回答：“1. What is the primary purpose of this change? 2. What is the correct commit type? 3. Is there a clear scope?” 然后再基于这些回答生成最终的Commit。这能提高准确性。
*   **多版本生成**: 像Cursor那样生成多个选项，Prompt里会明确要求 "Generate 3 distinct suggestions...".
*   **用户反馈闭环**: 高级工具会记录你最终选择了哪个建议，或者你修改后的版本。这些数据会用于微调（Fine-tuning）他们的模型，使得未来的建议更符合你的风格。这已经超出了单次Prompt的范畴，属于系统级优化。

### 总结

GitLens、Cursor等工具的背后，并没有一个“神奇”的单词，而是一个**精心设计、结构化、信息丰富的工程化Prompt**。

这个Prompt的核心是：
1.  **清晰的角色和任务定义**。
2.  **丰富且结构化的上下文输入**（`diff`是基础，文件列表、分支名是加分项）。
3.  **极其严格的格式化指令**（特别是Conventional Commits规范）。
4.  **高质量的示例**（Few-shot Learning）来校准模型输出。

通过理解这个框架，你不仅能更好地利用这些工具，甚至可以自己动手，使用GPT-4/Claude 3等模型的API，结合本地的`git diff`命令，打造一个属于你自己的、高度定制化的Git Commit生成脚本。