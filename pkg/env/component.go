package env

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PrepareComponentEnv converts ComponentEnvSection map to ComponentEnvList slice.
// ComponentEnvSection is populated by auth hooks and stack config env sections.
// This function is used by terraform, helmfile, and packer execution to prepare environment variables.
func PrepareComponentEnv(info *schema.ConfigAndStacksInfo) {
	defer perf.Track(nil, "env.PrepareComponentEnv")()

	for k, v := range info.ComponentEnvSection {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("%s=%v", k, v))
	}
}
