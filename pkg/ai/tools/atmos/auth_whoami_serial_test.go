package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Successful authentication temporarily changes the process-wide logger prefix
// and level. These tests must run serially so other tests cannot log while auth
// changes that shared state. The auth validation and metadata tests remain parallel.
func TestAuthWhoamiTool_Execute_DefaultMockIdentity(t *testing.T) {
	authConfig := mockAuthConfig(true)
	atmosConfig := &schema.AtmosConfiguration{
		Auth:          authConfig,
		CliConfigPath: t.TempDir(),
	}
	tool := NewAuthWhoamiTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "mock-identity", result.Data["identity"])
	assert.Contains(t, result.Output, "mock-identity")
	// Credentials must never be present in the output or data.
	assert.NotContains(t, result.Output, "MOCK_SECRET")
	assert.NotContains(t, result.Output, "MOCK_TOKEN")
}

func TestAuthWhoamiTool_Execute_ExplicitIdentity(t *testing.T) {
	authConfig := mockAuthConfig(false)
	atmosConfig := &schema.AtmosConfiguration{
		Auth:          authConfig,
		CliConfigPath: t.TempDir(),
	}
	tool := NewAuthWhoamiTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"identity": "mock-identity",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "mock-identity", result.Data["identity"])
	assert.Equal(t, true, result.Data["valid"])
}
