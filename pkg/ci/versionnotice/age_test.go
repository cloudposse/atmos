package versionnotice

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnotationTitleAge(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	const base = "Atmos 1.100.0 is 10 releases behind"
	for _, tc := range []struct{ name, date, suffix string }{
		{"four months", "2026-06-27T12:00:00Z", " (about 4 months old)"},
		{"one month", "2026-09-27T12:00:00Z", " (about 1 month old)"},
		{"years", "2024-10-27T12:00:00Z", " (about 2 years old)"},
		{"one day", "2026-10-26T12:00:00Z", " (about 1 day old)"},
		{"timezone offset", "2026-06-27T14:00:00+02:00", " (about 4 months old)"},
		{"unstamped", "", ""},
		{"malformed", "unknown", ""},
		{"zero", "0001-01-01T00:00:00Z", ""},
		{"future", "2026-10-28T12:00:00Z", ""},
		{"same instant", "2026-10-27T12:00:00Z", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, base+tc.suffix, annotationTitle("v1.100.0", 10, tc.date, now))
		})
	}
}

func TestPreparedNoticeIncludesEmbeddedAge(t *testing.T) {
	setup(t)
	s := settings()
	// Use elapsed days, with margin above four 30-day months, regardless of DST.
	s.buildDate = time.Now().Add(-121 * 24 * time.Hour).Format(time.RFC3339)
	client := &fakeClient{latest: "1.110.0"}
	notice, ok := prepare(t.Context(), s, fakeDependencies(client))
	require.True(t, ok)
	assert.Equal(t, "Atmos 1.100.0 is 10 releases behind (about 4 months old)", notice.annotation.Title)
	assert.Equal(t, 1, client.calls, "age must not require another release request")
}
