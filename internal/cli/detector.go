package cli

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Detector CLI工具检测器
type Detector struct {
	runner CommandRunner
}

// NewDetector 创建新的CLI检测器
func NewDetector(runner CommandRunner) *Detector {
	if runner == nil {
		runner = &DefaultCommandRunner{}
	}
	return &Detector{runner: runner}
}

// DefaultCommandRunner 默认命令执行器
type DefaultCommandRunner struct{}

func (r *DefaultCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// CheckInstalled 检查CLI工具是否已安装
func (d *Detector) CheckInstalled(ctx context.Context, cliName, checkCommand string) (bool, error) {
	var err error
	// Special handling for tea which uses --version as a flag
	if cliName == "tea" && checkCommand == "--version" {
		_, err = d.runner.Run(ctx, cliName, "--version")
	} else {
		_, err = d.runner.Run(ctx, cliName, checkCommand)
	}
	if err != nil {
		// 如果命令不存在，通常会包含 "command not found" 或 "not found"
		errStr := err.Error()
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "command not found") {
			return false, nil
		}
		// 命令存在但执行失败（如未认证）
		return true, nil
	}
	return true, nil
}

// GetVersion 获取CLI工具版本
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
		return "", fmt.Errorf("failed to get version: %w", err)
	}

	// 提取版本号的正则表达式
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

	return "", fmt.Errorf("version not found in output")
}

// CheckAuthStatus 检查认证状态
func (d *Detector) CheckAuthStatus(ctx context.Context, cliName, authCommand string, args ...string) (bool, string, error) {
	cmdArgs := append([]string{authCommand}, args...)
	output, err := d.runner.Run(ctx, cliName, cmdArgs...)
	
	outputStr := string(output)
	
	// GitHub CLI 认证检查
	if cliName == "gh" {
		if err != nil && strings.Contains(outputStr, "not logged") {
			return false, "", nil
		}
		// 新的输出格式: "✓ Logged in to github.com account username (keyring)"
		userPattern := regexp.MustCompile(`Logged in to .+ account (\w+)`)
		matches := userPattern.FindStringSubmatch(outputStr)
		if len(matches) > 1 {
			return true, matches[1], nil
		}
		// 旧的输出格式: "Logged in to github.com as username"
		userPattern2 := regexp.MustCompile(`Logged in to .+ as (\w+)`)
		matches2 := userPattern2.FindStringSubmatch(outputStr)
		if len(matches2) > 1 {
			return true, matches2[1], nil
		}
		// 如果找到 "✓ Logged in" 但没有匹配到用户名，仍然认为已认证
		if strings.Contains(outputStr, "✓") && strings.Contains(outputStr, "Logged in") {
			return true, "", nil
		}
	}
	
	// tea CLI 认证检查
	if cliName == "tea" {
		if err != nil && strings.Contains(outputStr, "No logins") {
			return false, "", nil
		}
		// 解析tea的表格输出 (支持旧的|分隔符和新的Unicode box drawing)
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			// 跳过表头和分隔线
			if strings.Contains(line, "NAME") || strings.Contains(line, "──") || strings.Contains(line, "┌") || strings.Contains(line, "└") {
				continue
			}
			// 检查包含实际数据的行
			if strings.Contains(line, "│") || strings.Contains(line, "|") {
				// 统一处理不同的分隔符
				normalizedLine := strings.ReplaceAll(line, "│", "|")
				parts := strings.Split(normalizedLine, "|")
				
				// 检查是否是表头行
				if strings.Contains(line, "USER") || strings.Contains(line, "#") {
					continue
				}
				
				// 旧格式: | # | URL | USER | ACTIVE |
				// 新格式: │ NAME │ URL │ SSH HOST │ USER │ DEFAULT │
				if len(parts) >= 5 {
					// 查找USER列的位置
					userIndex := -1
					if strings.Contains(outputStr, "SSH HOST") { // 新格式
						userIndex = 4 // parts[0]空, parts[1]NAME, parts[2]URL, parts[3]SSH HOST, parts[4]USER
					} else if strings.Contains(outputStr, "ACTIVE") { // 旧格式
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
	
	// glab CLI 认证检查
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
		return false, "", fmt.Errorf("failed to check auth status: %w", err)
	}
	
	return false, "", nil
}

// DetectCLI 综合检测CLI工具状态
func (d *Detector) DetectCLI(ctx context.Context, provider string) (CLIStatus, error) {
	// 根据provider确定CLI工具
	cliConfig := map[string]struct {
		name        string
		versionCmd  string
		authCmd     string
		authArgs    []string
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
		return CLIStatus{}, fmt.Errorf("unsupported provider: %s", provider)
	}

	status := CLIStatus{
		Name: config.name,
	}

	// 检查是否安装
	installed, err := d.CheckInstalled(ctx, config.name, config.versionCmd)
	if err != nil {
		return status, err
	}
	status.Installed = installed

	if !installed {
		return status, nil
	}

	// 获取版本
	version, err := d.GetVersion(ctx, config.name, config.versionCmd)
	if err == nil {
		status.Version = version
	}

	// 检查认证状态
	authenticated, username, err := d.CheckAuthStatus(ctx, config.name, config.authCmd, config.authArgs...)
	if err == nil {
		status.Authenticated = authenticated
		status.Username = username
	}

	return status, nil
}

// SuggestInstallCommand 建议安装命令
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

// CheckMinVersion 检查当前版本是否满足最低版本要求
func (d *Detector) CheckMinVersion(current, minimum string) (bool, error) {
	return CheckMinVersion(current, minimum)
}