package cli

import "context"

// Version represents a semantic version structure
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease string
	Build      string
}

// CLIStatus holds the status information of a CLI tool
type CLIStatus struct {
	Name          string // CLI name (e.g., gh, tea)
	Installed     bool   // Whether the CLI is installed
	Version       string // Version number
	Authenticated bool   // Whether the CLI is authenticated
	Username      string // Authenticated username
}

// CommandRunner is the interface for command execution
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}
