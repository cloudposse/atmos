package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEnvContext is a test implementation of EnvVarContext.
type mockEnvContext struct {
	envSection map[string]any
}

func (m *mockEnvContext) GetComponentEnvSection() map[string]any {
	return m.envSection
}

func TestProcessTagEnv_FromOS(t *testing.T) {
	t.Setenv("TEST_ATMOS_ENV_VAR", "from-os")

	result, err := ProcessTagEnv("!env TEST_ATMOS_ENV_VAR", nil)
	require.NoError(t, err)
	assert.Equal(t, "from-os", result)
}

func TestProcessTagEnv_DefaultValue(t *testing.T) {
	// Ensure the env var is not set.
	t.Setenv("ATMOS_TEST_MISSING_VAR_XYZ", "")
	_ = os.Unsetenv("ATMOS_TEST_MISSING_VAR_XYZ")

	result, err := ProcessTagEnv("!env ATMOS_TEST_MISSING_VAR_XYZ default-value", nil)
	require.NoError(t, err)
	assert.Equal(t, "default-value", result)
}

func TestProcessTagEnv_EmptyResult(t *testing.T) {
	// Var not set, no default.
	_ = os.Unsetenv("ATMOS_TEST_DEFINITELY_NOT_SET_VAR_XYZ123")

	result, err := ProcessTagEnv("!env ATMOS_TEST_DEFINITELY_NOT_SET_VAR_XYZ123", nil)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestProcessTagEnv_FromEnvContext(t *testing.T) {
	// Value from component env section should take precedence over OS env.
	t.Setenv("MY_VAR", "from-os")

	ctx := &mockEnvContext{
		envSection: map[string]any{
			"MY_VAR": "from-component",
		},
	}

	result, err := ProcessTagEnv("!env MY_VAR", ctx)
	require.NoError(t, err)
	assert.Equal(t, "from-component", result)
}

func TestProcessTagEnv_NilEnvContext(t *testing.T) {
	// Nil context falls back to OS env.
	t.Setenv("ATMOS_TEST_NIL_CTX_VAR", "os-value")

	result, err := ProcessTagEnv("!env ATMOS_TEST_NIL_CTX_VAR", nil)
	require.NoError(t, err)
	assert.Equal(t, "os-value", result)
}

func TestProcessTagEnv_EnvContextMissingKey(t *testing.T) {
	// Key not in env section falls back to OS env.
	t.Setenv("FALLBACK_VAR", "from-os-fallback")

	ctx := &mockEnvContext{
		envSection: map[string]any{
			"OTHER_VAR": "other-value",
		},
	}

	result, err := ProcessTagEnv("!env FALLBACK_VAR", ctx)
	require.NoError(t, err)
	assert.Equal(t, "from-os-fallback", result)
}

func TestProcessTagEnv_EmptyInput(t *testing.T) {
	// Empty string after tag prefix should return error.
	_, err := ProcessTagEnv("!env", nil)
	assert.Error(t, err)
}

func TestProcessTagEnv_TooManyArgs(t *testing.T) {
	// More than 2 args should return error.
	_, err := ProcessTagEnv("!env VAR_NAME default extra_arg", nil)
	assert.Error(t, err)
}

func TestProcessTagEnv_EnvContextNilSection(t *testing.T) {
	// Env context with nil section falls back to OS env.
	t.Setenv("ATMOS_TEST_NIL_SECTION", "os-value")

	ctx := &mockEnvContext{
		envSection: nil,
	}

	result, err := ProcessTagEnv("!env ATMOS_TEST_NIL_SECTION", ctx)
	require.NoError(t, err)
	assert.Equal(t, "os-value", result)
}
