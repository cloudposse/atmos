package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVersionSpec(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantType  VersionType
		wantValue string
		wantErr   bool
	}{
		// Explicit PR prefix.
		{
			name:      "explicit pr prefix",
			input:     "pr:2040",
			wantType:  VersionTypePR,
			wantValue: "2040",
			wantErr:   false,
		},

		// Auto-detect PR (all digits).
		{
			name:      "auto-detect PR single digit",
			input:     "1",
			wantType:  VersionTypePR,
			wantValue: "1",
			wantErr:   false,
		},
		{
			name:      "auto-detect PR multiple digits",
			input:     "2040",
			wantType:  VersionTypePR,
			wantValue: "2040",
			wantErr:   false,
		},
		{
			name:      "auto-detect PR large number",
			input:     "999999",
			wantType:  VersionTypePR,
			wantValue: "999999",
			wantErr:   false,
		},
		{
			name:      "auto-detect PR seven digits",
			input:     "1234567",
			wantType:  VersionTypePR,
			wantValue: "1234567",
			wantErr:   false,
		},

		// Valid semver.
		{
			name:      "semver with dots",
			input:     "1.2.3",
			wantType:  VersionTypeSemver,
			wantValue: "1.2.3",
			wantErr:   false,
		},
		{
			name:      "semver with v prefix",
			input:     "v1.2.3",
			wantType:  VersionTypeSemver,
			wantValue: "v1.2.3",
			wantErr:   false,
		},
		{
			name:      "semver three part version",
			input:     "v1.175.0",
			wantType:  VersionTypeSemver,
			wantValue: "v1.175.0",
			wantErr:   false,
		},
		{
			name:      "semver two part version",
			input:     "1.0",
			wantType:  VersionTypeSemver,
			wantValue: "1.0",
			wantErr:   false,
		},
		{
			name:      "latest keyword",
			input:     "latest",
			wantType:  VersionTypeSemver,
			wantValue: "latest",
			wantErr:   false,
		},

		// Invalid formats (should error).
		{
			name:      "empty string",
			input:     "",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "short hex string (abc)",
			input:     "abc",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "hex string 6 chars (abc123)",
			input:     "abc123",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "hex string 7 chars (looks like SHA)",
			input:     "ceb7526",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "hex string 8 chars",
			input:     "ceb75261",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "long hex string (40 chars)",
			input:     "ceb752612345678901234567890123456789abcd",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "uppercase letters",
			input:     "ABCDEFG",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "mixed case",
			input:     "AbCdEfG",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "non-hex letters",
			input:     "ghijklm",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "random word",
			input:     "foobar",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotValue, err := ParseVersionSpec(tt.input)
			if tt.wantErr {
				require.Error(t, err, "expected error but got none")
				assert.Equal(t, VersionTypeInvalid, gotType, "type should be invalid on error")
				return
			}
			require.NoError(t, err, "unexpected error")
			assert.Equal(t, tt.wantType, gotType, "type mismatch")
			assert.Equal(t, tt.wantValue, gotValue, "value mismatch")
		})
	}
}

func TestIsPRVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		wantPR   int
		wantIsPR bool
	}{
		// Valid PR versions.
		{
			name:     "explicit pr prefix",
			version:  "pr:2040",
			wantPR:   2040,
			wantIsPR: true,
		},
		{
			name:     "auto-detect PR",
			version:  "2040",
			wantPR:   2040,
			wantIsPR: true,
		},
		{
			name:     "auto-detect single digit PR",
			version:  "1",
			wantPR:   1,
			wantIsPR: true,
		},
		{
			name:     "large PR number",
			version:  "12345",
			wantPR:   12345,
			wantIsPR: true,
		},

		// Invalid PR versions.
		{
			name:     "semver",
			version:  "1.2.3",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr:0 invalid",
			version:  "pr:0",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr:-1 invalid",
			version:  "pr:-1",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "pr:abc invalid",
			version:  "pr:abc",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "empty string",
			version:  "",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "invalid format (abc)",
			version:  "abc",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "invalid format (abc123)",
			version:  "abc123",
			wantPR:   0,
			wantIsPR: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPR, gotIsPR := IsPRVersion(tt.version)
			assert.Equal(t, tt.wantPR, gotPR, "PR number mismatch")
			assert.Equal(t, tt.wantIsPR, gotIsPR, "isPR mismatch")
		})
	}
}

func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"123", true},
		{"0", true},
		{"12345678901234567890", true},
		{"", false},
		{"12a", false},
		{"a12", false},
		{"1.2", false},
		{"-1", false},
		{" 1", false},
		{"1 ", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isAllDigits(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsValidSemver(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid semver patterns.
		{"1.2.3", true},
		{"v1.2.3", true},
		{"1.0", true},
		{"v1.0", true},
		{"0.0.1", true},
		{"v0.0.1", true},
		{"1.175.0", true},
		{"v1.175.0", true},
		{"latest", true}, // Special keyword.

		// Invalid patterns.
		{"abc", false},
		{"abc123", false},
		{"ceb7526", false},
		{"v", false},
		{"1", false},
		{"v1", false},
		{"", false},
		{"1.", false},
		{".1", false},
		{"1..2", false},
		{"1.2.3.4.5", true}, // Valid - multiple parts allowed.
		{"1.2.a", false},    // Non-numeric part.
		{"1.2-beta", false}, // Pre-release suffix not supported in simple check.
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidSemver(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
