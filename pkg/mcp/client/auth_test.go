package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockAuthProvider implements AuthEnvProvider for testing.
type mockAuthProvider struct {
	preparedEnv []string
	err         error
	calledWith  string
}

func (m *mockAuthProvider) PrepareShellEnvironment(_ context.Context, identityName string, currentEnv []string) ([]string, error) {
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

	assert.Equal(t, "", mock.calledWith) // Should not be called.
	assert.Equal(t, env, result)         // Env unchanged.
}

func TestWithAuthManager_NilProvider_Passthrough(t *testing.T) {
	opt := WithAuthManager(nil)

	config := &ParsedConfig{
		Name:     "aws-eks",
		Identity: "production",
	}
	env := []string{"PATH=/usr/bin"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)
	assert.Equal(t, env, result) // Env unchanged.
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
	tc := &mockToolchain{
		resolvedPaths: map[string]string{"uvx": "/opt/toolchain/bin/uvx"},
		envVars:       []string{"PATH=/opt/toolchain/bin:/usr/bin"},
	}
	opt := WithToolchain(tc)

	config := &ParsedConfig{
		Name:    "aws-eks",
		Command: "uvx",
	}
	env := []string{"HOME=/home/user"}

	result, err := opt(context.Background(), config, env)
	require.NoError(t, err)

	// Command should be resolved to absolute path.
	assert.Equal(t, "/opt/toolchain/bin/uvx", config.Command)

	// Toolchain PATH should be appended to env.
	assert.Contains(t, result, "PATH=/opt/toolchain/bin:/usr/bin")
	assert.Contains(t, result, "HOME=/home/user")
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
