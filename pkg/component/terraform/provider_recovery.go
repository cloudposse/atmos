package terraform

import (
	"slices"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/autoinit"
)

// providerRecoveryInfo scopes an unusable shared-cache bypass to the init subprocess. The ordinary
// invocation configuration still identifies the marker: package selections and installation rules
// are unchanged, and the component-local recovery cache remains valid after this subprocess exits.
func providerRecoveryInfo(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) (*schema.ConfigAndStacksInfo, error) {
	cacheDir, err := autoinit.PrepareProviderInstallation(autoinit.InputsFromInfo(atmosConfig, info, componentPath, ""))
	if err != nil {
		return nil, err
	}
	if cacheDir == "" {
		return info, nil
	}
	clone := *info
	clone.ComponentEnvList = append(slices.Clone(info.ComponentEnvList), "TF_PLUGIN_CACHE_DIR="+cacheDir)
	return &clone, nil
}

// prepareExplicitProviderInit keeps explicit init's marker and provider preparation equivalent to
// implicit init without changing the command's ordinary execution or metadata capture context.
func prepareExplicitProviderInit(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) (*schema.ConfigAndStacksInfo, error) {
	if info.SubCommand != subcommandInit || info.DryRun {
		return info, nil
	}
	autoinit.InvalidateFromInfo(newAutoInitInputs(atmosConfig, info, componentPath, ConstructTerraformComponentVarfileName(info)))
	return providerRecoveryInfo(atmosConfig, info, componentPath)
}
