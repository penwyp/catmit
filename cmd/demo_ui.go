//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/catmit/internal/ui"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run demo_ui.go [review|squash] [--append-mode]")
		os.Exit(1)
	}

	appendMode := false
	for _, arg := range os.Args {
		if arg == "--append-mode" {
			appendMode = true
			break
		}
	}

	switch os.Args[1] {
	case "review":
		demoReview(appendMode)
	case "squash":
		fmt.Println("Squash demo requires actual squash instance")
	default:
		fmt.Println("Unknown mode:", os.Args[1])
		os.Exit(1)
	}
}

func demoReview(appendMode bool) {
	// Sample commit message
	message := `feat(ui): implement unified TUI framework

- Add BaseModel for consistent keyboard navigation
- Support arrow keys and Esc key globally
- Implement append mode for non-clearing console
- Simplify UI implementation across all models`

	model := ui.NewSimpleReviewModel(message, "en", appendMode)

	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Check the result
	if m, ok := finalModel.(*ui.SimpleReviewModel); ok {
		done, decision, finalMsg := m.IsDone()
		if done {
			switch decision {
			case ui.DecisionAccept:
				fmt.Printf("\n✓ Accepted commit message:\n%s\n", finalMsg)
			case ui.DecisionCancel:
				fmt.Println("\n✗ Cancelled")
			}
		}
	}
}
