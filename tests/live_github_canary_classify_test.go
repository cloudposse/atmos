package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassifyLiveGitHubFailure covers every transient signature the live-GitHub canaries treat
// as "skip, not fail" (docs/fixes/2026-08-10-github-transient-error-tls-cert-flake.md's pattern,
// adapted to stderr text since these canaries drive a subprocess), plus non-transient cases that
// must still fail the build: a real 404 body and plain unrelated output.
func TestClassifyLiveGitHubFailure(t *testing.T) {
	tests := []struct {
		name          string
		stderr        string
		wantTransient bool
	}{
		{name: "DNS resolution failure", stderr: "ERRO Could not resolve host: github.com", wantTransient: true},
		{name: "connect failure", stderr: "fatal: unable to access '...': Failed to connect to github.com port 443", wantTransient: true},
		{name: "could not connect", stderr: "dial tcp: could not connect to github.com:443", wantTransient: true},
		{name: "connection refused", stderr: "dial tcp 140.82.112.3:443: connect: connection refused", wantTransient: true},
		{name: "connection reset", stderr: "read: connection reset by peer", wantTransient: true},
		{name: "timeout word", stderr: "context deadline exceeded (Client.Timeout exceeded)", wantTransient: true},
		{name: "timed out phrase", stderr: "dial tcp: i/o timeout: operation timed out", wantTransient: true},
		{name: "TLS failure", stderr: "tls: failed to verify certificate: x509: certificate signed by unknown authority", wantTransient: true},
		{name: "x509 only", stderr: "x509: certificate has expired or is not yet valid", wantTransient: true},
		{name: "HTTP 429", stderr: "GitHub API error: 429 Too Many Requests", wantTransient: true},
		{name: "rate limit phrase", stderr: "API rate limit exceeded for xxx.xxx.xxx.xxx", wantTransient: true},
		{name: "HTTP 500", stderr: "unexpected server error: 500 Internal Server Error", wantTransient: true},
		{name: "HTTP 503", stderr: "ghcr.io returned 503 Service Unavailable", wantTransient: true},
		{
			name:          "real 404 is not transient",
			stderr:        "Error: failed to download release asset: 404 Not Found",
			wantTransient: false,
		},
		{
			name:          "real 403 forbidden is not transient",
			stderr:        "Error: GET https://api.github.com/repos/x/y: 403 Forbidden: token has insufficient scope",
			wantTransient: false,
		},
		{
			name:          "unrelated atmos error is not transient",
			stderr:        "Error: component \"vpc\" not found in stack \"nonprod\"",
			wantTransient: false,
		},
		{name: "empty stderr is not transient", stderr: "", wantTransient: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transient, pattern := classifyLiveGitHubFailure(tt.stderr)
			assert.Equal(t, tt.wantTransient, transient)
			if tt.wantTransient {
				assert.NotEmpty(t, pattern, "expected a matched pattern name when transient")
			} else {
				assert.Empty(t, pattern)
			}
		})
	}
}

// TestSkipOrFailLiveGitHubCanary_NilError verifies a nil error is always a silent no-op,
// regardless of stderr content.
func TestSkipOrFailLiveGitHubCanary_NilError(t *testing.T) {
	skipOrFailLiveGitHubCanary(t, "no-op", nil, "500 Internal Server Error")
}

func TestRemoveEnvKeys(t *testing.T) {
	env := []string{"PATH=/usr/bin", "GITHUB_TOKEN=secret", "HOME=/home/test"}

	got := removeEnvKeys(env, "GITHUB_TOKEN")

	assert.Equal(t, []string{"PATH=/usr/bin", "HOME=/home/test"}, got)
}

func TestRemoveEnvKeys_ExactMatchOnly(t *testing.T) {
	env := []string{"GITHUB_TOKEN=secret", "GITHUB_TOKEN_EXTRA=keepme"}

	got := removeEnvKeys(env, "GITHUB_TOKEN")

	assert.Equal(t, []string{"GITHUB_TOKEN_EXTRA=keepme"}, got)
}

func TestRemoveEnvPrefixed(t *testing.T) {
	env := []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"PATH=/usr/bin",
	}

	got := removeEnvPrefixed(env, "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_")

	assert.Equal(t, []string{"PATH=/usr/bin"}, got)
}

func TestSetEnvVar(t *testing.T) {
	env := []string{"PATH=/usr/bin", "ATMOS_XDG_CACHE_HOME=/old"}

	got := setEnvVar(env, "ATMOS_XDG_CACHE_HOME", "/new")

	assert.Equal(t, []string{"PATH=/usr/bin", "ATMOS_XDG_CACHE_HOME=/new"}, got)
}

func TestSetEnvVar_AddsNewKey(t *testing.T) {
	env := []string{"PATH=/usr/bin"}

	got := setEnvVar(env, "ATMOS_XDG_CACHE_HOME", "/new")

	assert.Equal(t, []string{"PATH=/usr/bin", "ATMOS_XDG_CACHE_HOME=/new"}, got)
}

// TestGithubCanaryEnv_Unauthenticated verifies the unauthenticated branch blanks every GitHub
// token env var, sets GH_CONFIG_DIR, strips mirror insteadOf rules, and does not add the
// extraheader entry even when the ambient environment has a real token.
func TestGithubCanaryEnv_Unauthenticated(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "should-not-be-used")

	base := []string{
		"PATH=/usr/bin",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=url.file:///mirror/cloudposse/.insteadOf",
		"GIT_CONFIG_VALUE_0=https://github.com/cloudposse/",
	}

	env := githubCanaryEnv(t, base, false)

	assertEnvContains(t, env, "GITHUB_TOKEN=")
	assertEnvContains(t, env, "ATMOS_GITHUB_TOKEN=")
	assertEnvContains(t, env, "ATMOS_PRO_GITHUB_TOKEN=")
	assertEnvContains(t, env, "GH_TOKEN=")
	assertEnvHasPrefix(t, env, "GH_CONFIG_DIR=")
	assertEnvNotContains(t, env, "url.file:///mirror/cloudposse/.insteadOf")
	assertEnvNotContains(t, env, "http.https://github.com/.extraheader")
}

// TestGithubCanaryEnv_Authenticated verifies the authenticated branch keeps GITHUB_TOKEN, adds
// the extraheader entry, and still strips mirror insteadOf rules.
func TestGithubCanaryEnv_Authenticated(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test_token")

	base := []string{
		"PATH=/usr/bin",
		"GITHUB_TOKEN=ghp_test_token",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=url.file:///mirror/cloudposse/.insteadOf",
		"GIT_CONFIG_VALUE_0=https://github.com/cloudposse/",
	}

	env := githubCanaryEnv(t, base, true)

	assertEnvContains(t, env, "GITHUB_TOKEN=ghp_test_token")
	assertEnvNotContains(t, env, "url.file:///mirror/cloudposse/.insteadOf")

	found := false
	for _, kv := range env {
		if kv == "GIT_CONFIG_KEY_0=credential.helper" || kv == "GIT_CONFIG_KEY_1=http.https://github.com/.extraheader" || kv == "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader" {
			found = true
		}
	}
	require.True(t, found, "expected an extraheader git config key in %v", env)
}

func assertEnvContains(t *testing.T, env []string, want string) {
	t.Helper()
	for _, kv := range env {
		if kv == want {
			return
		}
	}
	t.Fatalf("expected env to contain %q, got %v", want, env)
}

func assertEnvHasPrefix(t *testing.T, env []string, prefix string) {
	t.Helper()
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return
		}
	}
	t.Fatalf("expected env to contain an entry with prefix %q, got %v", prefix, env)
}

func assertEnvNotContains(t *testing.T, env []string, substr string) {
	t.Helper()
	for _, kv := range env {
		if strings.Contains(kv, substr) {
			t.Fatalf("expected env to not contain an entry with %q, got %v", substr, env)
		}
	}
}
