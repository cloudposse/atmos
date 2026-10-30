package configuration

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExplicitImportsWithoutAutomaticDiscovery(t *testing.T) {
	config := enabledConfig(t)
	settings := config.Components.Terraform.ConfigurationProviders["terragrunt"]
	settings.Discovery.Enabled = false
	config.Components.Terraform.ConfigurationProviders["terragrunt"] = settings
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform")
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
		require.Empty(t, req.IncludePaths)
		require.Equal(t, []string{"terragrunt://live", "terragrunt://other/terragrunt.hcl"}, req.Imports)
		require.Nil(t, req.Effects)
		return &pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "app")}}, nil
	})
	loads := 0
	session, err := New(t.Context(), config, "list", func(_ context.Context, names []string) (map[string]Provider, error) {
		loads++
		require.Equal(t, []string{"terragrunt"}, names)
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	for _, uri := range []string{"terragrunt://live", "terragrunt://live/.", "terragrunt://other/terragrunt.hcl"} {
		handled, err := session.RegisterImport(t.Context(), uri)
		require.NoError(t, err)
		require.True(t, handled)
	}
	require.Zero(t, loads)
	definitions, err := session.Discover(t.Context())
	require.NoError(t, err)
	require.Contains(t, definitions, "live")
	handled, err := session.RegisterImport(t.Context(), "terragrunt://live")
	require.NoError(t, err)
	require.True(t, handled)
	_, err = session.RegisterImport(t.Context(), "terragrunt://late")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestImportValidationCancellationAndRevision(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	session, err := New(ctx, enabledConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) { return nil, nil })
	require.NoError(t, err)
	for _, uri := range []string{"ordinary", "https://example.com/config.yaml"} {
		handled, err := session.RegisterImport(t.Context(), uri)
		require.NoError(t, err)
		require.False(t, handled)
	}
	for _, path := range []string{"", "../outside", "/absolute", `live\app`, "live?ref=main", "live#fragment", "live/*"} {
		handled, err := session.RegisterImport(t.Context(), "terragrunt://"+path)
		require.True(t, handled)
		require.Error(t, err)
	}
	_, err = session.RegisterImport(t.Context(), "terragrunt://live")
	require.NoError(t, err)
	rebased, err := session.ForRevision(filepath.Join(t.TempDir(), "base"))
	require.NoError(t, err)
	revision := rebased.(*Session)
	require.Empty(t, revision.requests[0].Imports)
	require.Equal(t, []string{"live/**"}, revision.requests[0].IncludePaths)
	_, err = revision.RegisterImport(t.Context(), "terragrunt://previous")
	require.NoError(t, err)
	require.Equal(t, []string{"terragrunt://previous"}, revision.requests[0].Imports)
	require.Equal(t, []string{"terragrunt://live"}, session.requests[0].Imports)
	canceled, stop := context.WithCancel(t.Context())
	stop()
	_, err = session.RegisterImport(canceled, "terragrunt://live")
	require.ErrorIs(t, err, context.Canceled)
	cancel()
	_, err = session.RegisterImport(t.Context(), "terragrunt://live")
	require.ErrorIs(t, err, context.Canceled)
}

func TestConcurrentImportRegistrationDeduplicates(t *testing.T) {
	session, err := New(t.Context(), enabledConfig(t), "list", func(context.Context, []string) (map[string]Provider, error) { return nil, nil })
	require.NoError(t, err)
	var workers sync.WaitGroup
	for range 10 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := session.RegisterImport(t.Context(), "terragrunt://live")
			if err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	require.Len(t, session.requests, 1)
	require.Equal(t, []string{"terragrunt://live"}, session.requests[0].Imports)
	var _ schema.ComponentConfigurationImportRuntime = session
}
