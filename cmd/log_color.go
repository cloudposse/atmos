package cmd

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/startup"
)

// globalColorDisabled combines configured and early command-line opt-outs before applying forced color.
func globalColorDisabled(config *schema.AtmosConfiguration) bool {
	return startup.GlobalColorDisabled(config)
}
