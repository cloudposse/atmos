package configuration

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ResolveState locates state with no evaluator commands, writes, network, or output reads.
func (s *Session) ResolveState(ctx context.Context, info *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	defer perf.Track(nil, "configuration.Session.ResolveState")()

	if !schema.ConfigurationOwned(info.ComponentSettingsSection) {
		return nil, nil
	}
	selected, err := s.componentRequest(ctx, info)
	if err != nil {
		return nil, err
	}
	// Discovery and preparation do not authorize state resolution; the request grants no effects.
	result, err := selected.provider.ResolveState(ctx, selected.request)
	if err != nil {
		return nil, err
	}
	adapter.RenderLogs(logScope(selected.request, info, &s.warnings), result.GetLogs())
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
