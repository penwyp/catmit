package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/penwyp/catmit/internal/provider"
)

// TeaPullRequest represents a pull request in tea CLI JSON output
type TeaPullRequest struct {
	Index int `json:"index"`
	URL   string `json:"url"`
	Head  struct {
		Name string `json:"name"`
		Repo struct {
			Owner struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repo"`
	} `json:"head"`
}

// checkGiteaPR checks if a Gitea PR exists for the current branch
func (c *Creator) checkGiteaPR(ctx context.Context, branch string, remoteInfo provider.RemoteInfo) (bool, string, error) {
	// Use tea pulls list with JSON output to get structured data
	// --output json for structured output
	// --fields to get only needed fields (index, head, url)
	// --state open to get only open PRs
	// --repo to specify the repository
	args := []string{"pulls", "list", "--output", "json", "--fields", "index,head,url", "--state", "open", "--repo", fmt.Sprintf("%s/%s", remoteInfo.Owner, remoteInfo.Repo)}

	output, err := c.commandRunner.Run(ctx, "tea", args...)
	if err != nil {
		// If command fails, assume no PR exists
		return false, "", nil
	}

	// Parse JSON output
	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" || outputStr == "[]" {
		// No PRs found
		return false, "", nil
	}

	var prs []TeaPullRequest
	if err := json.Unmarshal([]byte(outputStr), &prs); err != nil {
		// If JSON parsing fails, fall back to no PR exists
		return false, "", nil
	}

	// Check each PR to find one matching the current branch
	for _, pr := range prs {
		// Handle cross-fork scenarios where head branch is "owner:branch"
		if pr.Head.Name == branch {
			// Direct match (same repository)
			return true, pr.URL, nil
		}
		
		// Check for cross-fork format: "owner:branch"
		if strings.Contains(pr.Head.Name, ":") {
			parts := strings.SplitN(pr.Head.Name, ":", 2)
			if len(parts) == 2 && parts[1] == branch {
				// Cross-fork match
				return true, pr.URL, nil
			}
		}
		
		// Also check if the head repo owner matches and branch matches
		// This handles cases where the JSON structure includes repo owner info
		if pr.Head.Repo.Owner.Login != "" && pr.Head.Name == branch {
			return true, pr.URL, nil
		}
	}

	return false, "", nil
}
