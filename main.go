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
	// 使用 signal.NotifyContext 创建可取消的 Context；
	// 当收到 Ctrl+C (SIGINT) 或 SIGTERM 时，ctx.Done() 会被触发。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop() // 释放资源

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
