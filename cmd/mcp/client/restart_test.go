package client

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRestartCmd_Registration checks the basic shape of the registered command.
func TestRestartCmd_Registration(t *testing.T) {
	assert.Equal(t, "restart <name>", restartCmd.Use)
	assert.NotEmpty(t, restartCmd.Short)
	assert.NotEmpty(t, restartCmd.Long)
	assert.NotNil(t, restartCmd.RunE)
	assert.NotNil(t, restartCmd.Args, "ExactArgs(1) validator must be set")
}

// TestRestartCmd_HelpMentionsValidationSemantics is the regression guard for
// issue #3 in docs/fixes/2026-05-15-mcp-review-fixes.md.
//
// Pre-fix, the Short field said "Restart an MCP server" — which led users
// to expect a running server after `atmos mcp restart <name>`. Post-fix,
// both Short and Long spell out that the command validates a stop+start
// cycle and does not leave the server running.
//
// The assertions are intentionally semantic (contain a keyword from the
// disambiguation) rather than tied to exact phrasing so legitimate
// rewordings don't break the guard — but a future change that removes
// the disambiguation entirely will fail this test.
func TestRestartCmd_HelpMentionsValidationSemantics(t *testing.T) {
	t.Run("Short field mentions 'validate' or 'does not leave the server running'", func(t *testing.T) {
		short := strings.ToLower(restartCmd.Short)
		assert.True(t,
			strings.Contains(short, "validate") ||
				strings.Contains(short, "does not leave"),
			"Short field MUST clarify the stop+start+stop cycle; got: %q", restartCmd.Short)
	})

	t.Run("Long field explicitly states the server does not stay running", func(t *testing.T) {
		long := strings.ToLower(restartCmd.Long)
		// The phrase the fix doc commits to is "does NOT leave the server
		// running" — assert at least one of the equivalent phrasings is
		// present so a paraphrase doesn't break the test, but no
		// disambiguation at all does.
		assert.True(t,
			strings.Contains(long, "does not leave the server running") ||
				strings.Contains(long, "leaving nothing running") ||
				strings.Contains(long, "is no longer running after"),
			"Long field MUST tell the user the server is not running after the command exits; got:\n%s", restartCmd.Long)
	})

	t.Run("Long field directs users to AI commands for actually invoking tools", func(t *testing.T) {
		long := strings.ToLower(restartCmd.Long)
		// Users who want a running server need to know which command to use
		// instead. Pre-fix help text gave no such pointer.
		assert.True(t,
			strings.Contains(long, "atmos ai ask") ||
				strings.Contains(long, "atmos ai chat") ||
				strings.Contains(long, "atmos ai exec"),
			"Long field SHOULD point users to `atmos ai ask`/`chat`/`exec` "+
				"for actually running tool calls; got:\n%s", restartCmd.Long)
	})
}
