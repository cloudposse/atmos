package tests

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/tests/testhelpers/gitconfigenv"
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
		{
			name:          "near-miss: bare tls word in a semantic atmos error is not transient",
			stderr:        "Error: variable \"tls\" is not defined in stack \"nonprod\"",
			wantTransient: false,
		},
		{
			name:          "near-miss: bare timeout word in a semantic atmos error is not transient",
			stderr:        "Error: hook \"timeout\" must be a positive duration in stack \"nonprod\"",
			wantTransient: false,
		},
		{
			name:          "near-miss: bare 500-509 identifier in a semantic atmos error is not transient",
			stderr:        "Error: line 500: unexpected token in stack \"nonprod\"",
			wantTransient: false,
		},
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
	skipOrFailLiveGitHubCanary(t, context.Background(), "no-op", nil, "500 Internal Server Error")
}

// TestClassifyCanaryFailure covers classifyCanaryFailure's extra ctxErr branch on top of
// classifyLiveGitHubFailure's stderr-only patterns: exec.CommandContext kills a hung subprocess
// the instant canaryTimeout fires, which can leave stderr empty (or mid-write), so a deadline
// exceeded on the driving context must be treated as transient even when stderr has nothing to
// say -- and must NOT be misclassified when the context error is something else entirely (e.g.
// explicit cancellation, which is a real bug in the canary, not a network condition).
func TestClassifyCanaryFailure(t *testing.T) {
	tests := []struct {
		name          string
		ctxErr        error
		stderr        string
		wantTransient bool
	}{
		{
			name:          "deadline exceeded with empty stderr is transient",
			ctxErr:        context.DeadlineExceeded,
			stderr:        "",
			wantTransient: true,
		},
		{
			name:          "deadline exceeded with unrelated stderr is still transient",
			ctxErr:        context.DeadlineExceeded,
			stderr:        "Error: component \"vpc\" not found in stack \"nonprod\"",
			wantTransient: true,
		},
		{
			name:          "wrapped deadline exceeded is transient",
			ctxErr:        fmt.Errorf("running command: %w", context.DeadlineExceeded),
			stderr:        "",
			wantTransient: true,
		},
		{
			name:          "no ctx error falls back to stderr classification (transient)",
			ctxErr:        nil,
			stderr:        "dial tcp: could not connect to github.com:443",
			wantTransient: true,
		},
		{
			name:          "no ctx error falls back to stderr classification (not transient)",
			ctxErr:        nil,
			stderr:        "Error: component \"vpc\" not found in stack \"nonprod\"",
			wantTransient: false,
		},
		{
			name:          "context canceled (not a deadline) with unrelated stderr is not transient",
			ctxErr:        context.Canceled,
			stderr:        "Error: component \"vpc\" not found in stack \"nonprod\"",
			wantTransient: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transient, reason := classifyCanaryFailure(tt.ctxErr, tt.stderr)
			assert.Equal(t, tt.wantTransient, transient)
			if tt.wantTransient {
				assert.NotEmpty(t, reason, "expected a reason when transient")
			} else {
				assert.Empty(t, reason)
			}
		})
	}
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

// mirrorGitConfigEnv is the GIT_CONFIG_GLOBAL entry TestMain exports for the local git mirror's
// insteadOf rules (tests/testhelpers/gitmirror.WriteGitConfig); canaries must replace it.
const mirrorGitConfigEnv = "GIT_CONFIG_GLOBAL=/mirror/gitconfig"

// assertMirrorRulesDisabled verifies env points GIT_CONFIG_GLOBAL at an empty file (so the
// mirror's insteadOf rules do not apply) rather than at TestMain's mirror gitconfig, and that
// GIT_CONFIG_NOSYSTEM=true is set so a system-level git config (e.g. /etc/gitconfig) cannot
// redirect or authenticate the canary either.
func assertMirrorRulesDisabled(t *testing.T, env []string) {
	t.Helper()

	assertEnvNotContains(t, env, mirrorGitConfigEnv)
	assertEnvContains(t, env, "GIT_CONFIG_NOSYSTEM=true")
	for _, kv := range env {
		if path, ok := strings.CutPrefix(kv, "GIT_CONFIG_GLOBAL="); ok {
			info, err := os.Stat(path)
			require.NoError(t, err, "GIT_CONFIG_GLOBAL must point at an existing file")
			require.Zero(t, info.Size(), "GIT_CONFIG_GLOBAL must point at an EMPTY gitconfig")
			return
		}
	}
	t.Fatalf("expected env to set GIT_CONFIG_GLOBAL, got %v", env)
}

// TestGithubCanaryEnv_Unauthenticated verifies the unauthenticated branch blanks every GitHub
// token env var, sets GH_CONFIG_DIR, disables the mirror's insteadOf rules, and does not add the
// extraheader entry even when the ambient environment has a real token.
func TestGithubCanaryEnv_Unauthenticated(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "should-not-be-used")

	base := []string{
		"PATH=/usr/bin",
		mirrorGitConfigEnv,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
	}

	env := githubCanaryEnv(t, base, false)

	assertEnvContains(t, env, "GITHUB_TOKEN=")
	assertEnvContains(t, env, "ATMOS_GITHUB_TOKEN=")
	assertEnvContains(t, env, "ATMOS_PRO_GITHUB_TOKEN=")
	assertEnvContains(t, env, "GH_TOKEN=")
	assertEnvHasPrefix(t, env, "GH_CONFIG_DIR=")
	assertMirrorRulesDisabled(t, env)
	// Pre-existing git config entries are preserved, not dropped.
	assertEnvContains(t, env, "GIT_CONFIG_KEY_0=credential.helper")
	assertEnvNotContains(t, env, "http.https://github.com/.extraheader")
}

// TestGithubCanaryEnv_Authenticated verifies the authenticated branch keeps GITHUB_TOKEN, adds
// the extraheader entry, forces GIT_TRACE_REDACT=true, still disables the mirror's insteadOf
// rules, and -- critically -- drops any inherited http.*.extraheader entry rather than sending it
// alongside the controlled one. Git sends every repeated http.extraHeader value it is given, so an
// inherited Authorization header left in place would let the canary authenticate with unintended
// credentials.
func TestGithubCanaryEnv_Authenticated(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test_token")

	base := []string{
		"PATH=/usr/bin",
		"GITHUB_TOKEN=ghp_test_token",
		mirrorGitConfigEnv,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic inherited-should-not-survive",
	}

	env := githubCanaryEnv(t, base, true)

	assertEnvContains(t, env, "GITHUB_TOKEN=ghp_test_token")
	assertEnvContains(t, env, "GIT_TRACE_REDACT=true")
	assertMirrorRulesDisabled(t, env)

	var extraHeaders []string
	for _, entry := range gitconfigenv.ReadEntries(env) {
		if entry.Key == "http.https://github.com/.extraheader" {
			extraHeaders = append(extraHeaders, entry.Value)
		}
	}
	require.Len(t, extraHeaders, 1, "expected exactly one http.https://github.com/.extraheader entry in %v", env)
	assert.NotContains(t, extraHeaders, "AUTHORIZATION: basic inherited-should-not-survive",
		"inherited extraheader entry must be dropped, not sent alongside the controlled one")
	assert.NotEqual(t, "AUTHORIZATION: basic inherited-should-not-survive", extraHeaders[0])
}

// TestGithubCanaryEnv_Authenticated_RegistersTokenForMasking verifies that the authenticated
// branch registers the injected credential with the global masker, so
// skipOrFailLiveGitHubCanary's iolib.MaskString call actually redacts the token (and its
// basic-auth encoding) from captured stderr before it is classified or logged.
func TestGithubCanaryEnv_Authenticated_RegistersTokenForMasking(t *testing.T) {
	const token = "ghp_super_secret_test_token"
	t.Setenv("GITHUB_TOKEN", token)

	base := []string{"PATH=/usr/bin", "GITHUB_TOKEN=" + token, mirrorGitConfigEnv}

	env := githubCanaryEnv(t, base, true)

	var basicAuth string
	for _, entry := range gitconfigenv.ReadEntries(env) {
		if entry.Key == "http.https://github.com/.extraheader" {
			basicAuth = strings.TrimPrefix(entry.Value, "AUTHORIZATION: basic ")
			break
		}
	}
	require.NotEmpty(t, basicAuth, "expected an http.https://github.com/.extraheader entry in %v", env)

	stderr := "fatal: unable to access 'https://x-access-token:" + token + "@github.com/x/y.git/': " +
		"The requested URL returned error: 401\nAuthorization: basic " + basicAuth
	require.Contains(t, stderr, token, "sanity check: fixture stderr should contain the raw token before masking")

	masked := iolib.MaskString(stderr)
	assert.NotContains(t, masked, token, "masked stderr must not contain the raw token")
	assert.NotContains(t, masked, basicAuth, "masked stderr must not contain the base64-encoded credential")
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
