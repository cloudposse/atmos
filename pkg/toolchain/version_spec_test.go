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

		// Explicit SHA prefix.
		{
			name:      "explicit sha prefix short",
			input:     "sha:ceb7526",
			wantType:  VersionTypeSHA,
			wantValue: "ceb7526",
			wantErr:   false,
		},
		{
			name:      "explicit sha prefix full",
			input:     "sha:ceb7526be123456789abcdef0123456789abcdef",
			wantType:  VersionTypeSHA,
			wantValue: "ceb7526be123456789abcdef0123456789abcdef",
			wantErr:   false,
		},

		// Auto-detect SHA (7-40 hex chars with at least one letter a-f).
		{
			name:      "auto-detect SHA 7 chars",
			input:     "ceb7526",
			wantType:  VersionTypeSHA,
			wantValue: "ceb7526",
			wantErr:   false,
		},
		{
			name:      "auto-detect SHA 8 chars",
			input:     "ceb75261",
			wantType:  VersionTypeSHA,
			wantValue: "ceb75261",
			wantErr:   false,
		},
		{
			name:      "auto-detect SHA full 40 chars",
			input:     "ceb752612345678901234567890123456789abcd",
			wantType:  VersionTypeSHA,
			wantValue: "ceb752612345678901234567890123456789abcd",
			wantErr:   false,
		},
		{
			name:      "auto-detect SHA with all hex digits",
			input:     "a1b2c3d",
			wantType:  VersionTypeSHA,
			wantValue: "a1b2c3d",
			wantErr:   false,
		},
		{
			name:      "auto-detect SHA only letters a-f",
			input:     "abcdefab",
			wantType:  VersionTypeSHA,
			wantValue: "abcdefab",
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
			name:      "short hex string (abc) - too short for SHA",
			input:     "abc",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "hex string 6 chars (abc123) - too short for SHA",
			input:     "abc123",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "uppercase letters (invalid SHA)",
			input:     "ABCDEFG",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "mixed case (invalid SHA)",
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
		{
			name:      "hex string over 40 chars",
			input:     "ceb752612345678901234567890123456789abcde1",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},

		// Invalid PR formats.
		{
			name:      "pr:abc - non-numeric PR value",
			input:     "pr:abc",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "pr:0 - zero PR number",
			input:     "pr:0",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "pr:-1 - negative PR number",
			input:     "pr:-1",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "pr: - empty PR value",
			input:     "pr:",
			wantType:  VersionTypeInvalid,
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "0 - zero as PR number",
			input:     "0",
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
			name:     "invalid format (abc) - too short for SHA",
			version:  "abc",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "invalid format (abc123) - too short for SHA",
			version:  "abc123",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "SHA version (not PR)",
			version:  "ceb7526",
			wantPR:   0,
			wantIsPR: false,
		},
		{
			name:     "explicit SHA prefix (not PR)",
			version:  "sha:ceb7526",
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

func TestIsSHAVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantSHA   string
		wantIsSHA bool
	}{
		// Valid SHA versions.
		{
			name:      "explicit sha prefix",
			version:   "sha:ceb7526",
			wantSHA:   "ceb7526",
			wantIsSHA: true,
		},
		{
			name:      "auto-detect SHA",
			version:   "ceb7526",
			wantSHA:   "ceb7526",
			wantIsSHA: true,
		},
		{
			name:      "auto-detect SHA 8 chars",
			version:   "ceb75261",
			wantSHA:   "ceb75261",
			wantIsSHA: true,
		},
		{
			name:      "full 40 char SHA",
			version:   "ceb752612345678901234567890123456789abcd",
			wantSHA:   "ceb752612345678901234567890123456789abcd",
			wantIsSHA: true,
		},

		// Invalid SHA versions.
		{
			name:      "semver",
			version:   "1.2.3",
			wantSHA:   "",
			wantIsSHA: false,
		},
		{
			name:      "PR number",
			version:   "2040",
			wantSHA:   "",
			wantIsSHA: false,
		},
		{
			name:      "explicit PR prefix",
			version:   "pr:2040",
			wantSHA:   "",
			wantIsSHA: false,
		},
		{
			name:      "empty string",
			version:   "",
			wantSHA:   "",
			wantIsSHA: false,
		},
		{
			name:      "too short (abc)",
			version:   "abc",
			wantSHA:   "",
			wantIsSHA: false,
		},
		{
			name:      "uppercase letters",
			version:   "ABCDEFG",
			wantSHA:   "",
			wantIsSHA: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSHA, gotIsSHA := IsSHAVersion(tt.version)
			assert.Equal(t, tt.wantSHA, gotSHA, "SHA mismatch")
			assert.Equal(t, tt.wantIsSHA, gotIsSHA, "isSHA mismatch")
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

func TestIsValidSHA(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid SHAs (7-40 hex chars with at least one letter a-f).
		{"ceb7526", true},              // 7 chars with letter.
		{"ceb75261", true},             // 8 chars with letter.
		{"a1b2c3d", true},              // 7 chars mixed.
		{"abcdefab", true},             // 8 chars only letters a-f.
		{"1234567a", true},             // 8 chars, one letter at end.
		{"a1234567", true},             // 8 chars, one letter at start.
		{"1a2b3c4d5e6f7a8b9c0d", true}, // 20 chars.
		{"ceb752612345678901234567890123456789abcd", true}, // Full 40 chars.

		// Invalid SHAs.
		{"abc", false},      // Too short (3 chars).
		{"abc123", false},   // Too short (6 chars).
		{"1234567", false},  // 7 chars but no letter a-f (all digits -> PR).
		{"12345678", false}, // 8 chars but no letter a-f (all digits -> PR).
		{"ABCDEFG", false},  // Uppercase not allowed.
		{"AbCdEfG", false},  // Mixed case not allowed.
		{"ghijklm", false},  // Non-hex letters.
		{"ceb752612345678901234567890123456789abcde1", false}, // 41 chars, too long.
		{"", false}, // Empty string.
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidSHA(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
