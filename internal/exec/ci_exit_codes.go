package exec

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mapCIExitCode checks the CI exit code mapping and returns the remapped exit code.
// When global ci.enabled is true and components.terraform.ci.exit_codes maps the
// given exit code to true, it returns 0 (success). Otherwise the original exit code
// is returned unchanged.
func mapCIExitCode(atmosConfig *schema.AtmosConfiguration, exitCode int) int {
	if !atmosConfig.CI.Enabled {
		return exitCode
	}

	exitCodes := atmosConfig.Components.Terraform.CI.ExitCodes
	if exitCodes == nil {
		return exitCode
	}

	if success, ok := exitCodes[exitCode]; ok && success {
		log.Debug("CI exit code mapping: remapping to success",
			"original_exit_code", exitCode,
		)
		return 0
	}

	return exitCode
}
