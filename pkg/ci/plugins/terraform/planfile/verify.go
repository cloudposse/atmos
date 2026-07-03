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
//	explicit CLI override (--verify-plan / --verify-plan=false)
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

// IsPlanRequired reports whether `atmos terraform deploy` must find a stored
// planfile to verify against (as opposed to ResolveVerifyMode, which governs the
// drift comparison once a stored plan exists). Resolution:
//
//   - When verification resolves to off, nothing is downloaded or verified, so a
//     stored plan is never required (this also short-circuits an explicit
//     `required: true` paired with `--verify-plan=false`).
//   - An explicit components.terraform.planfiles.required wins.
//   - Unset: required tracks verify strictness — true only when verification
//     resolves to fail (e.g. under CI with storage configured). This makes a
//     fail-by-default deploy fail loudly on a missing plan instead of silently
//     applying an unverified fresh plan; local deploys (no CI, no storage)
//     resolve to off and are never required.
//
// The cliOverride is empty when neither flag was set.
func IsPlanRequired(atmosConfig *schema.AtmosConfiguration, ciEnabled bool, cliOverride schema.PlanfileVerifyMode) bool {
	defer perf.Track(atmosConfig, "planfile.IsPlanRequired")()

	verifyMode := ResolveVerifyMode(atmosConfig, ciEnabled, cliOverride)
	if verifyMode == schema.PlanfileVerifyOff {
		return false
	}
	if atmosConfig != nil {
		if required := atmosConfig.Components.Terraform.Planfiles.Required; required != nil {
			return *required
		}
	}

	// Unset: require a stored plan only when verification is strict.
	return verifyMode == schema.PlanfileVerifyFail
}
