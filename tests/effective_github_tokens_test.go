package tests

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEffectiveGitHubTokens covers the token-resolution logic that decides which GitHub tokens
// the local git mirror config must cover for a given test case: fixture-only, process-only,
// both (with de-duplication of an identical value), and the all-empty case.
func TestEffectiveGitHubTokens(t *testing.T) {
	tests := []struct {
		name     string
		tcEnv    map[string]string
		lookup   map[string]string
		expected []string
	}{
		{
			name:     "no tokens anywhere returns empty",
			tcEnv:    map[string]string{},
			lookup:   map[string]string{},
			expected: nil,
		},
		{
			name:     "process-only token is included",
			tcEnv:    map[string]string{},
			lookup:   map[string]string{"GITHUB_TOKEN": "process-token"},
			expected: []string{"process-token"},
		},
		{
			name:     "fixture-only token is included",
			tcEnv:    map[string]string{"GITHUB_TOKEN": "fixture-token"},
			lookup:   map[string]string{},
			expected: []string{"fixture-token"},
		},
		{
			name:     "fixture token distinct from process token: both included",
			tcEnv:    map[string]string{"GITHUB_TOKEN": "fixture-token"},
			lookup:   map[string]string{"GITHUB_TOKEN": "process-token"},
			expected: []string{"process-token", "fixture-token"},
		},
		{
			name:     "identical fixture and process token is de-duplicated",
			tcEnv:    map[string]string{"GITHUB_TOKEN": "same-token"},
			lookup:   map[string]string{"GITHUB_TOKEN": "same-token"},
			expected: []string{"same-token"},
		},
		{
			name: "distinct tokens across all three env vars are all included",
			tcEnv: map[string]string{
				"ATMOS_PRO_GITHUB_TOKEN": "fixture-pro-token",
			},
			lookup: map[string]string{
				"GITHUB_TOKEN":       "process-github-token",
				"ATMOS_GITHUB_TOKEN": "process-atmos-token",
			},
			expected: []string{"process-github-token", "process-atmos-token", "fixture-pro-token"},
		},
		{
			name:     "empty-string values in both sources are ignored",
			tcEnv:    map[string]string{"GITHUB_TOKEN": ""},
			lookup:   map[string]string{"GITHUB_TOKEN": ""},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(key string) string { return tt.lookup[key] }
			got := effectiveGitHubTokens(tt.tcEnv, lookup)
			require.Equal(t, tt.expected, got)
		})
	}
}

// TestEnvHasKey verifies the exact match on every OS and the case-insensitive match that only
// applies on Windows, where os/exec resolves environment names case-insensitively.
func TestEnvHasKey(t *testing.T) {
	env := map[string]string{"git_config_global": "/tmp/x", "OTHER": "1"}

	require.True(t, envHasKey(env, "git_config_global"))
	require.False(t, envHasKey(env, "MISSING"))
	if runtime.GOOS == "windows" {
		require.True(t, envHasKey(env, "GIT_CONFIG_GLOBAL"))
	} else {
		require.False(t, envHasKey(env, "GIT_CONFIG_GLOBAL"))
	}
}
