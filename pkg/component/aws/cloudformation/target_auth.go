package cloudformation

import (
	"fmt"
	"maps"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// createTargetAuthManager keeps target authentication independently testable.
var createTargetAuthManager = auth.CreateAndAuthenticateManagerWithAtmosConfigForStack

// ResolveTargetAuth authenticates one CFN delivery target without modifying its parent.
func ResolveTargetAuth(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, targetConfig map[string]any, requestedIdentity string) (*schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "cloudformation.ResolveTargetAuth")()

	resolved, err := auth.ResolveTargetAuth(&auth.TargetAuthOptions{
		AtmosConfig: atmosConfig, Info: info, TargetConfig: targetConfig,
		RequestedIdentity: requestedIdentity, CreateManager: createTargetAuthManager,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationIdentityResolutionFailed, err)
	}
	return resolved, nil
}

// clientForOperation uses a direct deploy target for stack operations, while
// StackSet create/update resolve their own target. Delete/instances deliberately
// use component auth: those verbs do not select or require a StackSet target.
func clientForOperation(octx *opContext, operation Operation) (CloudFormationClient, error) {
	block, err := operationTargetConfig(octx, operation)
	if err != nil {
		return nil, err
	}
	info, err := ResolveTargetAuth(octx.AtmosConfig, octx.Info, block, octx.RequestedIdentity)
	if err != nil {
		return nil, err
	}
	awsConfig, err := buildAWSConfig(octx.Ctx, info, resolveRegion(info.ComponentSection))
	if err != nil {
		return nil, err
	}
	return newClient(awsConfig, resolveEndpointURL(info)), nil
}

// operationTargetConfig selects the operation-specific auth scope without requiring targets for StackSet teardown.
func operationTargetConfig(octx *opContext, operation Operation) (map[string]any, error) {
	provision, _ := octx.Info.ComponentSection[cfg.ProvisionSectionName].(map[string]any)
	name, _ := octx.Flags[targetKey].(string)
	switch operation {
	case OperationStackSetCreate, OperationStackSetUpdate:
		selected, err := resolveStackSetTarget(provision, name)
		if err != nil {
			return nil, err
		}
		targets, _ := provision["targets"].(map[string]any)
		block, _ := targets[selected.Name].(map[string]any)
		return block, nil
	case OperationStackSetDelete, OperationStackSetInstances:
		return nil, nil
	default:
		selected, err := target.SelectTargetWithDefault(provision, name, "default", cfg.CloudFormationComponentType)
		if err != nil {
			return nil, err
		}
		if selected.Kind == cfg.CloudFormationComponentType {
			return selected.Config, nil
		}
		return nil, nil
	}
}

// externalTargetAuth adapts the full target auth block to the generic Git
// target's auth.identity selector. Keep the repository identity when no target
// or CLI override exists, and never change the original target configuration.
func externalTargetAuth(octx *opContext, selected *target.SelectedTarget) (*schema.ConfigAndStacksInfo, map[string]any, error) {
	info, err := ResolveTargetAuth(octx.AtmosConfig, octx.Info, selected.Config, octx.RequestedIdentity)
	if err != nil {
		return nil, nil, err
	}
	block := maps.Clone(selected.Config)
	if block == nil {
		block = make(map[string]any)
	}
	authBlock, _ := block[cfg.AuthSectionName].(map[string]any)
	if len(authBlock) == 0 && octx.RequestedIdentity == "" {
		return info, block, nil
	}
	authBlock = maps.Clone(authBlock)
	if authBlock == nil {
		authBlock = make(map[string]any)
	}
	authBlock["identity"] = activeIdentityName(info)
	block[cfg.AuthSectionName] = authBlock
	return info, block, nil
}

// targetConfigByName reads the unmodified target block from component configuration.
func targetConfigByName(componentSection map[string]any, name string) map[string]any {
	provision, _ := componentSection[cfg.ProvisionSectionName].(map[string]any)
	targets, _ := provision["targets"].(map[string]any)
	block, _ := targets[name].(map[string]any)
	return block
}
