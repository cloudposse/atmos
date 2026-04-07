package client

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

// fakeAuthManager is a minimal auth.AuthManager stand-in for tests. We only
// need PrepareShellEnvironment.
type fakeAuthManager struct {
	auth.AuthManager // embed nil — only PrepareShellEnvironment is exercised.
	prepareCalls     int
}

func (f *fakeAuthManager) PrepareShellEnvironment(_ context.Context, identityName string, currentEnv []string) ([]string, error) {
	f.prepareCalls++
	return append(currentEnv, "FAKE_IDENTITY="+identityName), nil
}

func newTestPerServerAuthManager(t *testing.T, observedProfile *string, errOnInit error) *PerServerAuthManager {
	t.Helper()
	p := NewPerServerAuthManager(&schema.AtmosConfiguration{})
	p.initConfig = func(_ schema.ConfigAndStacksInfo, _ bool) (schema.AtmosConfiguration, error) {
		if errOnInit != nil {
			return schema.AtmosConfiguration{}, errOnInit
		}
		// Capture the ATMOS_PROFILE that was active when InitCliConfig ran.
		if observedProfile != nil {
			*observedProfile = os.Getenv("ATMOS_PROFILE")
		}
		return schema.AtmosConfiguration{}, nil
	}
	p.createAuthManager = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		// envAtBuild is captured separately by initConfig, so just return a fake.
		return &fakeAuthManager{}, nil
	}
	return p
}

func TestPerServerAuthManager_ForServer_AppliesAtmosEnvBeforeInit(t *testing.T) {
	// Pre-condition: ATMOS_PROFILE is unset.
	old, had := os.LookupEnv("ATMOS_PROFILE")
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("ATMOS_PROFILE", old)
		} else {
			_ = os.Unsetenv("ATMOS_PROFILE")
		}
	})

	var observed string
	p := newTestPerServerAuthManager(t, &observed, nil)

	cfg := &mcpclient.ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
		Env: map[string]string{
			"ATMOS_PROFILE": "managers",
		},
	}

	mgr, err := p.ForServer(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, mgr)

	// initConfig saw the server-scoped ATMOS_PROFILE.
	assert.Equal(t, "managers", observed)

	// After ForServer returns, ATMOS_PROFILE has been restored to unset.
	_, stillSet := os.LookupEnv("ATMOS_PROFILE")
	assert.False(t, stillSet, "ATMOS_PROFILE must be restored after ForServer")
}

func TestPerServerAuthManager_ForServer_RestoresPreviousProfile(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "outer")

	var observed string
	p := newTestPerServerAuthManager(t, &observed, nil)

	cfg := &mcpclient.ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
		Env: map[string]string{
			"ATMOS_PROFILE": "managers",
		},
	}

	_, err := p.ForServer(context.Background(), cfg)
	require.NoError(t, err)

	// Initialization saw "managers", but afterwards ATMOS_PROFILE is back to "outer".
	assert.Equal(t, "managers", observed)
	assert.Equal(t, "outer", os.Getenv("ATMOS_PROFILE"))
}

func TestPerServerAuthManager_ForServer_NoEnvOverride(t *testing.T) {
	withCleanProfile(t)

	var observed string
	p := newTestPerServerAuthManager(t, &observed, nil)

	cfg := &mcpclient.ParsedConfig{
		Name:     "no-env",
		Identity: "id",
		Env:      nil,
	}

	mgr, err := p.ForServer(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.Empty(t, observed, "no override applied")
}

func TestPerServerAuthManager_ForServer_InitConfigError(t *testing.T) {
	withCleanProfile(t)

	p := newTestPerServerAuthManager(t, nil, errors.New("boom"))

	cfg := &mcpclient.ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
		Env: map[string]string{
			"ATMOS_PROFILE": "managers",
		},
	}

	_, err := p.ForServer(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "atmos")
	assert.Contains(t, err.Error(), "boom")

	// Even on error, env must be restored.
	_, set := os.LookupEnv("ATMOS_PROFILE")
	assert.False(t, set)
}

func TestPerServerAuthManager_PrepareShellEnvironment_FallbackPath(t *testing.T) {
	withCleanProfile(t)

	p := newTestPerServerAuthManager(t, nil, nil)

	out, err := p.PrepareShellEnvironment(
		context.Background(),
		"core-root/terraform",
		[]string{"PATH=/usr/bin"},
	)
	require.NoError(t, err)
	assert.Contains(t, out, "FAKE_IDENTITY=core-root/terraform")
	assert.Contains(t, out, "PATH=/usr/bin")
}

func TestPerServerAuthManager_ImplementsBothInterfaces(t *testing.T) {
	var p mcpclient.AuthEnvProvider = NewPerServerAuthManager(&schema.AtmosConfiguration{})
	_, ok := p.(mcpclient.PerServerAuthProvider)
	assert.True(t, ok, "PerServerAuthManager must implement PerServerAuthProvider")
}

// withCleanProfile unsets ATMOS_PROFILE for the duration of a test.
func withCleanProfile(t *testing.T) {
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

// ----------------------------------------------------------------------------
// buildAuthOption / mcpServersNeedAuth tests
// ----------------------------------------------------------------------------

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

func TestBuildAuthOption_ReturnsOption(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo", Identity: "ci"},
	}
	opts := buildAuthOption(cfg)
	require.Len(t, opts, 1)
}
