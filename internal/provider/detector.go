package provider

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
)

var (
	// SSH format: git@host:owner/repo.git or ssh://git@host:port/owner/repo.git
	sshPattern = regexp.MustCompile(`^(?:ssh://)?(?:git@)?([^:/]+)(?::(\d+))?[:/](.+?)(?:\.git)?$`)
)

// ParseGitRemoteURL parses a Git remote URL
func ParseGitRemoteURL(remoteURL string) (RemoteInfo, error) {
	if remoteURL == "" {
		return RemoteInfo{}, errors.New(errors.ErrTypeValidation, "empty URL")
	}

	var info RemoteInfo

	// Try to parse SSH format
	if strings.HasPrefix(remoteURL, "git@") || strings.HasPrefix(remoteURL, "ssh://") {
		matches := sshPattern.FindStringSubmatch(remoteURL)
		if len(matches) < 4 {
			return RemoteInfo{}, errors.Newf(errors.ErrTypeValidation, "invalid SSH URL format: %s", remoteURL)
		}

		info.Protocol = "ssh"
		info.Host = matches[1]

		// Handle port
		if matches[2] != "" {
			port, err := strconv.Atoi(matches[2])
			if err != nil {
				return RemoteInfo{}, errors.Newf(errors.ErrTypeValidation, "invalid port: %s", matches[2])
			}
			info.Port = port
		}

		// Parse path
		pathParts := strings.Split(matches[3], "/")
		if len(pathParts) < 2 {
			return RemoteInfo{}, errors.Newf(errors.ErrTypeValidation, "invalid repository path: %s", matches[3])
		}

		info.Repo = strings.TrimSuffix(pathParts[len(pathParts)-1], ".git")
		info.Owner = strings.Join(pathParts[:len(pathParts)-1], "/")

	} else if strings.HasPrefix(remoteURL, "http://") || strings.HasPrefix(remoteURL, "https://") {
		// Parse HTTPS format
		u, err := url.Parse(remoteURL)
		if err != nil {
			return RemoteInfo{}, errors.Wrap(errors.ErrTypeValidation, "invalid URL", err)
		}

		info.Protocol = "https"
		info.Host = u.Hostname()

		// Handle port
		if u.Port() != "" {
			port, err := strconv.Atoi(u.Port())
			if err != nil {
				return RemoteInfo{}, errors.Newf(errors.ErrTypeValidation, "invalid port: %s", u.Port())
			}
			info.Port = port
		}

		// Parse path
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")

		if path == "" {
			return RemoteInfo{}, errors.New(errors.ErrTypeValidation, "missing repository path")
		}

		pathParts := strings.Split(path, "/")
		if len(pathParts) < 2 {
			return RemoteInfo{}, errors.Newf(errors.ErrTypeValidation, "invalid repository path: %s", path)
		}

		info.Repo = pathParts[len(pathParts)-1]
		info.Owner = strings.Join(pathParts[:len(pathParts)-1], "/")

	} else {
		return RemoteInfo{}, errors.Newf(errors.ErrTypeValidation, "unsupported URL format: %s", remoteURL)
	}

	// Detect provider type
	info.Provider = detectProviderFromHost(info.Host)

	return info, nil
}

// detectProviderFromHost detects provider type by host name
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

// GetHTTPURL returns the HTTP(S) URL
func (r RemoteInfo) GetHTTPURL() string {
	if r.Port > 0 && r.Port != 80 && r.Port != 443 {
		return "https://" + r.Host + ":" + strconv.Itoa(r.Port)
	}
	return "https://" + r.Host
}
