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
	// Create context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), 
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// 使用 124 表示超时，符合 CLI 规范
			log.Println("Timeout exceeded")
			os.Exit(124)
		}
		if errors.Is(err, context.Canceled) {
			// Graceful shutdown
			log.Println("Operation canceled")
			os.Exit(0)
		}
		// 标准化错误处理：避免 log.Fatalf，使用 log.Println + os.Exit
		log.Printf("catmit error: %v", err)
		os.Exit(1)
	}
}
