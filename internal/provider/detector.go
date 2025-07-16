package provider

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	// SSH格式: git@host:owner/repo.git 或 ssh://git@host:port/owner/repo.git
	sshPattern = regexp.MustCompile(`^(?:ssh://)?(?:git@)?([^:/]+)(?::(\d+))?[:/](.+?)(?:\.git)?$`)
	
	// HTTPS格式: https://host[:port]/owner/repo[.git]
	httpsPattern = regexp.MustCompile(`^https?://([^/]+)/(.+?)(?:\.git)?$`)
)

// ParseGitRemoteURL 解析Git remote URL
func ParseGitRemoteURL(remoteURL string) (RemoteInfo, error) {
	if remoteURL == "" {
		return RemoteInfo{}, fmt.Errorf("empty URL")
	}

	var info RemoteInfo

	// 尝试解析SSH格式
	if strings.HasPrefix(remoteURL, "git@") || strings.HasPrefix(remoteURL, "ssh://") {
		matches := sshPattern.FindStringSubmatch(remoteURL)
		if len(matches) < 4 {
			return RemoteInfo{}, fmt.Errorf("invalid SSH URL format: %s", remoteURL)
		}

		info.Protocol = "ssh"
		info.Host = matches[1]
		
		// 处理端口
		if matches[2] != "" {
			port, err := strconv.Atoi(matches[2])
			if err != nil {
				return RemoteInfo{}, fmt.Errorf("invalid port: %s", matches[2])
			}
			info.Port = port
		}

		// 解析路径
		pathParts := strings.Split(matches[3], "/")
		if len(pathParts) < 2 {
			return RemoteInfo{}, fmt.Errorf("invalid repository path: %s", matches[3])
		}

		info.Repo = strings.TrimSuffix(pathParts[len(pathParts)-1], ".git")
		info.Owner = strings.Join(pathParts[:len(pathParts)-1], "/")

	} else if strings.HasPrefix(remoteURL, "http://") || strings.HasPrefix(remoteURL, "https://") {
		// 解析HTTPS格式
		u, err := url.Parse(remoteURL)
		if err != nil {
			return RemoteInfo{}, fmt.Errorf("invalid URL: %w", err)
		}

		info.Protocol = "https"
		info.Host = u.Hostname()

		// 处理端口
		if u.Port() != "" {
			port, err := strconv.Atoi(u.Port())
			if err != nil {
				return RemoteInfo{}, fmt.Errorf("invalid port: %s", u.Port())
			}
			info.Port = port
		}

		// 解析路径
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		
		if path == "" {
			return RemoteInfo{}, fmt.Errorf("missing repository path")
		}

		pathParts := strings.Split(path, "/")
		if len(pathParts) < 2 {
			return RemoteInfo{}, fmt.Errorf("invalid repository path: %s", path)
		}

		info.Repo = pathParts[len(pathParts)-1]
		info.Owner = strings.Join(pathParts[:len(pathParts)-1], "/")

	} else {
		return RemoteInfo{}, fmt.Errorf("unsupported URL format: %s", remoteURL)
	}

	// 检测Provider类型
	info.Provider = detectProviderFromHost(info.Host)

	return info, nil
}

// detectProviderFromHost 根据主机名检测Provider类型
func detectProviderFromHost(host string) string {
	switch {
	case strings.Contains(host, "github.com"):
		return "github"
	case strings.Contains(host, "gitlab.com"):
		return "gitlab"
	case strings.Contains(host, "bitbucket.org"):
		return "bitbucket"
	default:
		return "unknown"
	}
}

// GetHTTPURL 获取HTTP(S) URL
func (r RemoteInfo) GetHTTPURL() string {
	if r.Port > 0 && r.Port != 80 && r.Port != 443 {
		return fmt.Sprintf("https://%s:%d", r.Host, r.Port)
	}
	return fmt.Sprintf("https://%s", r.Host)
}