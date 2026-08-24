package target

import (
	emu "github.com/cloudposse/atmos/pkg/emulator"
	"github.com/cloudposse/atmos/pkg/perf"
)

// TerraformProviderName maps an emulator target to the Terraform provider it contributes
// a provider-config fragment for (aws → "aws", gcp → "google", azure → "azurerm"),
// returning ok=false for targets with no Terraform provider (kubernetes, vault,
// registry). This per-cloud mapping lives in the target package — which owns every other
// per-cloud profile detail — rather than leaking into the generic provider-config
// contributor.
func TerraformProviderName(target string) (string, bool) {
	defer perf.Track(nil, "emulator.target.TerraformProviderName")()

	switch target {
	case emu.TargetAWS:
		return "aws", true
	case emu.TargetGCP:
		return "google", true
	case emu.TargetAzure:
		return "azurerm", true
	default:
		return "", false
	}
}
