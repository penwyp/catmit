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

	require.Contains(t, out, "Generate a conventional commit message in English")
	require.Contains(t, out, diff)
	require.Contains(t, out, "feat: add feature")
}

func TestBuilder_Build_Chinese(t *testing.T) {
	b := NewBuilder("zh", 0)
	diff := "diff --git a/file.txt b/file.txt\n+增加一行"

	out := b.Build("seed text", diff, nil, "test", []string{"file.txt"})
	require.Contains(t, out, "请用中文撰写")
	require.Contains(t, out, "seed text")
}

func TestBuilder_Truncation(t *testing.T) {
	longDiff := strings.Repeat("a", 500)
	b := NewBuilder("en", 100) // 限制 100 字节

	out := b.Build("", longDiff, nil, "dev", []string{})

	require.Contains(t, out, "(diff truncated)")
	// 确保最终 diff 部分被截断
	require.LessOrEqual(t, len(out), 600) // 结果不应太长
}
