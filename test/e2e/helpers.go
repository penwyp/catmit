package e2e

import ()

// TestHelper provides utilities for E2E tests
type TestHelper struct {}

// RepoConfig holds configuration for creating a test repository
type RepoConfig struct {
	UserEmail     string
	UserName      string
	InitialCommit bool
	Remotes       map[string]string
	Branch        string
	SetUpstream   bool
}

// MockCLIScript creates a mock CLI script with custom behavior
type MockCLIScript struct {
	Name     string
	Commands map[string]MockCommand
}

// MockCommand represents a mocked CLI command response
type MockCommand struct {
	Output   string
	Error    string
	ExitCode int
}
