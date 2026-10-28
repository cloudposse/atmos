package auth

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/plugin/credential"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	_ = schema.Provider{Kind: "plugin/test", Spec: nil}
	_ = schema.Identity{Kind: "plugin/test", Principal: nil, Spec: nil, Provider: "", Via: nil}
)

func providerConfig(spec map[string]any) *schema.Provider {
	return &schema.Provider{Kind: "plugin/test", Spec: spec}
}

func TestProviderValidationAndEnvironmentIsolation(t *testing.T) {
	t.Parallel()
	definition := &Definition{Schema: func(value any) error {
		assert.Equal(t, map[string]any{"tenant": "one"}, value)
		return errUtils.ErrPluginInvalidConfig
	}}
	value, err := ProviderConstructor(definition)("corporate", providerConfig(map[string]any{"tenant": "one"}))
	require.NoError(t, err)
	require.ErrorIs(t, value.Validate(), errUtils.ErrPluginInvalidConfig)
	input := map[string]string{"KEEP": "yes"}
	output, err := value.PrepareEnvironment(t.Context(), input)
	require.NoError(t, err)
	input["KEEP"] = "input-change"
	assert.Equal(t, "yes", output["KEEP"])
	output["KEEP"] = "output-change"
	assert.Equal(t, "input-change", input["KEEP"])
}

func TestAdaptersRejectUnserializableConfigurationBeforeStarting(t *testing.T) {
	t.Parallel()
	invalid := map[string]any{"unsupported": make(chan int)}
	definition := &Definition{Credentials: credential.New()}
	p := &provider{definition: definition, config: providerConfig(invalid)}
	_, err := p.Authenticate(t.Context())
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, p.Logout(t.Context()), errUtils.ErrPluginInvalidConfig)
	upstream := &types.AWSCredentials{AccessKeyID: "key", SecretAccessKey: "secret", SessionToken: "token", Expiration: time.Now().Add(time.Hour).Format(time.RFC3339)}
	i := &identity{definition: definition, accepts: []string{"aws/v1"}, config: &schema.Identity{Principal: invalid}}
	_, err = i.Authenticate(t.Context(), upstream)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, i.Logout(t.Context()), errUtils.ErrPluginInvalidConfig)
	i.config.Principal = nil
	i.config.Spec = invalid
	_, err = i.Authenticate(t.Context(), upstream)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	i.accepts = []string{"oidc/v1"}
	_, err = i.Authenticate(t.Context(), upstream)
	require.Error(t, err)
}

func TestIdentityRoutingValidationAndMaterializer(t *testing.T) {
	t.Parallel()
	materializer := NewMockMaterializer(gomock.NewController(t))
	definition := &Definition{Schema: func(any) error { return io.ErrUnexpectedEOF }}
	constructor := IdentityConstructor(definition, []string{"oidc/v1"}, func(string, *schema.Identity) Materializer { return materializer })
	config := &schema.Identity{Kind: "plugin/broker"}
	value, err := constructor("broker", config)
	require.NoError(t, err)
	i := value.(*identity)
	_, err = IdentityConstructor(definition, nil, nil)("broker", config)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = i.GetProviderName()
	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
	require.ErrorIs(t, i.Validate(), errUtils.ErrInvalidIdentityConfig)
	config.Via = &schema.IdentityVia{}
	require.ErrorIs(t, i.Validate(), errUtils.ErrInvalidIdentityConfig)
	config.Provider = "legacy"
	name, err := i.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "legacy", name)
	config.Via.Provider = "direct"
	name, err = i.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "direct", name)
	require.ErrorIs(t, i.Validate(), io.ErrUnexpectedEOF)
	materializer.EXPECT().SetRootProvider("root")
	i.SetRootProvider("root")
	name, err = i.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "root", name)
	materializer.EXPECT().SetRealm("realm")
	i.SetRealm("realm")
	materializer.EXPECT().Paths().Return([]types.Path{{Location: "credentials", Type: types.PathTypeFile}}, nil)
	paths, err := i.Paths()
	require.NoError(t, err)
	assert.Equal(t, []types.Path{{Location: "credentials", Type: types.PathTypeFile}}, paths)
	input := map[string]string{"KEEP": "value"}
	materializer.EXPECT().PrepareEnvironment(input).Return(map[string]string{"PREPARED": "value"}, nil)
	output, err := i.PrepareEnvironment(t.Context(), input)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"PREPARED": "value"}, output)
}

func TestIdentityCachedCredentialAvailability(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		value  types.ICredentials
		err    error
		exists bool
	}{
		{name: "absent"},
		{name: "failure", err: io.ErrUnexpectedEOF},
		{name: "expired", value: &types.AWSCredentials{Expiration: time.Now().Add(-time.Hour).Format(time.RFC3339)}},
		{name: "valid", value: &types.AWSCredentials{Expiration: time.Now().Add(time.Hour).Format(time.RFC3339)}, exists: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			materializer := NewMockMaterializer(gomock.NewController(t))
			materializer.EXPECT().Load(gomock.Any()).Return(tt.value, tt.err)
			i := &identity{materializer: materializer}
			exists, err := i.CredentialsExist()
			assert.Equal(t, tt.exists, exists)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultBrowserRespectsInteractivePolicy(t *testing.T) {
	t.Parallel()
	browser, interactive := defaultBrowser(&Services{Interactive: func() bool { return false }})
	assert.False(t, interactive)
	require.ErrorIs(t, browser.Open(context.Background(), "https://issuer.example"), errUtils.ErrPluginInteractionRequired)
	supplied := api.LocalBrowser{Interactive: true}
	browser, interactive = defaultBrowser(&Services{Interactive: func() bool { return true }, Browser: supplied})
	assert.True(t, interactive)
	assert.Equal(t, supplied, browser)
}

func TestAdaptersForwardConfigurationDecodeCredentialsAndRevokeSessions(t *testing.T) {
	definition, directory := fakeDefinition(t, fakeplugin.ModeEcho)
	definition.Outputs = []string{"oidc/v1"}
	definition.Credentials = credential.New()
	definition.Services.Interactive = func() bool { return false }
	spec := map[string]any{
		"issuer": "https://issuer.example", "subject": "fixture-subject", "client_id": "fixture-client",
		"access_token": "fixture-access-token", "token_type": "Bearer", "access_token_expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	provider, err := ProviderConstructor(definition)("corporate", providerConfig(spec))
	require.NoError(t, err)
	provider.SetRealm("fixture")
	upstream, err := provider.Authenticate(t.Context())
	require.NoError(t, err)
	require.IsType(t, &types.PluginCredentials{}, upstream)
	require.Equal(t, "oidc/v1", upstream.(*types.PluginCredentials).Type)
	require.Equal(t, spec, upstream.(*types.PluginCredentials).Data)

	identityDefinition := *definition
	identityDefinition.Outputs = []string{"aws/v1"}
	materializer := NewMockMaterializer(gomock.NewController(t))
	identity, err := IdentityConstructor(&identityDefinition, []string{"oidc/v1"}, func(string, *schema.Identity) Materializer { return materializer })("broker", &schema.Identity{
		Kind: "plugin/broker", Via: &schema.IdentityVia{Provider: "corporate"},
		Principal: map[string]any{"endpoint": "https://broker.example"}, Spec: map[string]any{"tenant": "test"},
	})
	require.NoError(t, err)
	materializer.EXPECT().SetRealm("fixture")
	identity.SetRealm("fixture")
	cloud, err := identity.Authenticate(t.Context(), upstream)
	require.NoError(t, err)
	require.IsType(t, &types.AWSCredentials{}, cloud)
	aws := cloud.(*types.AWSCredentials)
	require.Equal(t, "access", aws.AccessKeyID)
	require.Equal(t, "secret", aws.SecretAccessKey)
	require.Equal(t, "session", aws.SessionToken)
	require.False(t, aws.IsExpired())
	materializer.EXPECT().Logout(gomock.Any()).Return(nil)
	require.NoError(t, identity.Logout(t.Context()))
	require.NoError(t, provider.Logout(t.Context()))
	for _, filename := range []string{"provider-logout", "identity-logout"} {
		content, err := os.ReadFile(filepath.Join(directory, filename))
		require.NoError(t, err)
		require.Equal(t, "revoked", string(content))
	}
}

func TestCredentialContractsCannotMutateSharedDefinitions(t *testing.T) {
	t.Parallel()
	definition := &Definition{Outputs: []string{"aws/v1"}}
	provider := &provider{definition: definition}
	identity := &identity{definition: definition, accepts: []string{"oidc/v1"}}
	providerContract := provider.CredentialContract()
	identityContract := identity.CredentialContract()
	providerContract.Produces[0] = "untrusted/v1"
	identityContract.Produces[0] = "other/v1"
	identityContract.Accepts[0] = "any/v1"
	require.Equal(t, []string{"aws/v1"}, provider.CredentialContract().Produces)
	require.Equal(t, []string{"aws/v1"}, identity.CredentialContract().Produces)
	require.Equal(t, []string{"oidc/v1"}, identity.CredentialContract().Accepts)
}
