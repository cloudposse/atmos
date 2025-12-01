package pro

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDefaultClientFactory_NewClient verifies that DefaultClientFactory delegates to NewAtmosProAPIClientFromEnv.
func TestDefaultClientFactory_NewClient(t *testing.T) {
	factory := &DefaultClientFactory{}

	// Create config with Pro settings.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Settings: schema.AtmosSettings{
			Pro: schema.ProSettings{
				BaseURL:  "https://test.example.com",
				Endpoint: "/api/v1",
				Token:    "test-token",
			},
		},
	}

	// Call NewClient.
	client, err := factory.NewClient(atmosConfig)

	// We expect successful client creation with proper config.
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

// TestDefaultClientFactory_NewClient_MissingToken verifies that NewClient fails gracefully without required token.
func TestDefaultClientFactory_NewClient_MissingToken(t *testing.T) {
	factory := &DefaultClientFactory{}

	// Create config without Pro token.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Settings: schema.AtmosSettings{
			Pro: schema.ProSettings{
				BaseURL:  "https://test.example.com",
				Endpoint: "/api/v1",
				// Token is empty.
			},
		},
	}

	// Clear GitHub OIDC env vars to ensure we don't use OIDC flow.
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")

	// Call NewClient without token.
	client, err := factory.NewClient(atmosConfig)

	// We expect an error when token is missing and not in GitHub Actions.
	assert.Error(t, err)
	assert.Nil(t, client)
}
