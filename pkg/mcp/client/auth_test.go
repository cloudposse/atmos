package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockAuthProvider implements AuthEnvProvider for testing.
type mockAuthProvider struct {
	preparedEnv []string
	err         error
	calledWith  string
	callCount   int
}

func (m *mockAuthProvider) PrepareShellEnvironment(_ context.Context, identityName string, currentEnv []string) ([]string, error) {
	m.callCount++
	m.calledWith = identityName
	if m.err != nil {
		return nil, m.err
	}
	if m.preparedEnv != nil {
		return m.preparedEnv, nil
	}
	// Default: append auth vars to existing env.
	return append(currentEnv,
		"AWS_PROFILE="+identityName,
		"AWS_REGION=us-east-1",
	), nil
}

func TestWithAuthManager_InjectsCredentials(t *testing.T) {
	mock := &mockAuthProvider{}
	opt := WithAuthManager(mock)

	config := &ParsedConfig{
		Name:     "aws-eks",
		Identity: "production",
	}
	env := []string{"PATH=/usr/bin"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)

	assert.Equal(t, "production", mock.calledWith)
	assert.Contains(t, result, "AWS_PROFILE=production")
	assert.Contains(t, result, "AWS_REGION=us-east-1")
	assert.Contains(t, result, "PATH=/usr/bin") // Original env preserved.
}

func TestWithAuthManager_NoIdentity_Passthrough(t *testing.T) {
	mock := &mockAuthProvider{}
	opt := WithAuthManager(mock)

	config := &ParsedConfig{
		Name:     "custom-server",
		Identity: "", // No auth identity.
	}
	env := []string{"PATH=/usr/bin"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)

	assert.Zero(t, mock.callCount) // Should not be called.
	assert.Equal(t, env, result)   // Env unchanged.
}

func TestWithAuthManager_NilProvider_WithIdentity_ReturnsError(t *testing.T) {
	opt := WithAuthManager(nil)

	config := &ParsedConfig{
		Name:     "aws-eks",
		Identity: "production",
	}
	env := []string{"PATH=/usr/bin"}

	_, err := opt(context.Background(), config, env)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerAuthUnavailable)
}

func TestWithAuthManager_NilProvider_NoIdentity_Passthrough(t *testing.T) {
	opt := WithAuthManager(nil)

	config := &ParsedConfig{
		Name:     "aws-docs",
		Identity: "", // No identity — should pass through.
	}
	env := []string{"PATH=/usr/bin"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)
	assert.Equal(t, env, result)
}

// perServerMockProvider also implements PerServerAuthProvider so we can test
// the per-server reload path inside WithAuthManager.
type perServerMockProvider struct {
	mockAuthProvider
	forServerCalls   int
	lastForServerCfg *ParsedConfig
	scopedProvider   AuthEnvProvider
	forServerErr     error
}

func (p *perServerMockProvider) ForServer(_ context.Context, config *ParsedConfig) (AuthEnvProvider, error) {
	p.forServerCalls++
	p.lastForServerCfg = config
	if p.forServerErr != nil {
		return nil, p.forServerErr
	}
	return p.scopedProvider, nil
}

func TestWithAuthManager_PerServerProvider_UsesScopedProvider(t *testing.T) {
	scoped := &mockAuthProvider{
		preparedEnv: []string{"AWS_PROFILE=scoped", "PATH=/usr/bin"},
	}
	root := &perServerMockProvider{scopedProvider: scoped}

	opt := WithAuthManager(root)
	config := &ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
		Env:      map[string]string{"ATMOS_PROFILE": "managers"},
	}
	env := []string{"PATH=/usr/bin"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)

	// Per-server factory should have been consulted exactly once.
	assert.Equal(t, 1, root.forServerCalls)
	assert.Same(t, config, root.lastForServerCfg)

	// Scoped provider — not the root — should have been called for env preparation.
	assert.Equal(t, 1, scoped.callCount)
	assert.Equal(t, 0, root.callCount)
	assert.Equal(t, "core-root/terraform", scoped.calledWith)

	// Output should come from the scoped provider.
	assert.Contains(t, result, "AWS_PROFILE=scoped")
}

func TestWithAuthManager_PerServerProvider_ForServerError(t *testing.T) {
	root := &perServerMockProvider{forServerErr: assert.AnError}

	opt := WithAuthManager(root)
	config := &ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
	}

	_, err := opt(context.Background(), config, []string{"PATH=/usr/bin"})
	require.Error(t, err)
	// Scoped provider should never have been built or called.
	assert.Equal(t, 0, root.callCount)
}

func TestWithAuthManager_PerServerProvider_NilScoped_ReturnsAuthUnavailable(t *testing.T) {
	root := &perServerMockProvider{scopedProvider: nil}

	opt := WithAuthManager(root)
	config := &ParsedConfig{
		Name:     "atmos",
		Identity: "core-root/terraform",
	}

	_, err := opt(context.Background(), config, []string{"PATH=/usr/bin"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerAuthUnavailable)
}

func TestWithAuthManager_PerServerProvider_NoIdentity_SkipsForServer(t *testing.T) {
	root := &perServerMockProvider{scopedProvider: &mockAuthProvider{}}
	opt := WithAuthManager(root)

	config := &ParsedConfig{
		Name:     "no-auth",
		Identity: "",
		Env:      map[string]string{"ATMOS_PROFILE": "managers"},
	}
	env := []string{"PATH=/usr/bin"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)
	// No-identity passthrough — neither ForServer nor PrepareShellEnvironment should run.
	assert.Equal(t, 0, root.forServerCalls)
	assert.Equal(t, env, result)
}

func TestWithAuthManager_Error_ReturnsError(t *testing.T) {
	mock := &mockAuthProvider{err: assert.AnError}
	opt := WithAuthManager(mock)

	config := &ParsedConfig{
		Name:     "aws-eks",
		Identity: "production",
	}
	env := []string{"PATH=/usr/bin"}

	_, err := opt(context.Background(), config, env)
	require.Error(t, err)
}

// newTestServerConfig creates a minimal MCPServerConfig for testing.
func newTestServerConfig(command string) schema.MCPServerConfig {
	return schema.MCPServerConfig{Command: command}
}

func TestParseConfig_Identity(t *testing.T) {
	cfg := newTestServerConfig("uvx")
	cfg.Identity = "billing-readonly"

	parsed, err := ParseConfig("aws-cost", cfg)
	require.NoError(t, err)
	assert.Equal(t, "billing-readonly", parsed.Identity)
}

func TestParseConfig_EmptyIdentity(t *testing.T) {
	cfg := newTestServerConfig("echo")

	parsed, err := ParseConfig("test", cfg)
	require.NoError(t, err)
	assert.Empty(t, parsed.Identity)
}

// ──────────────────────────────────────────────────────────────────────────────
// WithToolchain tests
// ──────────────────────────────────────────────────────────────────────────────

// mockToolchain implements ToolchainResolver for testing.
type mockToolchain struct {
	resolvedPaths map[string]string
	envVars       []string
}

func (m *mockToolchain) Resolve(command string) string {
	if p, ok := m.resolvedPaths[command]; ok {
		return p
	}
	return command
}

func (m *mockToolchain) EnvVars() []string {
	return m.envVars
}

func TestWithToolchain_ResolvesCommand(t *testing.T) {
	resolvedPath := filepath.Join("opt", "toolchain", "bin", "uvx")
	toolchainPATH := "PATH=" + filepath.Join("opt", "toolchain", "bin") + string(os.PathListSeparator) + filepath.Join("usr", "bin")
	tc := &mockToolchain{
		resolvedPaths: map[string]string{"uvx": resolvedPath},
		envVars:       []string{toolchainPATH},
	}
	opt := WithToolchain(tc)

	config := &ParsedConfig{
		Name:    "aws-eks",
		Command: "uvx",
	}
	env := []string{"HOME=" + filepath.Join("home", "user")}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)

	// Command should be resolved to absolute path.
	assert.Equal(t, resolvedPath, config.Command)

	// Toolchain PATH should be appended to env.
	assert.Contains(t, result, toolchainPATH)
	assert.Contains(t, result, "HOME="+filepath.Join("home", "user"))
}

func TestWithToolchain_CommandNotInToolchain(t *testing.T) {
	tc := &mockToolchain{
		resolvedPaths: map[string]string{}, // Empty — command not managed.
	}
	opt := WithToolchain(tc)

	config := &ParsedConfig{
		Name:    "custom",
		Command: "my-server",
	}
	env := []string{"PATH=/usr/bin"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)

	// Command unchanged.
	assert.Equal(t, "my-server", config.Command)
	// Env unchanged (no toolchain PATH).
	assert.Equal(t, []string{"PATH=/usr/bin"}, result)
}

func TestWithToolchain_NilResolver(t *testing.T) {
	opt := WithToolchain(nil)

	config := &ParsedConfig{Command: "uvx"}
	env := []string{"PATH=/usr/bin"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)
	assert.Equal(t, env, result)
	assert.Equal(t, "uvx", config.Command)
}
