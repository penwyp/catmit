package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/penwyp/catmit/cmd"
	catmitErrors "github.com/penwyp/catmit/internal/errors"
)

// main is the CLI entry point, calls cmd.Execute.
func main() {
	// Use signal.NotifyContext to create a cancellable Context;
	// When Ctrl+C (SIGINT) or SIGTERM is received, ctx.Done() will be triggered.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop() // Release resources

	// Execute the root command.
	if err := cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// Use 124 to indicate timeout, following CLI conventions
			log.Println("Timeout exceeded")
			os.Exit(124)
		}
		// Since we are handling SIGINT/SIGTERM separately,
		// a Canceled error here is likely from another cause.
		if errors.Is(err, context.Canceled) {
			log.Println("Operation canceled")
			os.Exit(0)
		}

		// Check for custom timeout errors
		if catmitErrors.GetType(err) == catmitErrors.ErrTypeTimeout {
			log.Printf("catmit error: %v", err)
			os.Exit(124)
		}

		// Standardized error handling: avoid log.Fatalf, use log.Println + os.Exit
		log.Printf("catmit error: %v", err)
		os.Exit(1)
	}
}
