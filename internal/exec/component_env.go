package exec

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConvertComponentEnvSectionToList converts ComponentEnvSection map to ComponentEnvList slice.
// ComponentEnvSection is populated by auth hooks and stack config env sections.
// This function is used by terraform, helmfile, and packer execution to prepare environment variables.
func ConvertComponentEnvSectionToList(info *schema.ConfigAndStacksInfo) {
	defer perf.Track(nil, "exec.ConvertComponentEnvSectionToList")()

	for k, v := range info.ComponentEnvSection {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("%s=%v", k, v))
	}
}
