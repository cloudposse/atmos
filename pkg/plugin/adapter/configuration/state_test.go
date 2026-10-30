package configuration

import (
	"context"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type stateProviderFixture struct {
	Provider
	resolve func(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error)
}

func (p *stateProviderFixture) ResolveState(ctx context.Context, req *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
	return p.resolve(ctx, req)
}

func TestSessionResolvesStateWithoutPreparation(t *testing.T) {
	config := enabledConfig(t)
	mock := NewMockProvider(gomock.NewController(t))
	mock.EXPECT().ComponentType().Return("terraform")
	mock.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{unit(t, "network")}}, nil)
	backend, err := adapter.EncodeObject(map[string]any{"path": "state.tfstate"})
	require.NoError(t, err)
	provider := &stateProviderFixture{Provider: mock, resolve: func(_ context.Context, req *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		require.Nil(t, req.Effects)
		require.Equal(t, "output", req.Operation.Command)
		require.Equal(t, "selected", req.Invocation.Environment["IDENTITY"])
		require.Equal(t, "stage", req.Invocation.Environment["TF_WORKSPACE"])
		return &pluginv1.ResolveComponentStateResponse{State: &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, Workspace: "stage", WorkingDirectory: config.BasePathAbsolute}}, nil
	}}
	session, err := New(t.Context(), config, "describe", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	info := &schema.ConfigAndStacksInfo{Stack: "live", ComponentFromArg: "network", SubCommand: "output", TerraformWorkspace: "stage", ComponentSettingsSection: map[string]any{"configuration": true}, SanitizedEnv: []string{"IDENTITY=selected"}}
	state, err := session.ResolveState(t.Context(), info)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(config.BasePathAbsolute, "state.tfstate"), state.Backend["path"])
	require.Equal(t, "stage", state.Workspace)
	provider.resolve = func(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		return nil, context.Canceled
	}
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, context.Canceled)
	provider.resolve = func(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		return &pluginv1.ResolveComponentStateResponse{}, nil
	}
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	provider.resolve = func(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error) {
		return &pluginv1.ResolveComponentStateResponse{State: &pluginv1.ComponentStateIdentity{Backend: &pluginv1.ComponentValue{}}}, nil
	}
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	session.registry["terragrunt"] = mock
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	info.ComponentFromArg = "absent"
	_, err = session.ResolveState(t.Context(), info)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	info.ComponentSettingsSection = nil
	state, err = session.ResolveState(t.Context(), info)
	require.NoError(t, err)
	require.Nil(t, state)
}
