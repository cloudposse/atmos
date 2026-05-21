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
	assert.Contains(t, args, "http.lowSpeedLimit="+gitHTTPLowSpeedLimit)
	assert.Contains(t, args, "http.lowSpeedTime="+gitHTTPLowSpeedTime)
}

func TestGitCommandContext_PrependsNetworkArgs(t *testing.T) {
	cmd := gitCommandContext(t.Context(), "clone", "https://example.com/repo.git", "/tmp/dst")

	// cmd.Args[0] is the resolved git binary path; subsequent args must lead
	// with the network-tuning -c flags before the subcommand.
	joined := strings.Join(cmd.Args, " ")
	assert.Contains(t, joined, "-c http.lowSpeedLimit="+gitHTTPLowSpeedLimit)
	assert.Contains(t, joined, "-c http.lowSpeedTime="+gitHTTPLowSpeedTime)

	// The actual subcommand must still be present after the -c flags.
	assert.Contains(t, joined, "clone")
	assert.Contains(t, joined, "https://example.com/repo.git")
}

func TestSetupGitEnv_AlwaysSetsSSHTimeouts(t *testing.T) {
	cmd := gitCommandContext(t.Context(), "ls-remote", "ssh://git@example.com/repo.git", "HEAD")
	setupGitEnv(cmd, "")

	// Even without an sshKeyFile we want the GIT_SSH_COMMAND env var so SSH
	// transports are bounded by ConnectTimeout / ServerAlive*.
	var sshEnv string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			sshEnv = e
			break
		}
	}
	assert.NotEmpty(t, sshEnv, "GIT_SSH_COMMAND must be set even without an sshKeyFile")
	assert.Contains(t, sshEnv, "ConnectTimeout="+sshConnectTimeoutSeconds)
	assert.Contains(t, sshEnv, "ServerAliveInterval="+sshServerAliveInterval)
	assert.Contains(t, sshEnv, "ServerAliveCountMax="+sshServerAliveCountMax)
}

func TestSetupGitEnv_WithSSHKeyStillSetsTimeouts(t *testing.T) {
	cmd := gitCommandContext(t.Context(), "clone", "ssh://git@example.com/repo.git", "/tmp/dst")
	setupGitEnv(cmd, "/tmp/fake-key")

	var sshEnv string
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "GIT_SSH_COMMAND=") {
			sshEnv = e
			break
		}
	}
	assert.NotEmpty(t, sshEnv)
	assert.Contains(t, sshEnv, "-i /tmp/fake-key", "must still inject the SSH key file")
	assert.Contains(t, sshEnv, "ConnectTimeout="+sshConnectTimeoutSeconds, "must still set timeouts when a key is configured")
}
