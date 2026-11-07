package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Use terminal-cell widths (rather than byte counts) for Chinese help. Keep
// option names on their own lines so descriptions and defaults have room.
func renderTagCIHelp(cmd *cobra.Command, _ []string) {
	width := 80
	if columns, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && columns > 0 {
		width = columns
	}
	if out, ok := cmd.OutOrStdout().(*os.File); ok {
		if columns, _, err := term.GetSize(int(out.Fd())); err == nil {
			width = columns
		}
	}
	width = max(40, min(width, 100))
	write := func(text string) {
		for _, line := range strings.Split(text, "\n") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			content := strings.TrimPrefix(line, indent)
			wrapped := ansi.Wrap(content, width-len(indent), "")
			for _, part := range strings.Split(wrapped, "\n") {
				fmt.Fprintln(cmd.OutOrStdout(), indent+part)
			}
		}
	}
	intro, body, _ := strings.Cut(cmd.Long, "\n\n")
	write(intro + "\n\n用法\n  " + cmd.CommandPath() + " [选项] [-- 命令 [参数...]]")
	write("\n" + body)
	for _, group := range []struct {
		title string
		names []string
	}{
		{"版本与代码选项", []string{"version-order", "version-env", "provider", "checkout"}},
		{"运行选项", []string{"remote", "timeout", "timezone", "debug", "help"}},
		{"历史计算选项（仅影响 history）", []string{"bump", "initial", "pre", "release", "notimestamp"}},
	} {
		write("\n" + group.title)
		for _, name := range group.names {
			flag := cmd.Flags().Lookup(name)
			syntax := "--" + name
			if flag.Shorthand != "" {
				syntax = "-" + flag.Shorthand + ", " + syntax
			}
			switch flag.Value.Type() {
			case "string":
				syntax += " <值>"
			case "int":
				syntax += " <秒>"
			}
			write("  " + syntax + "\n    " + flag.Usage)
			if flag.DefValue != "" && flag.Value.Type() != "bool" {
				write("    默认：" + flag.DefValue)
			}
		}
	}
	write("\n示例\n" + cmd.Example)
}
