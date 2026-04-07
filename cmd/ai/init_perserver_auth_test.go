package ai

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeAuthMgr is a minimal auth.AuthManager fake for tests.
type fakeAuthMgr struct {
	auth.AuthManager // nil embed; only PrepareShellEnvironment is exercised.
}

func (f *fakeAuthMgr) PrepareShellEnvironment(_ context.Context, identityName string, currentEnv []string) ([]string, error) {
	return append(currentEnv, "FAKE="+identityName), nil
}

func newTestAIPerServerProvider(t *testing.T, observed *string, initErr error) *perServerAuthProvider {
	t.Helper()
	p := newPerServerAuthProvider(&schema.AtmosConfiguration{})
	p.initConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		if initErr != nil {
			return schema.AtmosConfiguration{}, initErr
		}
		if observed != nil {
			*observed = os.Getenv("ATMOS_PROFILE")
		}
		return schema.AtmosConfiguration{}, nil
	}
	p.createAuthManager = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return &fakeAuthMgr{}, nil
	}
	return p
}

func clearAtmosProfile(t *testing.T) {
	t.Helper()
	old, had := os.LookupEnv("ATMOS_PROFILE")
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("ATMOS_PROFILE", old)
		} else {
			_ = os.Unsetenv("ATMOS_PROFILE")
		}
	})
}

func TestPerServerAuthProvider_ForServer_AppliesAtmosEnv(t *testing.T) {
	clearAtmosProfile(t)

	var observed string
	p := newTestAIPerServerProvider(t, &observed, nil)

	cfg := &mcpclient.ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
		Env:      map[string]string{"ATMOS_PROFILE": "managers"},
	}

	mgr, err := p.ForServer(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.Equal(t, "managers", observed, "auth manager built with server-scoped ATMOS_PROFILE")

	// Restored after ForServer.
	_, set := os.LookupEnv("ATMOS_PROFILE")
	assert.False(t, set)
}

func TestPerServerAuthProvider_ForServer_InitError(t *testing.T) {
	clearAtmosProfile(t)

	p := newTestAIPerServerProvider(t, nil, errors.New("kaboom"))
	cfg := &mcpclient.ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
		Env:      map[string]string{"ATMOS_PROFILE": "managers"},
	}

	_, err := p.ForServer(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "atmos")
	assert.Contains(t, err.Error(), "kaboom")

	// Env restored even on error.
	_, set := os.LookupEnv("ATMOS_PROFILE")
	assert.False(t, set)
}

func TestPerServerAuthProvider_PrepareShellEnvironment(t *testing.T) {
	clearAtmosProfile(t)

	p := newTestAIPerServerProvider(t, nil, nil)
	out, err := p.PrepareShellEnvironment(
		context.Background(),
		"core-root/terraform",
		[]string{"PATH=/usr/bin"},
	)
	require.NoError(t, err)
	assert.Contains(t, out, "FAKE=core-root/terraform")
}

func TestPerServerAuthProvider_ImplementsBothInterfaces(t *testing.T) {
	var p mcpclient.AuthEnvProvider = newPerServerAuthProvider(&schema.AtmosConfiguration{})
	_, ok := p.(mcpclient.PerServerAuthProvider)
	assert.True(t, ok, "perServerAuthProvider must satisfy mcpclient.PerServerAuthProvider")
}

func TestResolveAuthProvider_NoIdentity_ReturnsNil(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo"},
	}
	assert.Nil(t, resolveAuthProvider(cfg))
}

func TestResolveAuthProvider_WithIdentity_ReturnsPerServerProvider(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo", Identity: "ci"},
	}
	provider := resolveAuthProvider(cfg)
	require.NotNil(t, provider)
	_, isPerServer := provider.(mcpclient.PerServerAuthProvider)
	assert.True(t, isPerServer, "resolved provider must be per-server-aware")
}
