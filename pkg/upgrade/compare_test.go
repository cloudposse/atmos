package upgrade

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewerRelease(t *testing.T) {
	for _, tc := range []struct {
		name, current, latest, want string
		ok                          bool
	}{
		{"newer patch release", "1.234.0", "v1.234.1", "1.234.1", true},
		{"newer minor with v prefixes", "v1.9.0", "v1.10.0", "1.10.0", true},
		{"release after its prerelease", "1.235.0-rc.1", "v1.235.0", "1.235.0", true},
		{"same version", "v1.234.1", "1.234.1", "", false},
		{"candidate build newer than release", "1.235.0-candidate", "v1.234.1", "", false},
		{"nightly build newer than release", "1.235.0-nightly.44967635975", "v1.234.1", "", false},
		{"older release", "1.235.0", "v1.234.1", "", false},
		{"lexically larger but older release", "1.10.0", "v1.9.0", "", false},
		{"unparsable current version", "test", "v1.234.1", "", false},
		{"unparsable latest release", "1.234.1", "latest", "", false},
		{"empty latest release", "1.234.1", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NewerRelease(tc.current, tc.latest)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		order           int
		ok              bool
	}{
		{"1.234.0", "v1.234.1", -1, true},
		{"v1.234.1", "1.234.1", 0, true},
		{"1.235.0-candidate", "v1.234.1", 1, true},
		{"1.235.0-rc.1", "1.235.0", -1, true},
		{"test", "1.234.1", 0, false},
		{"1.234.1", "not-a-version", 0, false},
	} {
		t.Run(tc.current+"_vs_"+tc.latest, func(t *testing.T) {
			order, ok := CompareVersions(tc.current, tc.latest)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.order, order)
		})
	}
}
