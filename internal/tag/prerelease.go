package tagging

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var channelPattern = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z-]*$`)

func ValidateChannel(channel string) error {
	if !channelPattern.MatchString(channel) {
		return fmt.Errorf("invalid prerelease channel %q: use a letter followed by letters, digits or hyphens", channel)
	}
	return nil
}

// NextPrerelease always increments the channel counter. The optional timestamp
// follows the counter as a prerelease identifier without Docker's invalid '+' separator.
func NextPrerelease(base Version, channel string, noTimestamp bool, tags []string, now time.Time) (Version, error) {
	if err := ValidateChannel(channel); err != nil {
		return Version{}, err
	}
	var counter uint64
	for _, name := range tags {
		version, err := ParseVersion(name)
		if err != nil || version.Core().Compare(base.Core()) != 0 {
			continue
		}
		parts := strings.Split(version.Prerelease, ".")
		if len(parts) < 2 || parts[0] != channel || !isNumericIdentifier(parts[1]) {
			continue
		}
		n, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil || n == ^uint64(0) {
			return Version{}, fmt.Errorf("prerelease counter overflow in %q", name)
		}
		if n > counter {
			counter = n
		}
	}
	next := base.Core()
	next.Prerelease = fmt.Sprintf("%s.%d", channel, counter+1)
	if !noTimestamp {
		next.Prerelease += "." + FormatTimestamp(now)
	}
	return next, nil
}

// FormatTimestamp uses only SemVer-safe characters. Fractional-hour offsets
// use hyphens, for example UTC5-30 (India) or UTC-3-30 (Newfoundland).
func FormatTimestamp(now time.Time) string {
	_, offset := now.Zone()
	suffix := "Z"
	if offset != 0 {
		sign := ""
		if offset < 0 {
			sign, offset = "-", -offset
		}
		suffix = fmt.Sprintf("UTC%s%d", sign, offset/3600)
		if offset%3600 != 0 {
			suffix += fmt.Sprintf("-%02d", offset%3600/60)
		}
		if offset%60 != 0 {
			suffix += fmt.Sprintf("-%02d", offset%60)
		}
	}
	return now.Format("20060102T150405") + suffix
}
