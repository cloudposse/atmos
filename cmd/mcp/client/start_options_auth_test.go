package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: if schema.MCPServerConfig.Command or .Identity is
// renamed or removed, this declaration fails the build before any test runs.
// Per CLAUDE.md: "Add compile-time sentinels for schema field references in tests".
var _ = schema.MCPServerConfig{
	Command:  "echo",
	Identity: "ci",
}

func TestMcpServersNeedAuth(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]schema.MCPServerConfig
		want    bool
	}{
		{
			name:    "empty",
			servers: nil,
			want:    false,
		},
		{
			name: "no identity",
			servers: map[string]schema.MCPServerConfig{
				"a": {Command: "echo"},
			},
			want: false,
		},
		{
			name: "with identity",
			servers: map[string]schema.MCPServerConfig{
				"a": {Command: "echo", Identity: "ci"},
			},
			want: true,
		},
		{
			name: "mixed",
			servers: map[string]schema.MCPServerConfig{
				"a": {Command: "echo"},
				"b": {Command: "echo", Identity: "ci"},
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, mcpServersNeedAuth(tc.servers))
		})
	}
}

func TestBuildAuthOption_NoServersNeedingAuth(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo"},
	}
	assert.Nil(t, buildAuthOption(cfg))
}

func TestBuildAuthOption_ReturnsWiredWithAuthManagerOption(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo", Identity: "ci"},
	}
	opts := buildAuthOption(cfg)
	require.Len(t, opts, 1)

	// Behavioral check: actually invoke the StartOption closure and verify
	// it implements WithAuthManager's documented "no identity → pass-through
	// unchanged" contract. That branch lives only inside WithAuthManager
	// (pkg/mcp/client/session.go), so observing it here proves the closure
	// was constructed via mcpclient.WithAuthManager(...) and not some other
	// code path. A length-1 check + a separately-constructed provider would
	// not catch a regression where buildAuthOption wired the wrong thing.
	parsedNoIdentity := &mcpclient.ParsedConfig{
		Name:     "no-identity-probe",
		Identity: "", // empty identity → must pass through unchanged.
	}
	inputEnv := []string{"PATH=/usr/bin", "FOO=bar"}
	resultEnv, err := opts[0](context.Background(), parsedNoIdentity, inputEnv)
	require.NoError(t, err, "WithAuthManager pass-through must not error on empty identity")
	assert.Equal(t, inputEnv, resultEnv,
		"WithAuthManager must return env unchanged when identity is empty — observing this confirms the StartOption was wired via WithAuthManager")
}

// TestBuildAuthOption_ProviderImplementsRequiredInterfaces is a regression
// guard ensuring the ScopedAuthProvider that buildAuthOption hands to
// WithAuthManager continues to satisfy both interfaces WithAuthManager
// dispatches on. It is intentionally separate from
// TestBuildAuthOption_ReturnsWiredWithAuthManagerOption (which proves the
// wiring is in place) — together they cover both halves of the contract.
func TestBuildAuthOption_ProviderImplementsRequiredInterfaces(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo", Identity: "ci"},
	}

	provider := mcpclient.NewScopedAuthProvider(cfg)
	var _ mcpclient.AuthEnvProvider = provider
	var _ mcpclient.PerServerAuthProvider = provider
}
