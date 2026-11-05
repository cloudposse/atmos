package exec

import (
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/proexec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// attachComponentReporting runs after stack resolution and execution, preserving
// the resolved metadata and original command without changing error identity.
func attachComponentReporting(result *error, info *schema.ConfigAndStacksInfo, componentType, command string) {
	if id := proexec.ExecutionID(); id != "" && *result != nil {
		snapshot := *info
		snapshot.ComponentType, snapshot.SubCommand = componentType, command
		*result = errUtils.WithReportingContext(*result, &snapshot, id)
	}
}
