package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasPrecondition(t *testing.T) {
	tests := []struct {
		name          string
		preconditions []string
		want          string
		expect        bool
	}{
		{name: "present", preconditions: []string{"terraform", "live_github"}, want: "live_github", expect: true},
		{name: "absent", preconditions: []string{"terraform"}, want: "live_github", expect: false},
		{name: "empty list", preconditions: nil, want: "live_github", expect: false},
		{
			name:          "does not match a different live-github precondition",
			preconditions: []string{"live_github_authenticated"},
			want:          "live_github",
			expect:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, hasPrecondition(tt.preconditions, tt.want))
		})
	}
}

// TestScrubGitHubAuth verifies scrubGitHubAuth blanks every GitHub token env var atmos's
// git/HTTP clients read and points GH_CONFIG_DIR at a fresh, empty directory, generically -- for
// any YAML-driven test case whose preconditions include "live_github" -- without touching
// unrelated env entries already present on the test case.
func TestScrubGitHubAuth(t *testing.T) {
	tc := &TestCase{
		Preconditions: []string{"live_github"},
		Env: map[string]string{
			"GITHUB_TOKEN":  "should-be-blanked",
			"UNRELATED_VAR": "keep-me",
		},
	}

	scrubGitHubAuth(t, tc)

	assert.Equal(t, "", tc.Env["GITHUB_TOKEN"])
	assert.Equal(t, "", tc.Env["ATMOS_GITHUB_TOKEN"])
	assert.Equal(t, "", tc.Env["ATMOS_PRO_GITHUB_TOKEN"])
	assert.Equal(t, "", tc.Env["GH_TOKEN"])
	assert.Equal(t, "keep-me", tc.Env["UNRELATED_VAR"])
	require.NotEmpty(t, tc.Env["GH_CONFIG_DIR"])
}

// TestScrubGitHubAuth_NilEnv verifies scrubGitHubAuth initializes a nil Env map instead of
// panicking -- a YAML test case with no `env:` block unmarshals to a nil map.
func TestScrubGitHubAuth_NilEnv(t *testing.T) {
	tc := &TestCase{Preconditions: []string{"live_github"}}

	scrubGitHubAuth(t, tc)

	require.NotNil(t, tc.Env)
	assert.Equal(t, "", tc.Env["GITHUB_TOKEN"])
}
