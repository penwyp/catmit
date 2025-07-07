package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuilder_Build_English(t *testing.T) {
	b := NewBuilder("en", 0)
	diff := "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")"
	commits := []string{"feat: add feature", "fix: bug"}

	out := b.Build("", diff, commits, "test", []string{"main.go"})

	require.Contains(t, out, "MUST be in English")
	require.Contains(t, out, diff)
	require.Contains(t, out, "feat: add feature")
}

func TestBuilder_Build_Chinese(t *testing.T) {
	b := NewBuilder("zh", 0)
	diff := "diff --git a/file.txt b/file.txt\n+增加一行"

	out := b.Build("seed text", diff, nil, "test", []string{"file.txt"})
	require.Contains(t, out, "MUST be in Chinese")
	require.Contains(t, out, "seed text")
}

func TestBuilder_Truncation(t *testing.T) {
	longDiff := strings.Repeat("a", 500)
	b := NewBuilder("en", 100) // 限制 100 字节

	out := b.Build("", longDiff, nil, "dev", []string{})

	require.Contains(t, out, "(diff truncated)")
	// 确保最终 diff 部分被截断
	require.LessOrEqual(t, len(out), 2000) // 结果不应太长（系统提示词更长）
}

// 新增测试 - 测试系统提示词和用户提示词分离
func TestBuilder_BuildSystemPrompt_English(t *testing.T) {
	b := NewBuilder("en", 0)
	systemPrompt := b.BuildSystemPrompt()

	// 验证系统提示词包含关键元素
	require.Contains(t, systemPrompt, "expert software engineer")
	require.Contains(t, systemPrompt, "Conventional Commits")
	require.Contains(t, systemPrompt, "MUST be in English")
	require.Contains(t, systemPrompt, "feat, fix, refactor")
	require.Contains(t, systemPrompt, "EXAMPLES")
	require.Contains(t, systemPrompt, "raw commit message text")
}

func TestBuilder_BuildSystemPrompt_Chinese(t *testing.T) {
	b := NewBuilder("zh", 0)
	systemPrompt := b.BuildSystemPrompt()

	// 验证中文语言指令
	require.Contains(t, systemPrompt, "MUST be in Chinese")
	// 其他元素仍然存在
	require.Contains(t, systemPrompt, "expert software engineer")
	require.Contains(t, systemPrompt, "Conventional Commits")
}

func TestBuilder_BuildUserPrompt(t *testing.T) {
	b := NewBuilder("en", 0)
	diff := "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")"
	commits := []string{"feat: add feature", "fix: bug"}

	userPrompt := b.BuildUserPrompt("seed text", diff, commits, "test", []string{"main.go"})

	// 验证用户提示词包含数据元素
	require.Contains(t, userPrompt, "Seed: seed text")
	require.Contains(t, userPrompt, "Branch: test")
	require.Contains(t, userPrompt, "Changed files: main.go")
	require.Contains(t, userPrompt, "Recent commits:")
	require.Contains(t, userPrompt, "feat: add feature")
	require.Contains(t, userPrompt, "Git diff:")
	require.Contains(t, userPrompt, diff)
}

func TestBuilder_BuildUserPrompt_WithTruncation(t *testing.T) {
	b := NewBuilder("en", 100) // 限制 100 字节
	longDiff := strings.Repeat("a", 500)

	userPrompt := b.BuildUserPrompt("", longDiff, nil, "", []string{})

	// 验证 diff 截断
	require.Contains(t, userPrompt, "(diff truncated)")
	// 确保没有包含完整的原始 diff
	require.NotContains(t, userPrompt, strings.Repeat("a", 300))
}

func TestBuilder_BuildUserPrompt_EmptyContext(t *testing.T) {
	b := NewBuilder("en", 0)
	
	userPrompt := b.BuildUserPrompt("", "", nil, "", []string{})

	// 验证空上下文的处理
	require.Equal(t, "No changes detected.", userPrompt)
}

// 验证向后兼容性
func TestBuilder_Build_BackwardCompatibility(t *testing.T) {
	b := NewBuilder("en", 0)
	diff := "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")"
	commits := []string{"feat: add feature"}

	// 旧方法仍然可用
	oldResult := b.Build("seed", diff, commits, "test", []string{"main.go"})
	
	// 新方法的组合结果应该与旧方法类似
	systemPrompt := b.BuildSystemPrompt()
	userPrompt := b.BuildUserPrompt("seed", diff, commits, "test", []string{"main.go"})
	newResult := systemPrompt + "\n\n" + userPrompt
	
	// 验证结果类似（内容可能不完全一致，但都包含关键信息）
	require.Contains(t, oldResult, "MUST be in English")
	require.Contains(t, oldResult, "seed")
	require.Contains(t, oldResult, diff)
	require.Contains(t, newResult, "expert software engineer")
	require.Contains(t, newResult, "seed")
	require.Contains(t, newResult, diff)
}
