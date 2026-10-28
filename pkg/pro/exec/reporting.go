package exec

import (
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AttachComponentReporting runs after stack resolution and execution, preserving
// the resolved metadata and original command without changing error identity.
func AttachComponentReporting(result *error, info *schema.ConfigAndStacksInfo, componentType, command string) {
	defer perf.Track(nil, "proexec.AttachComponentReporting")()

	if id := ExecutionID(); id != "" && *result != nil {
		snapshot := *info
		snapshot.ComponentType, snapshot.SubCommand = componentType, command
		*result = errUtils.WithReportingContext(*result, &snapshot, id)
	}
}
