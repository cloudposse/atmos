package aqua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateVersionConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraint  string
		version     string // Full version (passed as Version).
		semver      string // SemVer-stripped version (passed as SemVer). If empty, uses version.
		want        bool
		wantErr     bool
		errContains string
	}{
		// Literal true/false.
		{
			name:       "literal true matches",
			constraint: "true",
			version:    "1.2.3",
			want:       true,
		},
		{
			name:       "literal true quoted matches",
			constraint: `"true"`,
			version:    "1.2.3",
			want:       true,
		},
		{
			name:       "literal false does not match",
			constraint: "false",
			version:    "1.2.3",
			want:       false,
		},
		{
			name:       "literal false quoted does not match",
			constraint: `"false"`,
			version:    "1.2.3",
			want:       false,
		},

		// Exact version match.
		{
			name:       "exact version match with v prefix",
			constraint: `Version == "v1.2.3"`,
			version:    "v1.2.3",
			want:       true,
		},
		{
			name:       "exact version match without v prefix",
			constraint: `Version == "1.2.3"`,
			version:    "1.2.3",
			want:       true,
		},
		{
			name:       "exact version mismatch",
			constraint: `Version == "v1.2.3"`,
			version:    "v1.2.4",
			want:       false,
		},
		{
			name:       "exact version with spaces",
			constraint: `Version ==   "v1.2.3"  `,
			version:    "v1.2.3",
			want:       true,
		},

		// Semver constraints.
		{
			name:       "semver greater than or equal",
			constraint: `semver(">= 1.0.0")`,
			version:    "1.2.3",
			want:       true,
		},
		{
			name:       "semver greater than or equal with v prefix in version",
			constraint: `semver(">= 1.0.0")`,
			version:    "v1.2.3",
			semver:     "1.2.3",
			want:       true,
		},
		{
			name:       "semver less than or equal",
			constraint: `semver("<= 1.5.0")`,
			version:    "1.2.3",
			want:       true,
		},
		{
			name:       "semver less than or equal fails",
			constraint: `semver("<= 1.5.0")`,
			version:    "2.0.0",
			want:       false,
		},
		{
			name:       "semver range",
			constraint: `semver(">= 1.0.0, < 2.0.0")`,
			version:    "1.5.0",
			want:       true,
		},
		{
			name:       "semver range fails",
			constraint: `semver(">= 1.0.0, < 2.0.0")`,
			version:    "2.1.0",
			want:       false,
		},
		{
			name:       "semver with outer spaces",
			constraint: `  semver(">= 1.0.0")  `,
			version:    "1.2.3",
			want:       true,
		},

		// Error and edge cases.
		// With expr-lang, semver() returns bool (false) instead of erroring for bad input.
		{
			name:       "invalid semver constraint returns false",
			constraint: `semver("not a constraint")`,
			version:    "1.2.3",
			want:       false,
		},
		{
			name:       "invalid version format returns false",
			constraint: `semver(">= 1.0.0")`,
			version:    "not-a-version",
			want:       false,
		},
		{
			name:        "unsupported constraint format",
			constraint:  `unknown("something")`,
			version:     "1.2.3",
			wantErr:     true,
			errContains: "unsupported version constraint",
		},

		// Real-world cases from Aqua registry.
		{
			name:       "terraform-backend-git catch-all",
			constraint: "true",
			version:    "0.1.8",
			want:       true,
		},
		{
			name:       "opentofu v1.6.0-beta4 or earlier",
			constraint: `semver("<= 1.6.0-beta4")`,
			version:    "1.6.0-beta3",
			want:       true,
		},
		{
			name:       "opentofu v1.6.0-rc1 or earlier",
			constraint: `semver("<= 1.6.0-rc1")`,
			version:    "1.6.0-beta5",
			want:       true,
		},
		{
			name:       "opentofu catch-all for newer versions",
			constraint: "true",
			version:    "1.10.7",
			want:       true,
		},

		// Version inequality (expr-lang).
		{
			name:       "version not equal - mismatch returns true",
			constraint: `Version != "v1.0.0"`,
			version:    "v2.0.0",
			want:       true,
		},
		{
			name:       "version not equal - match returns false",
			constraint: `Version != "v1.0.0"`,
			version:    "v1.0.0",
			want:       false,
		},

		// Compound expressions (expr-lang).
		{
			name:       "compound OR - first match",
			constraint: `Version == "v1.0" || Version == "v2.0"`,
			version:    "v1.0",
			want:       true,
		},
		{
			name:       "compound OR - second match",
			constraint: `Version == "v1.0" || Version == "v2.0"`,
			version:    "v2.0",
			want:       true,
		},
		{
			name:       "compound OR - no match",
			constraint: `Version == "v1.0" || Version == "v2.0"`,
			version:    "v3.0",
			want:       false,
		},

		// semverWithVersion + trimPrefix (expr-lang).
		{
			name:       "semverWithVersion with trimPrefix",
			constraint: `semverWithVersion(">= 0.11.1", trimPrefix("cli-", Version))`,
			version:    "cli-0.12.0",
			want:       true,
		},
		{
			name:       "semverWithVersion with trimPrefix below range",
			constraint: `semverWithVersion(">= 0.11.1", trimPrefix("cli-", Version))`,
			version:    "cli-0.10.0",
			want:       false,
		},

		// Commit hash handling.
		{
			name:       "commit hash returns false for semver constraint",
			constraint: `semver(">= 1.0.0")`,
			version:    "abc1234567890abcdef1234567890abcdef123456",
			want:       false,
		},

		// String operations (expr-lang built-in).
		{
			name:       "version startsWith match",
			constraint: `Version startsWith "go"`,
			version:    "go1.21.0",
			want:       true,
		},
		{
			name:       "version startsWith no match",
			constraint: `Version startsWith "go"`,
			version:    "v1.21.0",
			want:       false,
		},
		{
			name:       "version contains match",
			constraint: `Version contains "rc"`,
			version:    "v1.0.0-rc1",
			want:       true,
		},
		{
			name:       "version contains no match",
			constraint: `Version contains "rc"`,
			version:    "v1.0.0",
			want:       false,
		},

		// B2: SemVer variable tests - Version and SemVer should be different.
		{
			name:       "SemVer variable with custom prefix",
			constraint: `SemVer == "1.7.1"`,
			version:    "jq-1.7.1",
			semver:     "1.7.1",
			want:       true,
		},
		{
			name:       "semver function uses SemVer not Version",
			constraint: `semver(">= 1.0.0")`,
			version:    "jq-1.7.1",
			semver:     "1.7.1",
			want:       true,
		},
		{
			name:       "Version keeps full prefix",
			constraint: `Version == "jq-1.7.1"`,
			version:    "jq-1.7.1",
			semver:     "1.7.1",
			want:       true,
		},
		{
			name:       "semver not equal",
			constraint: `semver("!= 1.0.0")`,
			version:    "2.0.0",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := tt.semver
			if sv == "" {
				sv = tt.version
			}
			got, err := evaluateVersionConstraint(tt.constraint, tt.version, sv)
			if tt.wantErr {
				require.Error(t, err, "expected error but got none")
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains, "error message should contain expected string")
				}
				return
			}
			require.NoError(t, err, "unexpected error")
			assert.Equal(t, tt.want, got, "constraint evaluation result mismatch")
		})
	}
}

func TestEvaluateVersionConstraint_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		version    string
		semver     string
		want       bool
	}{
		{
			name:       "empty constraint string",
			constraint: "",
			version:    "1.2.3",
			want:       false,
		},
		{
			name:       "whitespace only constraint",
			constraint: "   ",
			version:    "1.2.3",
			want:       false,
		},
		{
			name:       "semver with prerelease",
			constraint: `semver(">= 1.0.0-beta")`,
			version:    "1.0.0-beta.2",
			want:       true,
		},
		{
			name:       "semver with build metadata",
			constraint: `semver(">= 1.0.0")`,
			version:    "1.0.0+build.123",
			want:       true,
		},
		{
			name:       "commit hash with semverWithVersion returns false",
			constraint: `semverWithVersion(">= 1.0.0", Version)`,
			version:    "abc1234567890abcdef1234567890abcdef123456",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := tt.semver
			if sv == "" {
				sv = tt.version
			}
			got, _ := evaluateVersionConstraint(tt.constraint, tt.version, sv)
			assert.Equal(t, tt.want, got)
		})
	}
}
