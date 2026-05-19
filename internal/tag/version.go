package tagging

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Bump string

const (
	BumpAuto  Bump = "auto"
	BumpPatch Bump = "patch"
	BumpMinor Bump = "minor"
	BumpMajor Bump = "major"
)

var (
	versionPattern        = regexp.MustCompile(`^(v?)(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
	breakingHeaderPattern = regexp.MustCompile(`(?m)^[a-z]+(?:\([^)]+\))?!:`)
	featureHeaderPattern  = regexp.MustCompile(`(?m)^feat(?:\([^)]+\))?:`)
)

type Version struct {
	Prefix string
	Major  int
	Minor  int
	Patch  int
}

func ParseVersion(raw string) (Version, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if matches == nil {
		return Version{}, fmt.Errorf("invalid semantic version %q", raw)
	}

	major, err := strconv.Atoi(matches[2])
	if err != nil {
		return Version{}, err
	}
	minor, err := strconv.Atoi(matches[3])
	if err != nil {
		return Version{}, err
	}
	patch, err := strconv.Atoi(matches[4])
	if err != nil {
		return Version{}, err
	}

	return Version{
		Prefix: matches[1],
		Major:  major,
		Minor:  minor,
		Patch:  patch,
	}, nil
}

func (v Version) String() string {
	return fmt.Sprintf("%s%d.%d.%d", v.Prefix, v.Major, v.Minor, v.Patch)
}

func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return compareInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return compareInt(v.Minor, other.Minor)
	}
	return compareInt(v.Patch, other.Patch)
}

func (v Version) Next(bump Bump) (Version, error) {
	switch bump {
	case BumpMajor:
		return Version{Prefix: v.Prefix, Major: v.Major + 1}, nil
	case BumpMinor:
		return Version{Prefix: v.Prefix, Major: v.Major, Minor: v.Minor + 1}, nil
	case BumpPatch:
		return Version{Prefix: v.Prefix, Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}, nil
	default:
		return Version{}, fmt.Errorf("unsupported bump %q", bump)
	}
}

func LatestSemVerTag(tags []string) (Version, bool) {
	versions := make([]Version, 0, len(tags))
	for _, name := range tags {
		version, err := ParseVersion(name)
		if err == nil {
			versions = append(versions, version)
		}
	}
	if len(versions) == 0 {
		return Version{}, false
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Compare(versions[j]) < 0
	})

	return versions[len(versions)-1], true
}

func NormalizeBump(raw string) (Bump, error) {
	bump := Bump(strings.ToLower(strings.TrimSpace(raw)))
	switch bump {
	case BumpAuto, BumpPatch, BumpMinor, BumpMajor:
		return bump, nil
	default:
		return "", fmt.Errorf("unsupported bump %q", raw)
	}
}

func InferBump(messages []string) Bump {
	for _, message := range messages {
		if hasBreakingChange(message) {
			return BumpMajor
		}
	}
	for _, message := range messages {
		if featureHeaderPattern.MatchString(strings.TrimSpace(message)) {
			return BumpMinor
		}
	}
	return BumpPatch
}

func hasBreakingChange(message string) bool {
	normalized := strings.TrimSpace(message)
	return strings.Contains(normalized, "BREAKING CHANGE:") ||
		strings.Contains(normalized, "BREAKING-CHANGE:") ||
		breakingHeaderPattern.MatchString(normalized)
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
