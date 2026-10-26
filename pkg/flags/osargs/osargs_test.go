package osargs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseString(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flagName string
		want     string
	}{
		{
			name:     "equals syntax",
			args:     []string{"terraform", "--edition=2025-09", "plan"},
			flagName: "edition",
			want:     "2025-09",
		},
		{
			name:     "space syntax with surrounding whitespace preserved",
			args:     []string{"helmfile", "--unknown", "x", "--edition", " 2025-10 "},
			flagName: "edition",
			want:     " 2025-10 ",
		},
		{
			name:     "flag absent",
			args:     []string{"terraform", "plan"},
			flagName: "edition",
			want:     "",
		},
		{
			name:     "help flag among args does not panic or leak usage output",
			args:     []string{"terraform", "plan", "--help"},
			flagName: "edition",
			want:     "",
		},
		{
			name:     "unknown flags are ignored",
			args:     []string{"--totally-unknown=value", "--base-path=/tmp"},
			flagName: "base-path",
			want:     "/tmp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseString(tt.args, tt.flagName))
		})
	}
}

func TestParseStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flagName string
		want     []string
	}{
		{
			name:     "comma-separated single flag",
			args:     []string{"atmos", "--profile=dev,staging"},
			flagName: "profile",
			want:     []string{"dev", "staging"},
		},
		{
			name:     "repeated flag accumulates",
			args:     []string{"atmos", "--config=a.yaml", "--config=b.yaml"},
			flagName: "config",
			want:     []string{"a.yaml", "b.yaml"},
		},
		{
			name:     "flag absent returns nil",
			args:     []string{"atmos", "describe", "config"},
			flagName: "profile",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseStringSlice(tt.args, tt.flagName))
		})
	}
}

func TestParseStringWithShorthand(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		flagName  string
		shorthand string
		want      string
	}{
		{
			name:      "long form with equals",
			args:      []string{"atmos", "--chdir=/tmp/foo", "terraform", "plan"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "/tmp/foo",
		},
		{
			name:      "long form with space",
			args:      []string{"atmos", "--chdir", "/tmp/bar", "terraform", "plan"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "/tmp/bar",
		},
		{
			name:      "shorthand with equals",
			args:      []string{"atmos", "-C=/tmp/baz", "terraform", "plan"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "/tmp/baz",
		},
		{
			name:      "shorthand concatenated",
			args:      []string{"atmos", "-C/tmp/concat", "terraform", "plan"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "/tmp/concat",
		},
		{
			name:      "shorthand concatenated with relative path",
			args:      []string{"atmos", "-C../foo", "terraform", "plan"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "../foo",
		},
		{
			name:      "shorthand with space",
			args:      []string{"atmos", "-C", "/tmp/qux", "terraform", "plan"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "/tmp/qux",
		},
		{
			name:      "flag after bare -- is ignored",
			args:      []string{"atmos", "--", "-C/tmp/should-not-parse"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "",
		},
		{
			name:      "repeated flags: last one wins",
			args:      []string{"atmos", "--chdir=/first", "--chdir=/second"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "/second",
		},
		{
			name:      "flag absent",
			args:      []string{"atmos", "terraform", "plan"},
			flagName:  "chdir",
			shorthand: "C",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseStringWithShorthand(tt.args, tt.flagName, tt.shorthand))
		})
	}
}

// TestParseDoesNotLeakUsageOnRepeatedCalls guards against a regression to the
// duplicate "Usage of <name>:" stderr leak this package's Usage-suppression
// exists to prevent — parsing the same --help-containing args twice (mirroring
// LoadConfig potentially running more than once per command) must not panic
// or otherwise behave differently the second time.
func TestParseDoesNotLeakUsageOnRepeatedCalls(t *testing.T) {
	args := []string{"atmos", "--help"}
	assert.Equal(t, "", ParseString(args, "edition"))
	assert.Equal(t, "", ParseString(args, "edition"))
}
