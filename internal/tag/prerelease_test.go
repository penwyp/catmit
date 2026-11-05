package tagging

import (
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrereleaseCounterSurvivesTimestampToggle(t *testing.T) {
	base, err := ParseVersion("1.2.4")
	require.NoError(t, err)
	now := time.Date(2026, 11, 5, 15, 30, 0, 0, time.FixedZone("UTC8", 8*3600))
	tags := []string{"v1.2.4-dev.2+20261104T153000UTC8", "1.2.4-rc.8", "v2.0.0-dev.99", "invalid"}
	first, err := NextPrerelease(base, "dev", true, tags, now)
	require.NoError(t, err)
	assert.Equal(t, "1.2.4-dev.3", first.String())
	tags = append(tags, first.String())
	second, err := NextPrerelease(base, "dev", false, tags, now)
	require.NoError(t, err)
	assert.Equal(t, "1.2.4-dev.4+20261105T153000UTC8", second.String())
	assert.Positive(t, second.Compare(first))
	tags = append(tags, second.String())
	third, err := NextPrerelease(base, "dev", false, tags, now)
	require.NoError(t, err)
	assert.Equal(t, "1.2.4-dev.5+20261105T153000UTC8", third.String(), "same-second calls still have distinct ordered versions")
	rc, err := NextPrerelease(base, "rc", true, tags, now)
	require.NoError(t, err)
	assert.Equal(t, "1.2.4-rc.9", rc.String())
}

func TestTimestampZones(t *testing.T) {
	instant := time.Date(2026, 11, 5, 7, 30, 0, 0, time.UTC)
	for _, tt := range []struct{ zone, want string }{
		{"Asia/Shanghai", "20261105T153000UTC8"},
		{"UTC", "20261105T073000Z"},
		{"Asia/Tokyo", "20261105T163000UTC9"},
		{"Asia/Kolkata", "20261105T130000UTC5-30"},
		{"Asia/Kathmandu", "20261105T131500UTC5-45"},
		{"America/New_York", "20261105T023000UTC-5"},
		{"America/St_Johns", "20261105T040000UTC-3-30"},
	} {
		t.Run(tt.zone, func(t *testing.T) {
			location, err := time.LoadLocation(tt.zone)
			require.NoError(t, err)
			stamp := FormatTimestamp(instant.In(location))
			assert.Equal(t, tt.want, stamp)
			_, err = ParseVersion("v1.2.3-dev.1+" + stamp)
			require.NoError(t, err)
		})
	}
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	assert.Equal(t, "20260705T033000UTC-4", FormatTimestamp(time.Date(2026, 7, 5, 7, 30, 0, 0, time.UTC).In(location)))
}

func TestPrereleaseRejectsInvalidChannelAndOverflow(t *testing.T) {
	for _, channel := range []string{"", "1", "dev.x", "dev+meta", "dev space"} {
		_, err := NextPrerelease(Version{Major: 1}, channel, true, nil, time.Time{})
		require.Error(t, err)
	}
	for _, tag := range []string{"v1.0.0-dev.18446744073709551615", "v1.0.0-dev.18446744073709551616"} {
		_, err := NextPrerelease(Version{Major: 1}, "dev", true, []string{tag}, time.Time{})
		require.ErrorContains(t, err, "overflow")
	}
}
