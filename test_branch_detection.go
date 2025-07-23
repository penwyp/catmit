package main

import (
	"context"
	"fmt"
	"log"

	"github.com/penwyp/catmit/internal/git"
)

func main() {
	// Create a git runner
	runner := git.NewRunner(false)
	
	// Create a remote manager
	remoteManager := git.NewRemoteManager(runner)
	
	// Create context
	ctx := context.Background()
	
	// Get remotes
	remotes, err := remoteManager.GetRemotes(ctx)
	if err != nil {
		log.Fatalf("Failed to get remotes: %v", err)
	}
	
	if len(remotes) == 0 {
		log.Fatal("No remotes found")
	}
	
	// Select origin remote or first available remote
	selectedRemote, err := remoteManager.SelectRemote(remotes, "origin")
	if err != nil {
		log.Fatalf("Failed to select remote: %v", err)
	}
	
	// Try to detect the default branch
	detectedBranch, err := remoteManager.GetDefaultBranch(ctx, selectedRemote.Name)
	if err != nil {
		log.Fatalf("Failed to detect default branch: %v", err)
	}
	
	fmt.Printf("Detected default branch for remote '%s': %s\n", selectedRemote.Name, detectedBranch)
}