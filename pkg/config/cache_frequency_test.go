package config

import (
	"testing"
	"time"
)

func TestParseFrequency(t *testing.T) {
	cases := []struct {
		freq    string
		expect  int64
		wantErr bool
	}{
		{"60", 60, false},
		{"2h", 7200, false},
		{"daily", 86400, false},
		{"invalid", 0, true},
	}
	for _, tc := range cases {
		got, err := parseFrequency(tc.freq)
		if tc.wantErr {
			if err == nil {
				t.Errorf("expected error for %s", tc.freq)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseFrequency(%s) unexpected error: %v", tc.freq, err)
			continue
		}
		if got != tc.expect {
			t.Errorf("parseFrequency(%s)=%d, want %d", tc.freq, got, tc.expect)
		}
	}
}

func TestShouldCheckForUpdates(t *testing.T) {
	const day = 24 * time.Hour
	now := time.Now()

	past := now.Add(-day - time.Hour).Unix() // 25 hours ago
	if !ShouldCheckForUpdates(past, "daily") {
		t.Errorf("expected true for past day check")
	}

	recent := now.Add(-10 * time.Second).Unix()
	if ShouldCheckForUpdates(recent, "invalid") {
		t.Errorf("expected false for recent check with invalid freq")
	}
}
