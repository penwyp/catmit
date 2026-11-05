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
	versionPattern            = regexp.MustCompile(`^(v?)(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
	breakingHeaderPattern     = regexp.MustCompile(`(?m)^[a-z]+(?:\([^)]+\))?!:`)
	featureHeaderPattern      = regexp.MustCompile(`(?m)^feat(?:\([^)]+\))?:`)
	conventionalCommitPattern = regexp.MustCompile(`^[a-z]+(?:\([^)]*\))?!?: .+`)
	validTypes                = map[string]bool{
		"feat": true, "fix": true, "refactor": true, "chore": true,
		"docs": true, "style": true, "test": true, "perf": true,
		"ci": true, "build": true, "revert": true,
	}
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

type BumpDetail struct {
	Bump              Bump
	TotalMessages     int
	ConventionalCount int
	HasFeature        bool
	HasBreaking       bool
}

func (d BumpDetail) AllConventional() bool {
	return d.TotalMessages > 0 && d.ConventionalCount == d.TotalMessages
}

func ValidateConventionalCommit(message string) error {
	firstLine := strings.SplitN(strings.TrimSpace(message), "\n", 2)[0]
	if firstLine == "" {
		return fmt.Errorf("commit message is empty")
	}
	if !conventionalCommitPattern.MatchString(firstLine) {
		return fmt.Errorf("first line does not match Conventional Commits format '<type>(<scope>): <subject>': %q", firstLine)
	}
	parts := strings.SplitN(firstLine, ":", 2)
	typeWithScope := strings.TrimSpace(parts[0])
	// Strip scope if present
	typeOnly := typeWithScope
	if idx := strings.Index(typeWithScope, "("); idx != -1 {
		typeOnly = strings.TrimSpace(typeWithScope[:idx])
	}
	// Strip bang suffix
	typeOnly = strings.TrimSuffix(typeOnly, "!")
	if !validTypes[typeOnly] {
		return fmt.Errorf("unsupported commit type %q in %q", typeOnly, firstLine)
	}
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("missing subject after type prefix: %q", firstLine)
	}
	return nil
}

func TryRepairCommitMessage(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	lines := strings.Split(trimmed, "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if conventionalCommitPattern.MatchString(line) {
			var result string
			if i == 0 {
				result = trimmed
			} else {
				rest := lines[i+1:]
				// Skip leading empty lines
				for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
					rest = rest[1:]
				}
				if len(rest) > 0 {
					result = line + "\n\n" + strings.Join(rest, "\n")
				} else {
					result = line
				}
				result = strings.TrimSpace(result)
			}
			if err := ValidateConventionalCommit(result); err == nil {
				return result, nil
			}
		}
	}
	return "", fmt.Errorf("no Conventional Commits header found in LLM output")
}

func InferBumpWithDetail(messages []string) BumpDetail {
	detail := BumpDetail{
		TotalMessages: len(messages),
		Bump:          BumpPatch,
	}
	for _, message := range messages {
		if hasBreakingChange(message) {
			detail.HasBreaking = true
		}
		if featureHeaderPattern.MatchString(strings.TrimSpace(message)) {
			detail.HasFeature = true
		}
		if ValidateConventionalCommit(message) == nil {
			detail.ConventionalCount++
		}
	}
	if detail.HasBreaking {
		detail.Bump = BumpMajor
	} else if detail.HasFeature {
		detail.Bump = BumpMinor
	}
	return detail
}

func InferBump(messages []string) Bump {
	return InferBumpWithDetail(messages).Bump
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
