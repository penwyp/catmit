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
	versionPattern            = regexp.MustCompile(`^(v?)(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
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
	Prefix     string
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
}

func ParseVersion(raw string) (Version, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if matches == nil {
		return Version{}, fmt.Errorf("invalid semantic version %q", raw)
	}
	for _, identifier := range strings.Split(matches[5], ".") {
		if isNumericIdentifier(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return Version{}, fmt.Errorf("invalid semantic version %q: numeric prerelease identifier has a leading zero", raw)
		}
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
		Prefix:     matches[1],
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[5],
		Build:      matches[6],
	}, nil
}

func (v Version) String() string {
	version := fmt.Sprintf("%s%d.%d.%d", v.Prefix, v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		version += "-" + v.Prerelease
	}
	if v.Build != "" {
		version += "+" + v.Build
	}
	return version
}

func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return compareInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return compareInt(v.Minor, other.Minor)
	}
	if v.Patch != other.Patch {
		return compareInt(v.Patch, other.Patch)
	}
	if v.Prerelease == other.Prerelease {
		return 0 // Build metadata does not affect SemVer precedence.
	}
	if v.Prerelease == "" {
		return 1
	}
	if other.Prerelease == "" {
		return -1
	}
	left, right := strings.Split(v.Prerelease, "."), strings.Split(other.Prerelease, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] == right[i] {
			continue
		}
		leftNumeric, rightNumeric := isNumericIdentifier(left[i]), isNumericIdentifier(right[i])
		if leftNumeric && rightNumeric {
			// Compare arbitrary-length numbers without integer overflow.
			if len(left[i]) != len(right[i]) {
				return compareInt(len(left[i]), len(right[i]))
			}
		} else if leftNumeric {
			return -1
		} else if rightNumeric {
			return 1
		}
		return strings.Compare(left[i], right[i])
	}
	return compareInt(len(left), len(right))
}

func isNumericIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (v Version) Next(bump Bump) (Version, error) {
	maxInt := int(^uint(0) >> 1)
	if (bump == BumpMajor && v.Major == maxInt) ||
		(bump == BumpMinor && v.Minor == maxInt) ||
		(bump == BumpPatch && v.Patch == maxInt) {
		return Version{}, fmt.Errorf("version component overflow for %s bump", bump)
	}
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

// Core retains the historical tag prefix while removing release suffixes.
func (v Version) Core() Version {
	v.Prerelease, v.Build = "", ""
	return v
}

// SortedVersions returns valid tags in descending precedence, with a stable
// lexical tie-break for refs whose build metadata or prefix differs.
func SortedVersions(tags []string) []Version {
	versions := make([]Version, 0, len(tags))
	for _, name := range tags {
		if version, err := ParseVersion(name); err == nil {
			versions = append(versions, version)
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		if cmp := versions[i].Compare(versions[j]); cmp != 0 {
			return cmp > 0
		}
		return versions[i].String() > versions[j].String()
	})
	return versions
}

func LatestSemVerTag(tags []string) (Version, bool) {
	versions := SortedVersions(tags)
	if len(versions) == 0 {
		return Version{}, false
	}

	return versions[0], true
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
