package registry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the provider fields passed through the registry.
var _ = schema.Provider{Kind: "gitlab/oidc", Spec: map[string]any{}}

func TestNewProvider_GitLabOIDCCustomSource(t *testing.T) {
	t.Setenv("ATMOS_OIDC_TOKEN", "default-job-token")
	t.Setenv("GITLAB_ID_TOKEN", "custom-job-token")
	provider, err := NewProvider("gitlab-ci", &schema.Provider{
		Kind: "gitlab/oidc",
		Spec: map[string]any{"token_environment_variable": "GITLAB_ID_TOKEN"},
	})
	require.NoError(t, err)
	assert.Equal(t, "gitlab-ci", provider.Name())
	assert.Equal(t, "gitlab/oidc", provider.Kind())
	assert.True(t, types.ProviderIsAmbient(provider))
	creds, err := provider.Authenticate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, &types.OIDCCredentials{Token: "custom-job-token", Provider: "gitlab"}, creds)
}
