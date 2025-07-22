package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/spf13/cobra"
)

// RemoteAuthStatus represents the authentication status of a remote repository
type RemoteAuthStatus struct {
	Remote        string
	Provider      string
	CLI           string
	Status        string
	Version       string
	Username      string
	Authenticated bool
}

// AuthGitRunner is the interface for running git commands (for auth commands)
type AuthGitRunner interface {
	GetRemotes(ctx context.Context) ([]string, error)
	GetRemoteURL(ctx context.Context, remote string) (string, error)
}

// AuthProviderDetector is the interface for detecting providers (for auth commands)
type AuthProviderDetector interface {
	DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error)
}

// AuthCLIDetector is the interface for detecting CLI status (for auth commands)
type AuthCLIDetector interface {
	DetectCLI(ctx context.Context, provider string) (cli.CLIStatus, error)
	SuggestInstallCommand(cliName string) []string
}

// NewAuthStatusCommand creates the 'auth status' command
func NewAuthStatusCommand(git AuthGitRunner, providerDetector AuthProviderDetector, cliDetector AuthCLIDetector) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check authentication status for git remotes",
		Long:  `Check the authentication status of CLI tools for all configured git remotes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Get all remotes
			remotes, err := git.GetRemotes(ctx)
			if err != nil {
				return errors.Wrap(errors.ErrTypeGit, "failed to get git remotes", err)
			}

			if len(remotes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No git remotes found")
				return nil
			}

			// Collect authentication status for each remote
			var statuses []RemoteAuthStatus
			installSuggestions := make(map[string][]string)

			for _, remote := range remotes {
				// Get remote URL
				url, err := git.GetRemoteURL(ctx, remote)
				if err != nil {
					continue
				}

				// Detect provider
				info, err := providerDetector.DetectFromRemote(ctx, url)
				if err != nil {
					continue
				}

				status := RemoteAuthStatus{
					Remote:   remote,
					Provider: info.Provider,
				}

				// If provider is unknown
				if info.Provider == "unknown" {
					status.CLI = "-"
					status.Status = "Provider not supported"
					status.Version = "-"
					status.Username = "-"
					statuses = append(statuses, status)
					continue
				}

				// Detect CLI status
				cliStatus, err := cliDetector.DetectCLI(ctx, info.Provider)
				if err != nil {
					status.CLI = "-"
					status.Status = "Detection failed"
					status.Version = "-"
					status.Username = "-"
					statuses = append(statuses, status)
					continue
				}

				status.CLI = cliStatus.Name

				if !cliStatus.Installed {
					status.Status = "✗ Not installed"
					status.Version = "-"
					status.Username = "-"
					// Collect install suggestions
					suggestions := cliDetector.SuggestInstallCommand(cliStatus.Name)
					if len(suggestions) > 0 {
						installSuggestions[cliStatus.Name] = suggestions
					}
				} else {
					status.Version = cliStatus.Version
					if cliStatus.Authenticated {
						status.Status = "✓ Authenticated"
						status.Username = cliStatus.Username
						status.Authenticated = true
					} else {
						status.Status = "✗ Not authenticated"
						status.Username = "-"
					}
				}

				statuses = append(statuses, status)
			}

			// Output table
			table := formatAuthStatusTable(statuses)
			fmt.Fprintln(cmd.OutOrStdout(), table)

			// Output install suggestions
			if len(installSuggestions) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "\nInstall with:")
				for cli, suggestions := range installSuggestions {
					fmt.Fprintf(cmd.OutOrStdout(), "\n%s:\n", cli)
					for _, suggestion := range suggestions {
						fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", suggestion)
					}
				}
			}

			return nil
		},
	}

	return cmd
}

// formatAuthStatusTable formats the authentication status table
func formatAuthStatusTable(statuses []RemoteAuthStatus) string {
	var sb strings.Builder

	// Use the standard library's tabwriter
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', tabwriter.Debug)

	// Print table header
	fmt.Fprintf(w, "Remote\tProvider\tCLI\tStatus\tVersion\tUser\n")
	fmt.Fprintf(w, "------\t--------\t---\t------\t-------\t----\n")

	// Print data rows
	for _, status := range statuses {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			status.Remote,
			status.Provider,
			status.CLI,
			status.Status,
			status.Version,
			status.Username,
		)
	}

	w.Flush()
	return sb.String()
}
