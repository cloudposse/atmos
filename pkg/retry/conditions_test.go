package retry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompileConditions covers the supported pattern shapes (plain, slash-delimited,
// whitespace-padded, empty/skipped, invalid) and verifies that compile failures wrap
// ErrInvalidCondition so callers can branch on errors.Is.
func TestCompileConditions(t *testing.T) {
	tests := []struct {
		name      string
		patterns  []string
		wantCount int
		wantErr   bool
	}{
		{name: "nil input returns nil", patterns: nil, wantCount: 0},
		{name: "empty input returns nil", patterns: []string{}, wantCount: 0},
		{name: "single plain pattern", patterns: []string{"Bad Gateway"}, wantCount: 1},
		{name: "single slash-delimited pattern", patterns: []string{"/Bad Gateway/"}, wantCount: 1},
		{name: "multiple mixed patterns", patterns: []string{"/5\\d\\d /", "connection reset", "/timeout/"}, wantCount: 3},
		{name: "whitespace trimmed", patterns: []string{"  /Bad Gateway/  "}, wantCount: 1},
		{name: "empty pattern skipped", patterns: []string{"", "Bad Gateway", "  "}, wantCount: 1},
		{name: "invalid regex returns error", patterns: []string{"/(/"}, wantErr: true},
		{name: "valid then invalid returns error", patterns: []string{"good", "/(/"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CompileConditions(tc.patterns)
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrInvalidCondition), "error should wrap ErrInvalidCondition")
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tc.wantCount)
		})
	}
}

// TestMatchesAny verifies the OR-of-patterns matching behaviour over real Terraform-style
// transient-error strings (5xx, connection reset, Bad Gateway) and confirms matching is
// case-sensitive by default — users must opt in to case-insensitivity via (?i) in the regex.
func TestMatchesAny(t *testing.T) {
	patterns, err := CompileConditions([]string{"/Bad Gateway/", "/5\\d\\d /", "connection reset"})
	require.NoError(t, err)

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "empty output", output: "", want: false},
		{name: "no match", output: "permission denied", want: false},
		{name: "matches Bad Gateway literal", output: "Error: 502 Bad Gateway returned", want: true},
		{name: "matches 5xx regex", output: "got 503 Service Unavailable", want: true},
		{name: "matches connection reset", output: "read tcp: connection reset by peer", want: true},
		{name: "case sensitive by default", output: "bad gateway", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, MatchesAny(patterns, tc.output))
		})
	}
}

// TestMatchesAny_NilOrEmptyPatterns asserts the no-patterns short-circuit: an empty
// compiled slice (and a literal nil) must never match, regardless of the output content.
func TestMatchesAny_NilOrEmptyPatterns(t *testing.T) {
	empty, err := CompileConditions(nil)
	require.NoError(t, err)
	assert.False(t, MatchesAny(empty, "anything"))
	assert.False(t, MatchesAny(nil, "anything"))
}

// TestStripSlashDelimiters covers the YAML-readability shim that lets users write
// patterns as /.../ — a balanced pair is stripped, an unbalanced or single slash is
// preserved (so users still see a regex compile error rather than a silently mangled
// pattern), and surrounding whitespace is trimmed first.
func TestStripSlashDelimiters(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/foo/", "foo"},
		{"foo", "foo"},
		{"  /foo/  ", "foo"},
		{"/", "/"},
		{"", ""},
		{"/foo", "/foo"},
		{"foo/", "foo/"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, stripSlashDelimiters(tc.in))
		})
	}
}
