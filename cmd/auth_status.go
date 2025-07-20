package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/penwyp/catmit/internal/cli"
	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/provider"
	"github.com/spf13/cobra"
)

// RemoteAuthStatus 远程仓库认证状态
type RemoteAuthStatus struct {
	Remote        string
	Provider      string
	CLI           string
	Status        string
	Version       string
	Username      string
	Authenticated bool
}

// GitRunner Git命令执行器接口
type GitRunner interface {
	GetRemotes(ctx context.Context) ([]string, error)
	GetRemoteURL(ctx context.Context, remote string) (string, error)
}

// ProviderDetector Provider检测器接口
type ProviderDetector interface {
	DetectFromRemote(ctx context.Context, remoteURL string) (provider.RemoteInfo, error)
}

// CLIDetector CLI检测器接口
type CLIDetector interface {
	DetectCLI(ctx context.Context, provider string) (cli.CLIStatus, error)
	SuggestInstallCommand(cliName string) []string
}

// NewAuthStatusCommand 创建auth status命令
func NewAuthStatusCommand(git GitRunner, providerDetector ProviderDetector, cliDetector CLIDetector) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check authentication status for git remotes",
		Long:  `Check the authentication status of CLI tools for all configured git remotes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			
			// 获取所有remotes
			remotes, err := git.GetRemotes(ctx)
			if err != nil {
				return errors.Wrap(errors.ErrTypeGit, "failed to get git remotes", err)
			}
			
			if len(remotes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No git remotes found")
				return nil
			}
			
			// 收集每个remote的认证状态
			var statuses []RemoteAuthStatus
			installSuggestions := make(map[string][]string)
			
			for _, remote := range remotes {
				// 获取remote URL
				url, err := git.GetRemoteURL(ctx, remote)
				if err != nil {
					continue
				}
				
				// 检测provider
				info, err := providerDetector.DetectFromRemote(ctx, url)
				if err != nil {
					continue
				}
				
				status := RemoteAuthStatus{
					Remote:   remote,
					Provider: info.Provider,
				}
				
				// 如果是未知provider
				if info.Provider == "unknown" {
					status.CLI = "-"
					status.Status = "Provider not supported"
					status.Version = "-"
					status.Username = "-"
					statuses = append(statuses, status)
					continue
				}
				
				// 检测CLI状态
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
					// 收集安装建议
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
			
			// 输出表格
			table := formatAuthStatusTable(statuses)
			fmt.Fprintln(cmd.OutOrStdout(), table)
			
			// 输出安装建议
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

// formatAuthStatusTable 格式化认证状态表格
func formatAuthStatusTable(statuses []RemoteAuthStatus) string {
	var sb strings.Builder
	
	// 使用标准库的tabwriter
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', tabwriter.Debug)
	
	// 打印表头
	fmt.Fprintf(w, "Remote\tProvider\tCLI\tStatus\tVersion\tUser\n")
	fmt.Fprintf(w, "------\t--------\t---\t------\t-------\t----\n")
	
	// 打印数据行
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

// formatAuthStatusWithColor 格式化带颜色的认证状态
func formatAuthStatusWithColor(status RemoteAuthStatus) string {
	if status.Authenticated {
		return color.GreenString(status.Status)
	}
	return color.RedString(status.Status)
}