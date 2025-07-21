package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/penwyp/catmit/squash"
	"github.com/penwyp/catmit/ui"
	"github.com/spf13/cobra"
)

// clientAdapter 适配 clientInterface 到 squash.ClientInterface
type clientAdapter struct {
	client clientInterface
}

func (a *clientAdapter) GenerateCommitMessage(ctx context.Context, prompt string) (string, error) {
	// squash 模块传入的是完整的 prompt，需要分成 system 和 user 部分
	// 简单起见，我们将整个 prompt 作为 user prompt，system prompt 为空
	return a.client.GetCommitMessage(ctx, "", prompt)
}

var (
	squashNoConfirm bool
	squashLang      string
	squashTimeout   int
)

var squashCmd = &cobra.Command{
	Use:   "squash",
	Short: "Consolidate multiple commit messages into one",
	Long: `Squash command helps you consolidate multiple commit messages into a single,
comprehensive commit message using LLM.

Opens your default editor to input commit messages (one per line).
The generated message will be automatically copied to your clipboard.`,
	Example: `  # Default mode (opens editor)
  catmit squash
  
  # No confirmation with Chinese output
  catmit squash --no-confirm --lang zh
  
  # Custom timeout
  catmit squash --timeout 60`,
	RunE: runSquash,
}

func init() {
	rootCmd.AddCommand(squashCmd)

	squashCmd.Flags().BoolVarP(&squashNoConfirm, "no-confirm", "n", false, "Skip confirmation and output directly")
	squashCmd.Flags().StringVarP(&squashLang, "lang", "l", "en", "Output language (en/zh)")
	squashCmd.Flags().IntVarP(&squashTimeout, "timeout", "t", 30, "Timeout in seconds")
}

func runSquash(cmd *cobra.Command, args []string) error {
	// 默认使用编辑器模式
	messages, err := getMessagesFromEditor()
	if err != nil {
		return fmt.Errorf("failed to get messages from editor: %w", err)
	}

	if len(messages) < 2 {
		return fmt.Errorf("at least 2 commit messages are required")
	}

	// 创建 squash 实例
	// 创建一个包装器，将 clientInterface 适配为 squash.ClientInterface
	llmClient := &clientAdapter{client: clientProvider()}

	squashInstance := squash.New(llmClient, squashLang)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(squashTimeout)*time.Second)
	defer cancel()

	// 如果是 no-confirm 模式，直接生成并输出
	if squashNoConfirm {
		result, err := squashInstance.Generate(ctx, messages)
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		fmt.Println(result)
		
		// 尝试复制到剪贴板
		if err := clipboard.WriteAll(result); err == nil {
			fmt.Fprintln(os.Stderr, "✓ Copied to clipboard")
		}
		
		return nil
	}

	// TUI 模式
	model := ui.NewSquashModel(squashInstance, messages)
	if err := model.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

func getMessagesFromEditor() ([]string, error) {
	// 获取默认编辑器
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// 创建临时文件
	tmpfile, err := os.CreateTemp("", "catmit-squash-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpfile.Name())

	// 写入提示信息
	_, err = tmpfile.WriteString("# Enter commit messages, one per line\n# Lines starting with # will be ignored\n# Save and exit when done\n\n")
	if err != nil {
		return nil, err
	}
	tmpfile.Close()

	// 打开编辑器
	editorCmd := exec.Command(editor, tmpfile.Name())
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return nil, err
	}

	// 读取文件内容
	content, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		return nil, err
	}

	// 解析内容
	var messages []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			messages = append(messages, line)
		}
	}

	return messages, scanner.Err()
}

