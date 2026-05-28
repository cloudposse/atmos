package downloader

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitNetworkConfigArgs_AlwaysIncludesLowSpeedKnobs(t *testing.T) {
	args := gitNetworkConfigArgs()

	// Args must always include the low-speed limit/time pair so a stalled
	// HTTP(S) transfer surfaces an error within ~30 s instead of hanging
	// until the caller's context cancels.
	assert.Contains(t, args, "-c")
	assert.Contains(t, args, "http.lowSpeedLimit="+defaultGitHTTPLowSpeedLimit)
	assert.Contains(t, args, "http.lowSpeedTime="+defaultGitHTTPLowSpeedTime)
}

func TestGitCommandContext_PrependsNetworkArgs(t *testing.T) {
	cmd := gitCommandContext(t.Context(), "clone", "https://example.com/repo.git", "/tmp/dst")

	// cmd.Args[0] is the resolved git binary path; subsequent args must lead
	// with the network-tuning -c flags before the subcommand.
	joined := strings.Join(cmd.Args, " ")
	assert.Contains(t, joined, "-c http.lowSpeedLimit="+defaultGitHTTPLowSpeedLimit)
	assert.Contains(t, joined, "-c http.lowSpeedTime="+defaultGitHTTPLowSpeedTime)

	// The actual subcommand must still be present after the -c flags.
	assert.Contains(t, joined, "clone")
	assert.Contains(t, joined, "https://example.com/repo.git")
}

// lastGitSSHCommand returns the value of the last GIT_SSH_COMMAND= entry in env.
// Effective env follows last-wins semantics, and setupGitEnv may leave a stale
// empty entry behind (its filter only removes non-empty existing values), so
// reading the first match is not deterministic. Always pick the last match.
func lastGitSSHCommand(env []string) string {
	var got string
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			got = e
		}
	}
	return got
}

func TestSetupGitEnv_AlwaysSetsSSHTimeouts(t *testing.T) {
	// Scrub any inherited GIT_SSH_COMMAND so the env baseline is deterministic.
	t.Setenv("GIT_SSH_COMMAND", "")

	cmd := gitCommandContext(t.Context(), "ls-remote", "ssh://git@example.com/repo.git", "HEAD")
	setupGitEnv(cmd, "")

	// Even without an sshKeyFile we want the GIT_SSH_COMMAND env var so SSH
	// transports are bounded by ConnectTimeout / ServerAlive*.
	sshEnv := lastGitSSHCommand(cmd.Env)
	assert.NotEmpty(t, sshEnv, "GIT_SSH_COMMAND must be set even without an sshKeyFile")
	assert.Contains(t, sshEnv, "ConnectTimeout="+defaultSSHConnectTimeoutSeconds)
	assert.Contains(t, sshEnv, "ServerAliveInterval="+defaultSSHServerAliveInterval)
	assert.Contains(t, sshEnv, "ServerAliveCountMax="+defaultSSHServerAliveCountMax)
}

func TestSetupGitEnv_WithSSHKeyStillSetsTimeouts(t *testing.T) {
	t.Setenv("GIT_SSH_COMMAND", "")

	cmd := gitCommandContext(t.Context(), "clone", "ssh://git@example.com/repo.git", "/tmp/dst")
	setupGitEnv(cmd, "/tmp/fake-key")

	sshEnv := lastGitSSHCommand(cmd.Env)
	assert.NotEmpty(t, sshEnv)
	assert.Contains(t, sshEnv, "-i /tmp/fake-key", "must still inject the SSH key file")
	assert.Contains(t, sshEnv, "ConnectTimeout="+defaultSSHConnectTimeoutSeconds, "must still set timeouts when a key is configured")
}

func TestNumericEnvOr_AcceptsValidOverride(t *testing.T) {
	t.Setenv(envGitHTTPLowSpeedTime, "60")
	assert.Equal(t, "60", gitHTTPLowSpeedTime(), "valid override should win over default")
}

func TestNumericEnvOr_RejectsNonNumericOverride(t *testing.T) {
	// A typo like "60s" must not silently disable the timeout (git would error
	// or interpret it as 0). Fall back to the safe default.
	t.Setenv(envGitHTTPLowSpeedTime, "60s")
	assert.Equal(t, defaultGitHTTPLowSpeedTime, gitHTTPLowSpeedTime())
}

func TestNumericEnvOr_RejectsNegativeOverride(t *testing.T) {
	t.Setenv(envGitHTTPLowSpeedTime, "-1")
	assert.Equal(t, defaultGitHTTPLowSpeedTime, gitHTTPLowSpeedTime())
}

func TestNumericEnvOr_EmptyValueFallsBackToDefault(t *testing.T) {
	t.Setenv(envGitHTTPLowSpeedTime, "")
	assert.Equal(t, defaultGitHTTPLowSpeedTime, gitHTTPLowSpeedTime())
}

func TestGitNetworkConfigArgs_HonorsEnvOverrides(t *testing.T) {
	t.Setenv(envGitHTTPLowSpeedLimit, "2048")
	t.Setenv(envGitHTTPLowSpeedTime, "90")

	args := gitNetworkConfigArgs()
	assert.Contains(t, args, "http.lowSpeedLimit=2048")
	assert.Contains(t, args, "http.lowSpeedTime=90")
}

func TestSetupGitEnv_HonorsSSHTimeoutEnvOverrides(t *testing.T) {
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv(envSSHConnectTimeout, "45")
	t.Setenv(envSSHServerAliveInterval, "20")
	t.Setenv(envSSHServerAliveCountMax, "6")

	cmd := gitCommandContext(t.Context(), "ls-remote", "ssh://git@example.com/repo.git", "HEAD")
	setupGitEnv(cmd, "")

	sshEnv := lastGitSSHCommand(cmd.Env)
	assert.Contains(t, sshEnv, "ConnectTimeout=45")
	assert.Contains(t, sshEnv, "ServerAliveInterval=20")
	assert.Contains(t, sshEnv, "ServerAliveCountMax=6")
}
