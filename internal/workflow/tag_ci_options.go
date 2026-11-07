package workflow

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const DefaultVersionOrder = "webhook-tag,env,history,timestamp"

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type TagCIOptions struct {
	Provider     string
	VersionOrder string
	VersionEnv   string
	Checkout     bool
	Version      TagOptions
}

func (o TagCIOptions) sources() []string { return strings.Split(o.VersionOrder, ",") }

func (o TagCIOptions) Validate() error {
	if o.Provider != "bk-ci" {
		return fmt.Errorf("unsupported CI provider %q; supported: bk-ci", o.Provider)
	}
	if !envNamePattern.MatchString(o.VersionEnv) {
		return fmt.Errorf("--version-env must be a shell environment variable name")
	}
	seen := make(map[string]bool)
	for _, source := range o.sources() {
		switch source {
		case "webhook-tag", "env", "history", "timestamp":
		default:
			return fmt.Errorf("invalid --version-order source %q; use webhook-tag,env,history,timestamp", source)
		}
		if seen[source] {
			return fmt.Errorf("duplicate --version-order source %q", source)
		}
		seen[source] = true
	}
	return o.Version.Validate()
}

func validateCIVersion(version string) error {
	if version == "" || strings.TrimSpace(version) != version {
		return fmt.Errorf("version must be nonempty and have no surrounding whitespace")
	}
	if strings.ContainsFunc(version, unicode.IsControl) {
		return fmt.Errorf("version must be a single line without control characters")
	}
	return nil
}
