package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNewSession_InitialState(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test-server",
		Command: "echo",
	}
	session := NewSession(cfg)

	assert.Equal(t, "test-server", session.Name())
	assert.Equal(t, StatusStopped, session.Status())
	assert.Nil(t, session.LastError())
	assert.Nil(t, session.Tools())
}

func TestSession_Start_InvalidCommand(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "bad-server",
		Command: "nonexistent-binary-that-does-not-exist-xyz",
	}
	session := NewSession(cfg)

	err := session.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPIntegrationStartFailed)
	assert.Equal(t, StatusError, session.Status())
	assert.NotNil(t, session.LastError())
}

func TestSession_Stop_WhenStopped(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test",
		Command: "echo",
	}
	session := NewSession(cfg)

	// Stopping an already-stopped session should succeed.
	err := session.Stop()
	require.NoError(t, err)
	assert.Equal(t, StatusStopped, session.Status())
}

func TestSession_CallTool_WhenNotRunning(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test",
		Command: "echo",
	}
	session := NewSession(cfg)

	_, err := session.CallTool(context.Background(), "some-tool", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPIntegrationNotRunning)
}

func TestSession_Ping_WhenNotRunning(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test",
		Command: "echo",
	}
	session := NewSession(cfg)

	err := session.Ping(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPIntegrationNotRunning)
}

func TestBuildEnv(t *testing.T) {
	env := map[string]string{
		"AWS_REGION":  "us-east-1",
		"AWS_PROFILE": "production",
	}

	result := buildEnv(env)

	// Should contain the original environment plus the new vars.
	assert.True(t, len(result) > 2, "should include OS env plus configured vars")

	// Check that configured vars are present.
	found := 0
	for _, e := range result {
		if e == "AWS_REGION=us-east-1" || e == "AWS_PROFILE=production" {
			found++
		}
	}
	assert.Equal(t, 2, found, "both configured env vars should be present")
}

func TestSession_Config(t *testing.T) {
	cfg := &ParsedConfig{
		Name:        "test",
		Command:     "echo",
		Description: "Test server",
	}
	session := NewSession(cfg)
	assert.Equal(t, cfg, session.Config())
	assert.Equal(t, "Test server", session.Config().Description)
}

func TestSession_Start_AlreadyRunning(t *testing.T) {
	cfg := &ParsedConfig{
		Name:    "test",
		Command: "echo",
	}
	session := NewSession(cfg)
	// Manually set to running to test the early return.
	session.mu.Lock()
	session.status = StatusRunning
	session.mu.Unlock()

	err := session.Start(context.Background())
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, session.Status())
}

func TestBuildEnv_EmptyMap(t *testing.T) {
	result := buildEnv(map[string]string{})
	// Should just be the OS environment.
	assert.NotEmpty(t, result)
}
