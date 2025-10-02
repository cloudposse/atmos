package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GetComponentRemoteStateBackendStaticType returns the `remote_state_backend` section for a component in a stack.
func GetComponentRemoteStateBackendStaticType(sections *map[string]any) map[string]any {
	defer perf.Track(nil, "exec.GetComponentRemoteStateBackendStaticType")()

	var remoteStateBackend map[string]any
	var remoteStateBackendType string
	var ok bool

	if remoteStateBackendType, ok = (*sections)[cfg.RemoteStateBackendTypeSectionName].(string); !ok {
		return nil
	}
	if remoteStateBackendType != cfg.StaticSectionName {
		return nil
	}
	if remoteStateBackend, ok = (*sections)[cfg.RemoteStateBackendSectionName].(map[string]any); ok {
		return remoteStateBackend
	}
	return nil
}
