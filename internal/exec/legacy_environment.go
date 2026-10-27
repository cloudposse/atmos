package exec

import (
	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConvertComponentEnvSectionToList prepares the component's environment.
//
// Deprecated: Use env.PrepareComponentEnv.
func ConvertComponentEnvSectionToList(info *schema.ConfigAndStacksInfo) {
	defer perf.Track(nil, "exec.ConvertComponentEnvSectionToList")()

	env.PrepareComponentEnv(info)
}
