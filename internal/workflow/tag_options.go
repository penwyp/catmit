package workflow

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // Keep IANA zones available in minimal installations.

	tagging "github.com/penwyp/catmit/internal/tag"
)

// TagOptions are supplied exclusively by CLI flags, never a config file or env.
type TagOptions struct {
	DryRun         bool
	Yes            bool
	Timeout        int
	Remote         string
	Bump           string
	ExplicitTag    string
	InitialVersion string
	Pre            string
	Release        bool
	NoTimestamp    bool
	Timezone       string
}

func (o TagOptions) Validate() error {
	if o.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}
	if strings.TrimSpace(o.Remote) == "" || strings.HasPrefix(o.Remote, "-") {
		return fmt.Errorf("remote must be nonempty and cannot start with '-'")
	}
	if _, err := tagging.NormalizeBump(o.Bump); err != nil {
		return err
	}
	if o.Timezone == "" || o.Timezone == "Local" {
		return fmt.Errorf("--timezone must be UTC or an IANA name, such as Asia/Shanghai")
	}
	if _, err := time.LoadLocation(o.Timezone); err != nil {
		return fmt.Errorf("invalid --timezone %q: %w", o.Timezone, err)
	}
	initial, err := tagging.ParseVersion(o.InitialVersion)
	if err != nil || initial.Prerelease != "" || initial.Build != "" {
		return fmt.Errorf("--initial must be a plain semantic version, such as v0.1.0")
	}
	if o.Pre != "" {
		if err := tagging.ValidateChannel(o.Pre); err != nil {
			return err
		}
		if o.Release {
			return fmt.Errorf("--pre and --release are mutually exclusive")
		}
	}
	if o.ExplicitTag != "" {
		if _, err := tagging.ParseVersion(o.ExplicitTag); err != nil {
			return fmt.Errorf("invalid --tag: %w", err)
		}
		if o.Pre != "" || o.Release || o.NoTimestamp || o.Bump != "auto" {
			return fmt.Errorf("--tag cannot be combined with --pre, --release, --notimestamp or --bump")
		}
	}
	return nil
}
