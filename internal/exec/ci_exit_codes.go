package exec

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultCIExitCodes are the default exit code mappings when ci.enabled is true
// but components.terraform.ci.exit_codes is not explicitly configured.
// Exit code 0 (no changes) and 2 (changes detected) are treated as success;
// exit code 1 (error) is preserved as failure.
var defaultCIExitCodes = map[int]bool{
	0: true,
	1: false,
	2: true,
}

// mapCIExitCode checks the CI exit code mapping and returns the remapped exit code.
// When global ci.enabled is true and components.terraform.ci.exit_codes maps the
// given exit code to true, it returns 0 (success). Otherwise the original exit code
// is returned unchanged. If no exit_codes are configured, sensible defaults apply.
func mapCIExitCode(atmosConfig *schema.AtmosConfiguration, exitCode int) int {
	if !atmosConfig.CI.Enabled {
		return exitCode
	}

	exitCodes := atmosConfig.Components.Terraform.CI.ExitCodes
	if exitCodes == nil {
		exitCodes = defaultCIExitCodes
	}

	if success, ok := exitCodes[exitCode]; ok && success {
		log.Debug("CI exit code mapping: remapping to success",
			"original_exit_code", exitCode,
		)
		return 0
	}

	return exitCode
}
