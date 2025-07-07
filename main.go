package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/penwyp/catmit/cmd"
)

// main 为 CLI 入口，调用 cmd.Execute。
func main() {
	// Setup a channel to listen for SIGINT and SIGTERM signals.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Create a context that will be used for the command execution.
	ctx := context.Background()

	// Start a goroutine to handle signals.
	go func() {
		sig := <-sigs
		log.Printf("Received signal: %s, exiting immediately.", sig)
		os.Exit(1)
	}()

	// Execute the root command.
	if err := cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// 使用 124 表示超时，符合 CLI 规范
			log.Println("Timeout exceeded")
			os.Exit(124)
		}
		// Since we are handling SIGINT/SIGTERM separately,
		// a Canceled error here is likely from another cause.
		if errors.Is(err, context.Canceled) {
			log.Println("Operation canceled")
			os.Exit(0)
		}
		// 标准化错误处理：避免 log.Fatalf，使用 log.Println + os.Exit
		log.Printf("catmit error: %v", err)
		os.Exit(1)
	}
}
