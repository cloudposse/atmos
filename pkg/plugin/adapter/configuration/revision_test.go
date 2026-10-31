package configuration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestRevisionDiscoveryIsolated(t *testing.T) {
	config := enabledConfig(t)
	baseRoot := t.TempDir()
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").Times(2)
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		require.Nil(t, request.Effects, "revision discovery cannot read state or run commands")
		require.Equal(t, "plan", request.Operation.Command)
		name := "head"
		if request.RootDirectory == baseRoot {
			name = "base"
		} else {
			require.Equal(t, config.BasePathAbsolute, request.RootDirectory)
		}
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, name)}}, nil
	}).Times(2)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	head, err := New(ctx, config, "plan", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	headDefinitions, err := head.Discover(t.Context())
	require.NoError(t, err)
	revision, err := head.ForRevision(baseRoot)
	require.NoError(t, err)
	baseDefinitions, err := revision.Discover(t.Context())
	require.NoError(t, err)
	require.Contains(t, baseDefinitions["live"]["components"].(map[string]any)["terraform"], "base")
	again, err := head.Discover(t.Context())
	require.NoError(t, err)
	require.Equal(t, headDefinitions, again)
	require.Contains(t, again["live"]["components"].(map[string]any)["terraform"], "head")
	// A revision's request data must not alias the HEAD request in either direction.
	base := revision.(*Session)
	base.requests[0].IncludePaths[0] = "base/**"
	require.Equal(t, "live/**", head.requests[0].IncludePaths[0])
	head.requests[0].Invocation.Environment["REVISION"] = "head"
	require.NotContains(t, base.requests[0].Invocation.Environment, "REVISION")
	_, err = head.ForRevision("relative")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	cancel()
	_, err = head.ForRevision(baseRoot)
	require.ErrorIs(t, err, context.Canceled)
	_, err = revision.Discover(t.Context())
	require.ErrorIs(t, err, context.Canceled)
}

func TestRevisionKeepsImportProvidersButNotImportedUnits(t *testing.T) {
	config := enabledConfig(t)
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Discovery.Enabled = false
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = settings
	directories(t, config.BasePathAbsolute, "live")
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil
	}).Times(2)
	head, err := New(t.Context(), config, "list", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	components, handled, err := head.ImportComponents(t.Context(), "terragrunt://live")
	require.NoError(t, err)
	require.True(t, handled)
	require.Contains(t, components, "app")

	baseRoot := t.TempDir()
	directories(t, baseRoot, "previous")
	rebased, err := head.ForRevision(baseRoot)
	require.NoError(t, err)
	revision := rebased.(*Session)
	require.Empty(t, revision.requests, "the revision must not inherit HEAD's discovery scope")
	require.Empty(t, revision.imported, "the revision must not inherit HEAD's imported units")
	require.Equal(t, baseRoot, revision.importProviders["terragrunt"].RootDirectory)
	require.Equal(t, config.BasePathAbsolute, head.importProviders["terragrunt"].RootDirectory, "HEAD's provider template is unchanged")

	// The revision accepts its own YAML imports independently of HEAD's.
	components, handled, err = revision.ImportComponents(t.Context(), "terragrunt://previous")
	require.NoError(t, err)
	require.True(t, handled)
	require.Contains(t, components, "app")
	require.Contains(t, revision.imported, "live/app/terragrunt.hcl")
}
