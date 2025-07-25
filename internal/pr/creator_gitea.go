package pr

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/penwyp/catmit/internal/provider"
)

// checkGiteaPR checks if a Gitea PR exists for the current branch
func (c *Creator) checkGiteaPR(ctx context.Context, branch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Use tea pulls to get open PR list
	// --state open to get only open PRs
	// --repo to specify the repository
	args := []string{"pulls", "--state", "open", "--repo", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}

	output, err := c.commandRunner.Run(ctx, "tea", args...)
	if err != nil {
		// If command fails, assume no PR exists
		return false, "", nil
	}

	// Parse output to find matching branch
	// tea output format: "#123 PR Title (source-branch -> target-branch)"
	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")

	// Create regex pattern to match PR with the current branch
	branchPattern := fmt.Sprintf(`\(%s\s*->\s*`, regexp.QuoteMeta(branch))

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Check if line contains the current branch as source
		if matched, _ := regexp.MatchString(branchPattern, line); matched {
			// Extract PR number from the line
			prNumberRe := regexp.MustCompile(`#(\d+)`)
			if matches := prNumberRe.FindStringSubmatch(line); len(matches) > 1 {
				prNumber := matches[1]
				// Construct PR URL
				// Use HTTPS protocol and ensure no port in URL for standard web access
				host := remoteInfo.Host
				if remoteInfo.Port != 0 && remoteInfo.Port != 80 && remoteInfo.Port != 443 {
					host = fmt.Sprintf("%s:%d", host, remoteInfo.Port)
				}
				prURL := fmt.Sprintf("https://%s/%s/%s/pulls/%s",
					host, remoteInfo.Owner, remoteInfo.Repo, prNumber)
				return true, prURL, nil
			}
		}
	}

	return false, "", nil
}
