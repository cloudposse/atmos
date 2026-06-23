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

// ResolveMissingMode resolves what `atmos terraform deploy` does when no stored
// planfile was found to verify against (as opposed to ResolveVerifyMode, which
// governs the drift comparison once a stored plan exists). Precedence:
//
//	explicit CLI override (--verify-plan/--no-verify-plan)
//	  > config (components.terraform.planfiles.on_missing)
//	  > default: tracks the resolved verify mode.
//
// Tracking the verify mode by default means a fail-by-default CI deploy fails
// loudly when the expected stored plan is absent, instead of silently applying
// an unverified fresh plan; local deploys (no CI, no storage) resolve to off.
//
// The cliOverride is empty when neither flag was set.
func ResolveMissingMode(atmosConfig *schema.AtmosConfiguration, ciEnabled bool, cliOverride schema.PlanfileVerifyMode) schema.PlanfileVerifyMode {
	defer perf.Track(atmosConfig, "planfile.ResolveMissingMode")()

	if cliOverride != "" {
		return cliOverride
	}
	if atmosConfig == nil {
		return schema.PlanfileVerifyOff
	}

	if onMissing := atmosConfig.Components.Terraform.Planfiles.OnMissing; onMissing != "" {
		return onMissing
	}

	// Unset: track the verify mode so missing-plan strictness mirrors drift strictness.
	return ResolveVerifyMode(atmosConfig, ciEnabled, cliOverride)
}
