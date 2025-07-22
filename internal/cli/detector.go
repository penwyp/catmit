package cli

import (
	"context"
	"os/exec"
	"regexp"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
)

// Detector CLI tool detector
type Detector struct {
	runner CommandRunner
}

// NewDetector creates a new CLI detector
func NewDetector(runner CommandRunner) *Detector {
	if runner == nil {
		runner = &DefaultCommandRunner{}
	}
	return &Detector{runner: runner}
}

// DefaultCommandRunner default command runner
type DefaultCommandRunner struct{}

func (r *DefaultCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// CheckInstalled checks if the CLI tool is installed
func (d *Detector) CheckInstalled(ctx context.Context, cliName, checkCommand string) (bool, error) {
	var err error
	// Special handling for tea which uses --version as a flag
	if cliName == "tea" && checkCommand == "--version" {
		_, err = d.runner.Run(ctx, cliName, "--version")
	} else {
		_, err = d.runner.Run(ctx, cliName, checkCommand)
	}
	if err != nil {
		// If the command does not exist, it usually contains "command not found" or "not found"
		errStr := err.Error()
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "command not found") {
			return false, nil
		}
		// The command exists but fails to execute (e.g., not authenticated)
		return true, nil
	}
	return true, nil
}

// GetVersion gets the CLI tool version
func (d *Detector) GetVersion(ctx context.Context, cliName, versionCommand string, args ...string) (string, error) {
	var output []byte
	var err error

	// Special handling for tea which uses --version as a flag, not a subcommand
	if cliName == "tea" && versionCommand == "--version" {
		output, err = d.runner.Run(ctx, cliName, "--version")
	} else {
		cmdArgs := append([]string{versionCommand}, args...)
		output, err = d.runner.Run(ctx, cliName, cmdArgs...)
	}
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeExternal, "failed to get version", err)
	}

	// Regular expression to extract version numbers
	versionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`version\s+v?(\d+\.\d+\.\d+(?:-[^\s]+)?(?:\+[^\s]+)?)`),
		regexp.MustCompile(`(\d+\.\d+\.\d+(?:-[^\s]+)?(?:\+[^\s]+)?)`),
		regexp.MustCompile(`Version:\s*v?(\d+\.\d+\.\d+(?:-[^\s]+)?(?:\+[^\s]+)?)`),
		// tea specific pattern for "Version: development" or "Version: x.y.z" with ANSI codes
		regexp.MustCompile(`Version:\s*(?:\x1b\[\d+m)?([^\x1b\s\t]+)(?:\x1b\[\d+m)?`),
	}

	outputStr := string(output)
	for _, pattern := range versionPatterns {
		matches := pattern.FindStringSubmatch(outputStr)
		if len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "", errors.New(errors.ErrTypeValidation, "version not found in output")
}

// CheckAuthStatus checks the authentication status
func (d *Detector) CheckAuthStatus(ctx context.Context, cliName, authCommand string, args ...string) (bool, string, error) {
	cmdArgs := append([]string{authCommand}, args...)
	output, err := d.runner.Run(ctx, cliName, cmdArgs...)

	outputStr := string(output)

	// GitHub CLI authentication check
	if cliName == "gh" {
		if err != nil && strings.Contains(outputStr, "not logged") {
			return false, "", nil
		}
		// New output format: "✓ Logged in to github.com account username (keyring)"
		userPattern := regexp.MustCompile(`Logged in to .+ account (\w+)`)
		matches := userPattern.FindStringSubmatch(outputStr)
		if len(matches) > 1 {
			return true, matches[1], nil
		}
		// Old output format: "Logged in to github.com as username"
		userPattern2 := regexp.MustCompile(`Logged in to .+ as (\w+)`)
		matches2 := userPattern2.FindStringSubmatch(outputStr)
		if len(matches2) > 1 {
			return true, matches2[1], nil
		}
		// If "✓ Logged in" is found but no username is matched, still consider it authenticated
		if strings.Contains(outputStr, "✓") && strings.Contains(outputStr, "Logged in") {
			return true, "", nil
		}
	}

	// tea CLI authentication check
	if cliName == "tea" {
		if err != nil && strings.Contains(outputStr, "No logins") {
			return false, "", nil
		}
		// Parse tea's table output (supports old | separator and new Unicode box drawing)
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			// Skip header and separator lines
			if strings.Contains(line, "NAME") || strings.Contains(line, "──") || strings.Contains(line, "┌") || strings.Contains(line, "└") {
				continue
			}
			// Check lines containing actual data
			if strings.Contains(line, "│") || strings.Contains(line, "|") {
				// Handle different separators uniformly
				normalizedLine := strings.ReplaceAll(line, "│", "|")
				parts := strings.Split(normalizedLine, "|")

				// Check if it's a header line
				if strings.Contains(line, "USER") || strings.Contains(line, "#") {
					continue
				}

				// Old format: | # | URL | USER | ACTIVE |
				// New format: │ NAME │ URL │ SSH HOST │ USER │ DEFAULT │
				if len(parts) >= 5 {
					// Find the position of the USER column
					userIndex := -1
					if strings.Contains(outputStr, "SSH HOST") { // New format
						userIndex = 4 // parts[0]空, parts[1]NAME, parts[2]URL, parts[3]SSH HOST, parts[4]USER
					} else if strings.Contains(outputStr, "ACTIVE") { // Old format
						userIndex = 3 // parts[0]空, parts[1]#, parts[2]URL, parts[3]USER, parts[4]ACTIVE
					}

					if userIndex > 0 && userIndex < len(parts) {
						username := strings.TrimSpace(parts[userIndex])
						if username != "" && username != "USER" {
							return true, username, nil
						}
					}
				}
			}
		}
	}

	// glab CLI authentication check
	if cliName == "glab" {
		if err != nil && strings.Contains(outputStr, "No accounts configured") {
			return false, "", nil
		}
		// Check for checkmark indicating logged in
		if strings.Contains(outputStr, "✓") && strings.Contains(outputStr, "Logged in to") {
			// Extract username from pattern like "✓ Logged in to gitlab.com as username"
			userPattern := regexp.MustCompile(`Logged in to .+ as (\w+)`)
			matches := userPattern.FindStringSubmatch(outputStr)
			if len(matches) > 1 {
				return true, matches[1], nil
			}
			return true, "", nil
		}
		// Also check for pattern without checkmark
		if strings.Contains(outputStr, "Logged in to") {
			userPattern := regexp.MustCompile(`Logged in to .+ as (\w+)`)
			matches := userPattern.FindStringSubmatch(outputStr)
			if len(matches) > 1 {
				return true, matches[1], nil
			}
		}
	}

	if err != nil {
		return false, "", errors.Wrap(errors.ErrTypeExternal, "failed to check auth status", err)
	}

	return false, "", nil
}

// DetectCLI comprehensive CLI detection functionality
func (d *Detector) DetectCLI(ctx context.Context, provider string) (CLIStatus, error) {
	// Determine the CLI tool based on the provider
	cliConfig := map[string]struct {
		name       string
		versionCmd string
		authCmd    string
		authArgs   []string
	}{
		"github": {
			name:       "gh",
			versionCmd: "version",
			authCmd:    "auth",
			authArgs:   []string{"status"},
		},
		"gitea": {
			name:       "tea",
			versionCmd: "--version",
			authCmd:    "login",
			authArgs:   []string{"list"},
		},
		"gitlab": {
			name:       "glab",
			versionCmd: "version",
			authCmd:    "auth",
			authArgs:   []string{"status"},
		},
	}

	config, exists := cliConfig[provider]
	if !exists {
		return CLIStatus{}, errors.Newf(errors.ErrTypeValidation, "unsupported provider: %s", provider)
	}

	status := CLIStatus{
		Name: config.name,
	}

	// Check if installed
	installed, err := d.CheckInstalled(ctx, config.name, config.versionCmd)
	if err != nil {
		return status, err
	}
	status.Installed = installed

	if !installed {
		return status, nil
	}

	// Get version
	version, err := d.GetVersion(ctx, config.name, config.versionCmd)
	if err == nil {
		status.Version = version
	}

	// Check authentication status
	authenticated, username, err := d.CheckAuthStatus(ctx, config.name, config.authCmd, config.authArgs...)
	if err == nil {
		status.Authenticated = authenticated
		status.Username = username
	}

	return status, nil
}

// SuggestInstallCommand suggests installation commands
func (d *Detector) SuggestInstallCommand(cliName string) []string {
	installCommands := map[string][]string{
		"gh": {
			"brew install gh",
			"https://github.com/cli/cli#installation",
		},
		"tea": {
			"go install gitea.com/gitea/tea@latest",
			"https://gitea.com/gitea/tea",
		},
	}

	if commands, exists := installCommands[cliName]; exists {
		return commands
	}
	return []string{}
}

// CheckMinVersion checks if the current version meets the minimum version requirement
func (d *Detector) CheckMinVersion(current, minimum string) (bool, error) {
	return CheckMinVersion(current, minimum)
}
