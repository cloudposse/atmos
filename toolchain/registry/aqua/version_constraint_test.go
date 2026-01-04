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
		version     string
		want        bool
		wantErr     bool
		errContains string
	}{
		// Literal true/false
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

		// Exact version match
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

		// Semver constraints
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
			name:       "semver exact match",
			constraint: `semver("1.2.3")`,
			version:    "1.2.3",
			want:       true,
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

		// Error cases
		{
			name:        "invalid semver constraint",
			constraint:  `semver("not a constraint")`,
			version:     "1.2.3",
			wantErr:     true,
			errContains: "invalid semver constraint",
		},
		{
			name:        "invalid version format",
			constraint:  `semver(">= 1.0.0")`,
			version:     "not-a-version",
			wantErr:     true,
			errContains: "invalid version",
		},
		{
			name:        "unsupported constraint format",
			constraint:  `unknown("something")`,
			version:     "1.2.3",
			wantErr:     true,
			errContains: "unsupported version constraint format",
		},

		// Real-world cases from Aqua registry
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evaluateVersionConstraint(tt.constraint, tt.version)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := evaluateVersionConstraint(tt.constraint, tt.version)
			assert.Equal(t, tt.want, got)
		})
	}
}
