package configuration

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StateProvider is optional; discovery and preparation do not authorize state resolution.
type StateProvider interface {
	ResolveState(context.Context, *pluginv1.ComponentRequest) (*pluginv1.ResolveComponentStateResponse, error)
}

// ResolveState locates state with no evaluator commands, writes, network, or output reads.
func (s *Session) ResolveState(ctx context.Context, info *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	defer perf.Track(nil, "configuration.Session.ResolveState")()

	if _, owned := info.ComponentSettingsSection["configuration"]; !owned {
		return nil, nil
	}
	selected, err := s.componentRequest(ctx, info)
	if err != nil {
		return nil, err
	}
	resolver, ok := selected.provider.(StateProvider)
	if !ok {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	result, err := resolver.ResolveState(ctx, selected.request)
	if err != nil {
		return nil, err
	}
	if result == nil || result.State == nil {
		return nil, errUtils.ErrPluginProtocol
	}
	state := result.State
	backend, err := adapter.DecodeObject(state.Backend)
	if err != nil {
		return nil, err
	}
	if state.BackendType == "local" {
		resolveLocalBackend(backend, state.WorkingDirectory)
	}
	return &schema.ComponentConfigurationState{BackendType: state.BackendType, Backend: backend, Workspace: state.Workspace, WorkingDirectory: state.WorkingDirectory}, nil
}
