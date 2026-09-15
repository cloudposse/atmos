package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEnsureNoProxyForMirror verifies the mirror host is appended to both no-proxy variables,
// existing entries are preserved, an existing exemption (case or process env, or "*") is left
// alone, and an unparsable mirror URL is a no-op.
func TestEnsureNoProxyForMirror(t *testing.T) {
	empty := func(string) string { return "" }

	env := map[string]string{}
	ensureNoProxyForMirror(env, empty, "http://127.0.0.1:54321")
	require.Equal(t, "127.0.0.1", env["NO_PROXY"])
	require.Equal(t, "127.0.0.1", env["no_proxy"])

	env = map[string]string{"NO_PROXY": "example.com, localhost"}
	ensureNoProxyForMirror(env, empty, "http://127.0.0.1:54321")
	require.Equal(t, "example.com, localhost,127.0.0.1", env["NO_PROXY"])
	require.Equal(t, "127.0.0.1", env["no_proxy"])

	env = map[string]string{"NO_PROXY": "127.0.0.1,example.com", "no_proxy": "*"}
	ensureNoProxyForMirror(env, empty, "http://127.0.0.1:54321")
	require.Equal(t, "127.0.0.1,example.com", env["NO_PROXY"])
	require.Equal(t, "*", env["no_proxy"])

	// Exempt through the ambient process env: the case env must stay untouched.
	env = map[string]string{}
	ensureNoProxyForMirror(env, func(name string) string { return "127.0.0.1" }, "http://127.0.0.1:54321")
	require.Empty(t, env)

	env = map[string]string{}
	ensureNoProxyForMirror(env, empty, "://bad")
	require.Empty(t, env)
}
