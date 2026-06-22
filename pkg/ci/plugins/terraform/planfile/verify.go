package planfile

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StorageConfigured reports whether explicit planfile storage is configured
// (via a default store, a priority list, or named stores). When no planfile
// configuration exists, upload/download/verify are skipped.
func StorageConfigured(pf *schema.PlanfilesConfig) bool {
	defer perf.Track(nil, "planfile.StorageConfigured")()

	return pf.Default != "" || len(pf.Priority) > 0 || len(pf.Stores) > 0
}

// ResolveVerifyMode resolves the effective planfile drift-verification mode for
// `atmos terraform deploy`, applying precedence:
//
//	explicit CLI override (--verify-plan/--no-verify-plan)
//	  > config (components.terraform.planfiles.verify)
//	  > default: fail when CI is enabled and planfile storage is configured, else off.
//
// cliOverride is empty when neither flag was set.
func ResolveVerifyMode(atmosConfig *schema.AtmosConfiguration, ciEnabled bool, cliOverride schema.PlanfileVerifyMode) schema.PlanfileVerifyMode {
	defer perf.Track(atmosConfig, "planfile.ResolveVerifyMode")()

	if cliOverride != "" {
		return cliOverride
	}
	if atmosConfig == nil {
		return schema.PlanfileVerifyOff
	}

	pf := atmosConfig.Components.Terraform.Planfiles
	if pf.Verify != "" {
		return pf.Verify
	}
	if ciEnabled && StorageConfigured(&pf) {
		return schema.PlanfileVerifyFail
	}
	return schema.PlanfileVerifyOff
}
