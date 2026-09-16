package tests

import (
	"os"
	"path/filepath"
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

// TestHomeFilesToCopy verifies an unauthenticated "live_github" case never copies .netrc into the
// test's isolated HOME (a real one on the host/CI runner would otherwise silently authenticate
// git's HTTP transport, defeating the point of the unauthenticated case), while every other case
// -- including "live_github_authenticated", which passes liveGitHub=false here -- keeps copying it
// alongside .gitconfig/.ssh.
func TestHomeFilesToCopy(t *testing.T) {
	tests := []struct {
		name       string
		liveGitHub bool
		want       []string
	}{
		{name: "live_github omits .netrc", liveGitHub: true, want: []string{".gitconfig", ".ssh"}},
		{name: "default case keeps .netrc", liveGitHub: false, want: []string{".gitconfig", ".ssh", ".netrc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := homeFilesToCopy(tt.liveGitHub)
			assert.Equal(t, tt.want, got)
			if tt.liveGitHub {
				assert.NotContains(t, got, ".netrc")
			}
		})
	}
}

func TestValidateLiveGitHubPreconditions(t *testing.T) {
	for _, tc := range []struct {
		name          string
		preconditions []string
		invalid       bool
	}{
		{name: "no preconditions"},
		{name: "unrelated preconditions", preconditions: []string{"terraform", "github_token"}},
		{name: "unauthenticated", preconditions: []string{"terraform", "live_github"}},
		{name: "authenticated", preconditions: []string{"live_github_authenticated"}},
		{name: "combined", preconditions: []string{"live_github", "live_github_authenticated"}, invalid: true},
		{name: "combined reversed", preconditions: []string{"live_github_authenticated", "terraform", "live_github"}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLiveGitHubPreconditions(tc.preconditions)
			if tc.invalid {
				require.EqualError(t, err, `preconditions "live_github" and "live_github_authenticated" are mutually exclusive`)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRequireNetrcFreeHome(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hasNetrc  bool
		emptyHome bool
		wantSkip  bool
	}{
		{name: "no netrc"},
		{name: "existing netrc", hasNetrc: true, wantSkip: true},
		{name: "unknown home", emptyHome: true, wantSkip: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			homeDir := t.TempDir()
			if tc.hasNetrc {
				require.NoError(t, os.WriteFile(filepath.Join(homeDir, ".netrc"), nil, 0o600))
			}
			if tc.emptyHome {
				homeDir = ""
			}
			var skipped, continued bool
			t.Run("canary", func(t *testing.T) {
				defer func() { skipped = t.Skipped() }()
				requireNetrcFreeHome(t, homeDir)
				continued = true
			})
			assert.Equal(t, tc.wantSkip, skipped)
			assert.Equal(t, !tc.wantSkip, continued)
		})
	}
}
