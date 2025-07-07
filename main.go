package main

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/penwyp/catmit/cmd"
)

// main 为 CLI 入口，调用 cmd.Execute。
func main() {
	if err := cmd.Execute(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// 使用 124 表示超时，符合 CLI 规范
			log.Println("Timeout exceeded")
			os.Exit(124)
		}
		// 标准化错误处理：避免 log.Fatalf，使用 log.Println + os.Exit
		log.Printf("catmit error: %v", err)
		os.Exit(1)
	}

// fgwgr
}
