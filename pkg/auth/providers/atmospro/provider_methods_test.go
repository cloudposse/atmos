package atmospro

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewProvider_Errors(t *testing.T) {
	_, err := NewProvider("atmos-pro", nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)

	_, err = NewProvider("", &schema.Provider{Kind: Kind})
	require.ErrorIs(t, err, errUtils.ErrInvalidProviderConfig)
}

func TestProvider_TrivialMethods(t *testing.T) {
	p := newProvider(t, map[string]interface{}{})

	require.NoError(t, p.Validate())
	assert.Equal(t, Kind, p.Kind())
	assert.Equal(t, "atmos-pro", p.Name())

	env, err := p.Environment()
	require.NoError(t, err)
	assert.Empty(t, env)

	paths, err := p.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)

	in := map[string]string{"FOO": "bar"}
	out, err := p.PrepareEnvironment(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, in, out)

	assert.Empty(t, p.GetFilesDisplayPath())

	// PreAuthenticate is a no-op; SetRealm is not on the interface, so call it on the concrete type.
	require.NoError(t, p.PreAuthenticate(nil))
	p.(*proProvider).SetRealm("realmA")
}

// TestProvider_ResolutionFromEnv verifies base_url/endpoint/workspace_id fall back to their
// environment variables when the spec does not set them.
func TestProvider_ResolutionFromEnv(t *testing.T) {
	calls := withStubbedMintExchange(t, "oidc-tok", nil, "session-jwt", nil)
	t.Setenv("ATMOS_PRO_BASE_URL", "https://env.example.com")
	t.Setenv("ATMOS_PRO_ENDPOINT", "env/api")
	t.Setenv("ATMOS_PRO_WORKSPACE_ID", "ws-env")

	p := newProvider(t, map[string]interface{}{})
	_, err := p.Authenticate(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "https://env.example.com", calls.gotBaseURL)
	assert.Equal(t, "env/api", calls.gotEndpoint)
	assert.Equal(t, "ws-env", calls.gotWorkspce)
}

// TestProvider_ResolutionDefaults verifies base_url/endpoint fall back to the built-in defaults
// when neither spec nor environment supplies them.
func TestProvider_ResolutionDefaults(t *testing.T) {
	calls := withStubbedMintExchange(t, "oidc-tok", nil, "session-jwt", nil)
	t.Setenv("ATMOS_PRO_BASE_URL", "")
	t.Setenv("ATMOS_PRO_ENDPOINT", "")
	t.Setenv("ATMOS_PRO_WORKSPACE_ID", "")

	// workspace_id via spec so the missing-workspace guard passes; base/endpoint hit defaults.
	p := newProvider(t, map[string]interface{}{"workspace_id": "ws-spec"})
	_, err := p.Authenticate(context.Background())
	require.NoError(t, err)

	assert.Equal(t, cfg.AtmosProDefaultBaseUrl, calls.gotBaseURL)
	assert.Equal(t, cfg.AtmosProDefaultEndpoint, calls.gotEndpoint)
	assert.Equal(t, "ws-spec", calls.gotWorkspce)
}
