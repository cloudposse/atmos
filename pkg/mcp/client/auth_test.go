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
		Name:         "aws-eks",
		AuthIdentity: "production",
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
		Name:         "custom-server",
		AuthIdentity: "", // No auth identity.
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
		Name:         "aws-eks",
		AuthIdentity: "production",
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
		Name:         "aws-eks",
		AuthIdentity: "production",
	}
	env := []string{"PATH=/usr/bin"}

	_, err := opt(context.Background(), config, env)
	require.Error(t, err)
}

// newTestServerConfig creates a minimal MCPServerConfig for testing.
func newTestServerConfig(command string) schema.MCPServerConfig {
	return schema.MCPServerConfig{Command: command}
}

func TestParseConfig_AuthIdentity(t *testing.T) {
	cfg := newTestServerConfig("uvx")
	cfg.AuthIdentity = "billing-readonly"

	parsed, err := ParseConfig("aws-cost", cfg)
	require.NoError(t, err)
	assert.Equal(t, "billing-readonly", parsed.AuthIdentity)
}

func TestParseConfig_EmptyAuthIdentity(t *testing.T) {
	cfg := newTestServerConfig("echo")

	parsed, err := ParseConfig("test", cfg)
	require.NoError(t, err)
	assert.Empty(t, parsed.AuthIdentity)
}
