package git

import (
	"fmt"
	"regexp"
	"strings"
)

// GongfengHost is the host of Tencent Gongfeng (TGIT / git.woa.com).
const GongfengHost = "git.woa.com"

// MRLink is a merge request link announced by the remote right after a
// successful push, for example the TAPD and TGIT shortcuts that the Gongfeng
// pre-receive hook prints on the console.
type MRLink struct {
	// Label is the short description taken from the text in front of the URL,
	// e.g. "Open in TAPD".
	Label string
	// URL is the link itself.
	URL string
}

var (
	// ansiEscapeRegexp matches the terminal color/control sequences that git or
	// the remote hook may emit around the message.
	ansiEscapeRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[0-9A-Za-z]`)
	// remoteLineRegexp matches the "remote: " prefix that git adds to every line
	// coming from the receiving side (pre-receive hooks and friends).
	remoteLineRegexp = regexp.MustCompile(`(?m)^[ \t]*remote:[ \t]?`)
	// urlRegexp matches an http(s) URL, stopping at whitespace or at the box
	// drawing characters the remote hook uses to frame its message.
	urlRegexp = regexp.MustCompile(`https?://[^\s|]+`)
	// toLineRegexp matches the "To <url>" line git prints for the push target.
	toLineRegexp = regexp.MustCompile(`(?m)^[ \t]*To[ \t]+(\S+)`)
	// remoteHostRegexp captures the host part of a git remote URL, skipping the
	// scheme, the user info and the port.
	remoteHostRegexp = regexp.MustCompile(`^(?:[A-Za-z][A-Za-z0-9+.\-]*://)?(?:[^@/]+@)?(\[[^\]]+\]|[^:/?#]+)`)
)

// urlTrailingPunctuation is trimmed from the end of a matched URL.
const urlTrailingPunctuation = ".,;:!?)]}>\"'"

// HostFromRemoteURL extracts the host name from any git remote URL, e.g.
// "git@git.woa.com:nosql/oss_backend.git", "ssh://git@git.woa.com:22/x.git" and
// "https://git.woa.com/nosql/oss_backend.git" all yield "git.woa.com".
func HostFromRemoteURL(remoteURL string) string {
	raw := strings.TrimSpace(remoteURL)
	if raw == "" {
		return ""
	}

	m := remoteHostRegexp.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}

	host := strings.ToLower(strings.Trim(m[1], "[]"))
	return host
}

// IsGongfengHost reports whether the host is exactly git.woa.com.
func IsGongfengHost(host string) bool {
	return strings.EqualFold(strings.TrimSpace(host), GongfengHost)
}

// IsGongfengRemote reports whether the remote URL points at Tencent Gongfeng.
func IsGongfengRemote(remoteURL string) bool {
	return IsGongfengHost(HostFromRemoteURL(remoteURL))
}

// HostFromPushOutput extracts the host of the push destination from the
// "To <url>" line that git prints while pushing.
func HostFromPushOutput(pushOutput string) string {
	clean := stripTerminalNoise(pushOutput)
	m := toLineRegexp.FindStringSubmatch(clean)
	if len(m) < 2 {
		return ""
	}
	return HostFromRemoteURL(m[1])
}

// ParseMRLinks extracts the links printed by the remote (or one of its hooks)
// during a push. Duplicated links are reported once, in order of appearance.
func ParseMRLinks(pushOutput string) []MRLink {
	if strings.TrimSpace(pushOutput) == "" {
		return nil
	}

	var (
		links []MRLink
		seen  = make(map[string]bool)
	)

	for _, line := range strings.Split(stripTerminalNoise(pushOutput), "\n") {
		for _, loc := range urlRegexp.FindAllStringIndex(line, -1) {
			link := strings.TrimRight(line[loc[0]:loc[1]], urlTrailingPunctuation)
			if link == "" || seen[link] {
				continue
			}
			seen[link] = true
			links = append(links, MRLink{
				Label: linkLabel(line[:loc[0]], link),
				URL:   link,
			})
		}
	}

	return links
}

// MRLinksFromPush returns the merge request links announced while pushing to
// remoteURL. Only Tencent Gongfeng remotes (git.woa.com) are considered, since
// that is the platform whose pre-receive hook prints the TAPD/TGIT shortcuts.
func MRLinksFromPush(remoteURL, pushOutput string) []MRLink {
	host := HostFromRemoteURL(remoteURL)
	if host == "" {
		// Fall back to the destination reported by git itself.
		host = HostFromPushOutput(pushOutput)
	}
	if !IsGongfengHost(host) {
		return nil
	}
	return ParseMRLinks(pushOutput)
}

// FormatMRLinks renders the links as an extra block to print after a push.
func FormatMRLinks(links []MRLink) string {
	if len(links) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\nMerge request links:\n")
	for _, link := range links {
		fmt.Fprintf(&b, "  %s: %s\n", link.Label, link.URL)
	}
	return b.String()
}

// stripTerminalNoise removes ANSI escapes, normalizes line endings and drops
// the "remote: " prefix of the receiving side.
func stripTerminalNoise(output string) string {
	clean := ansiEscapeRegexp.ReplaceAllString(output, "")
	clean = strings.ReplaceAll(clean, "\r\n", "\n")
	clean = strings.ReplaceAll(clean, "\r", "\n")
	return remoteLineRegexp.ReplaceAllString(clean, "")
}

// linkLabel builds the display label of a link from the text in front of it.
func linkLabel(prefix, url string) string {
	label := strings.TrimSpace(prefix)
	label = strings.TrimLeft(label, "|+-=*> \t")
	label = strings.TrimSpace(label)
	label = strings.TrimRight(label, ":：")
	label = strings.TrimSpace(label)
	if label != "" {
		return label
	}
	return defaultLabelForURL(url)
}

// defaultLabelForURL derives a label from the link itself when the remote did
// not put any text in front of it.
func defaultLabelForURL(url string) string {
	host := HostFromRemoteURL(url)
	switch {
	case strings.Contains(host, "tapd"):
		return "TAPD"
	case IsGongfengHost(host):
		return "TGIT"
	case host != "":
		return host
	default:
		return "Merge request"
	}
}
