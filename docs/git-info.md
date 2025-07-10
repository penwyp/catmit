对于在Go项目中与Git交互，特别是读取`diff`这种复杂数据，**直接执行 `git` 的shell命令是目前最推荐、最可靠、也是最简单的方法**。

虽然存在一些原生的Go Git库（如 `go-git`），但它们在处理所有边缘情况、配置（如 `.git/config`, `~/.gitconfig`）、hooks和不同版本的Git时，其复杂性和潜在的兼容性问题远超直接调用已安装在用户系统上的 `git` 二进制文件。对于CLI工具来说，依赖用户已有的`git`是最稳妥的选择。

---

### 方法1：简单直接的执行 (`exec.Command`)

这是最基础的方式，可以直接获取 `git diff` 的输出。

根据你的需求(F-2)，你需要获取 **staged (已暂存)** 和 **unstaged (未暂存)** 的变更。这需要两个不同的命令：
1.  `git diff --staged`：获取暂存区的变更。
2.  `git diff`：获取工作目录中未暂存的变更。

**示例代码：**

```go
package main

import (
	"fmt"
	"log"
	"os/exec"
)

// getStagedDiff 获取暂存区的 diff
func getStagedDiff() (string, error) {
	// git diff --staged 会显示已添加到暂存区但尚未提交的更改
	cmd := exec.Command("git", "diff", "--staged")
	output, err := cmd.Output()
	if err != nil {
		// 注意：如果命令执行出错（例如不在git仓库里），err 会是 exec.ExitError 类型
		log.Printf("Error running git diff --staged: %v", err)
		return "", err
	}
	return string(output), nil
}

// getUnstagedDiff 获取未暂存的 diff
func getUnstagedDiff() (string, error) {
	// git diff 会显示工作目录中已修改但尚未暂存的更改
	cmd := exec.Command("git", "diff")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Error running git diff: %v", err)
		return "", err
	}
	return string(output), nil
}

func main() {
	stagedDiff, err := getStagedDiff()
	if err != nil {
		// 错误处理...
	}
	fmt.Println("--- STAGED CHANGES ---")
	fmt.Println(stagedDiff)

	unstagedDiff, err := getUnstagedDiff()
	if err != nil {
		// 错误处理...
	}
	fmt.Println("\n--- UNSTAGED CHANGES ---")
	fmt.Println(unstagedDiff)
}
```

**优点：**
*   代码非常简单直观。

**缺点：**
*   **极难测试！** 你的单元测试会依赖于本地是否安装了`git`，并且需要一个真实的Git仓库环境。这违背了TDD的原则。
*   错误处理比较粗糙，`stderr` 的信息被混入 `err` 对象，不易单独捕获。

---

### 方法2：更稳健的执行 (分离Stdout和Stderr)

为了更好地控制和诊断，我们可以分别捕获标准输出（stdout）和标准错误（stderr）。

```go
package main

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
)

// getDiffRobust 是一个更健壮的函数，可以运行任意 git diff 命令
func getDiffRobust(args ...string) (string, error) {
	// 将 'diff' 作为基础命令，并附加其他参数
	baseArgs := []string{"diff"}
	args = append(baseArgs, args...)

	cmd := exec.Command("git", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// 如果 git 命令本身返回非 0 退出码，err 就不为 nil
		// 我们可以从 stderr 中获取更详细的错误信息
		log.Printf("Git command failed. Stderr: %s", stderr.String())
		return "", fmt.Errorf("git command error: %w. Stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

func main() {
	stagedDiff, _ := getDiffRobust("--staged")
	fmt.Println("--- STAGED CHANGES ---")
	fmt.Println(stagedDiff)
	
	unstagedDiff, _ := getDiffRobust()
	fmt.Println("\n--- UNSTAGED CHANGES ---")
	fmt.Println(unstagedDiff)
}
```
**优点：**
*   错误信息更清晰，可以明确区分是程序执行错误还是 `git` 命令自身的错误。
*   代码结构更健壮。

**缺点：**
*   **依然难以测试。**

---

### 方法3：TDD友好的专业方法 (接口抽象)

这完美契合我们之前制定的TDD开发计划。我们将`git`的交互抽象到一个接口后面。这样，在测试时，我们可以提供一个`mock`实现，完全不需要执行真正的`git`命令。

**第1步：定义接口 (在 `collector/collector.go` 中)**

```go
// package collector
package collector

// GitCollector 定义了与 Git 仓库交互所需操作的契约。
// 这样设计使得我们可以轻松地在测试中 mock Git 的行为。
type GitCollector interface {
	GetStagedDiff() (string, error)
	GetUnstagedDiff() (string, error)
    GetLatestLogs(count int) (string, error) // 同样可以扩展到其他git操作
}
```

**第2步：创建真实实现 (在 `collector/shell_collector.go` 中)**

这个实现会真正地调用shell命令。

```go
// package collector
package collector

import (
	"bytes"
	"fmt"
	"os/exec"
)

// ShellGitCollector 是 GitCollector 接口的真实实现，它通过执行 shell 命令与 Git 交互。
type ShellGitCollector struct{}

// NewShellGitCollector 创建一个新的 ShellGitCollector 实例。
func NewShellGitCollector() *ShellGitCollector {
	return &ShellGitCollector{}
}

func (c *ShellGitCollector) runGitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git command `git %v` failed: %w, stderr: %s", args, err, stderr.String())
	}
	return stdout.String(), nil
}

func (c *ShellGitCollector) GetStagedDiff() (string, error) {
	return c.runGitCommand("diff", "--staged")
}

func (c *ShellGitCollector) GetUnstagedDiff() (string, error) {
	return c.runGitCommand("diff")
}

func (c *ShellGitCollector) GetLatestLogs(count int) (string, error) {
    arg := fmt.Sprintf("-n%d", count)
    return c.runGitCommand("log", arg, "--pretty=oneline")
}
```

**第3步：创建Mock实现 (在 `collector/collector_test.go` 中)**

这个`mock`用于单元测试，它实现了相同的接口，但返回预设的数据。

```go
// package collector
package collector

import "testing"

// MockGitCollector 是用于测试的 GitCollector 实现。
type MockGitCollector struct {
	StagedDiffContent   string
	StagedDiffError     error
	UnstagedDiffContent string
	UnstagedDiffError   error
    LogsContent         string
    LogsError           error
}

func (m *MockGitCollector) GetStagedDiff() (string, error) {
	return m.StagedDiffContent, m.StagedDiffError
}

func (m *MockGitCollector) GetUnstagedDiff() (string, error) {
	return m.UnstagedDiffContent, m.UnstagedDiffError
}

func (m *MockGitCollector) GetLatestLogs(count int) (string, error) {
    return m.LogsContent, m.LogsError
}

// 现在，你的业务逻辑测试可以这样写：
func TestYourBusinessLogic(t *testing.T) {
	// Arrange
	mockCollector := &MockGitCollector{
		StagedDiffContent: "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-hello\n+hello world",
		UnstagedDiffContent: "",
        LogsContent: "sha123 feat: existing feature",
	}

	// Act: 你的业务逻辑函数接收一个接口，而不是一个具体的类型
	// result := BuildPrompt(mockCollector, "en") // 假设有这么一个函数

	// Assert
	// ... 检查 result 是否正确 ...
}
```

**第4步：在主程序中使用**

你的主程序 (`cmd/root.go`) 将会依赖于 `GitCollector` 接口。

```go
// In cmd/root.go
// ...
import "github.com/penwyp/catmit/collector"
// ...

// 在你的Cobra命令执行函数中
func run(cmd *cobra.Command, args []string) {
    // 依赖注入：在这里创建真实的 collector
    var gitSvc collector.GitCollector = collector.NewShellGitCollector()
    
    // 将 gitSvc 传递给需要它的业务逻辑函数
    staged, err := gitSvc.GetStagedDiff()
    // ...
}
```

### **结论与推荐**

**请务必使用方法3 (TDD友好的专业方法)。**

这完全符合我们在开发计划中定义的TDD策略，特别是 `TEST-GIT-01` 和 `IMPL-GIT-01` 任务。它将外部依赖（`git` shell）与你的核心业务逻辑完全解耦，带来了巨大的好处：

1.  **可测试性 (Testability):** 你可以对 `collector` 包之外的所有逻辑进行快速、可靠的单元测试，而无需实际的Git仓库。
2.  **清晰的职责 (Separation of Concerns):** `ShellGitCollector` 的唯一职责就是与`git`命令交互。所有其他逻辑（如构建prompt）都不知道也不关心`diff`是如何获取的。
3.  **可扩展性 (Extensibility):** 如果未来你决定支持SVN或者其他版本控制系统，你只需要创建一个新的 `SvnCollector` 实现相同的接口，而无需改动任何上层业务逻辑。