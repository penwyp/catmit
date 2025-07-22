package cli

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
)

// Regular expression for semantic versioning
var versionRegex = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([^+]+))?(?:\+(.+))?$`)

// ParseVersion parses a semantic version string
func ParseVersion(versionStr string) (Version, error) {
	if versionStr == "" {
		return Version{}, errors.New(errors.ErrTypeValidation, "empty version string")
	}

	matches := versionRegex.FindStringSubmatch(versionStr)
	if matches == nil {
		return Version{}, errors.Newf(errors.ErrTypeValidation, "invalid version format: %s", versionStr)
	}

	var v Version

	// Major version (required)
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return Version{}, errors.Newf(errors.ErrTypeValidation, "invalid major version: %s", matches[1])
	}
	v.Major = major

	// Minor version (optional, defaults to 0)
	if matches[2] != "" {
		minor, err := strconv.Atoi(matches[2])
		if err != nil {
			return Version{}, errors.Newf(errors.ErrTypeValidation, "invalid minor version: %s", matches[2])
		}
		v.Minor = minor
	}

	// Patch version (optional, defaults to 0)
	if matches[3] != "" {
		patch, err := strconv.Atoi(matches[3])
		if err != nil {
			return Version{}, errors.Newf(errors.ErrTypeValidation, "invalid patch version: %s", matches[3])
		}
		v.Patch = patch
	}

	// Pre-release (optional)
	if matches[4] != "" {
		v.PreRelease = matches[4]
	}

	// Build metadata (optional)
	if matches[5] != "" {
		v.Build = matches[5]
	}

	return v, nil
}

// CompareVersions compares two version strings
// Returns: -1 (v1 < v2), 0 (v1 == v2), 1 (v1 > v2)
func CompareVersions(v1Str, v2Str string) int {
	v1, err1 := ParseVersion(v1Str)
	v2, err2 := ParseVersion(v2Str)

	// If parsing fails, fall back to simple string comparison
	if err1 != nil || err2 != nil {
		return strings.Compare(v1Str, v2Str)
	}

	// Compare major version
	if v1.Major != v2.Major {
		if v1.Major > v2.Major {
			return 1
		}
		return -1
	}

	// Compare minor version
	if v1.Minor != v2.Minor {
		if v1.Minor > v2.Minor {
			return 1
		}
		return -1
	}

	// Compare patch version
	if v1.Patch != v2.Patch {
		if v1.Patch > v2.Patch {
			return 1
		}
		return -1
	}

	// Compare pre-release version
	// A version without pre-release is considered higher than one with pre-release
	if v1.PreRelease == "" && v2.PreRelease != "" {
		return 1
	}
	if v1.PreRelease != "" && v2.PreRelease == "" {
		return -1
	}

	// If both have pre-release, compare them
	if v1.PreRelease != "" && v2.PreRelease != "" {
		result := comparePreRelease(v1.PreRelease, v2.PreRelease)
		if result != 0 {
			return result
		}
	}

	// Build metadata does not affect version comparison
	return 0
}

// comparePreRelease compares pre-release versions
func comparePreRelease(pre1, pre2 string) int {
	// Simplified pre-release comparison
	// Actual semver spec is more complex; this is a simplified approach
	parts1 := strings.Split(pre1, ".")
	parts2 := strings.Split(pre2, ".")

	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		// Try numeric comparison
		num1, err1 := strconv.Atoi(parts1[i])
		num2, err2 := strconv.Atoi(parts2[i])

		if err1 == nil && err2 == nil {
			// Both are numbers
			if num1 != num2 {
				if num1 > num2 {
					return 1
				}
				return -1
			}
		} else {
			// String comparison
			cmp := strings.Compare(parts1[i], parts2[i])
			if cmp != 0 {
				return cmp
			}
		}
	}

	// More segments means higher pre-release version
	if len(parts1) > len(parts2) {
		return 1
	}
	if len(parts1) < len(parts2) {
		return -1
	}

	return 0
}

// CheckMinVersion checks if the current version meets the minimum required version
func CheckMinVersion(current, minimum string) (bool, error) {
	// Parse versions to validate format
	_, err := ParseVersion(current)
	if err != nil {
		return false, errors.Wrap(errors.ErrTypeValidation, "invalid current version", err)
	}

	_, err = ParseVersion(minimum)
	if err != nil {
		return false, errors.Wrap(errors.ErrTypeValidation, "invalid minimum version", err)
	}

	// Use comparison function
	result := CompareVersions(current, minimum)
	return result >= 0, nil
}
