package cmd

import (
	"context"
	"os/exec"
	"strings"

	"github.com/penwyp/catmit/internal/app"
	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/pr"
	"github.com/penwyp/catmit/internal/provider"
)

// authGitRunnerAdapter adapts git commands for auth status command
type authGitRunnerAdapter struct {
	debug bool
}

func newAuthGitRunner(debug bool) AuthGitRunner {
	return &authGitRunnerAdapter{debug: debug}
}

func (a *authGitRunnerAdapter) GetRemotes(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var remotes []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			remotes = append(remotes, line)
		}
	}
	return remotes, nil
}

func (a *authGitRunnerAdapter) GetRemoteURL(ctx context.Context, remote string) (string, error) {
	// First check if we have a catmit-specific remote URL for testing
	catmitKey := "catmit.remote." + remote + ".url"
	cmd := exec.CommandContext(ctx, "git", "config", "--get", catmitKey)
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		return strings.TrimSpace(string(output)), nil
	}

	// Fall back to normal remote URL
	cmd = exec.CommandContext(ctx, "git", "remote", "get-url", remote)
	output, err = cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// authProviderDetectorAdapter delegates to the actual provider detector
type authProviderDetectorAdapter struct {
	// We create the detector directly since it's internal to app package
	detector pr.ProviderDetector
}

func newAuthProviderDetector() AuthProviderDetector {
	// Creating the provider detector directly using the same logic from app.newDefaultProviderDetector
	return &authProviderDetectorAdapter{
		detector: app.GetDefaultProviderDetector(),
	}
}

func (a *authProviderDetectorAdapter) DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error) {
	return a.detector.DetectFromRemote(ctx, remoteURL)
}

// authCLIDetectorAdapter wraps the CLI detector
type authCLIDetectorAdapter struct {
	detector *cli.Detector
}

func newAuthCLIDetector() AuthCLIDetector {
	return &authCLIDetectorAdapter{
		detector: cli.NewDetector(nil),
	}
}

func (a *authCLIDetectorAdapter) DetectCLI(ctx context.Context, providerName string) (cli.CLIStatus, error) {
	return a.detector.DetectCLI(ctx, providerName)
}

func (a *authCLIDetectorAdapter) SuggestInstallCommand(cliName string) []string {
	return a.detector.SuggestInstallCommand(cliName)
}
