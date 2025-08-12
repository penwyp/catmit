package pr

import ()

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

