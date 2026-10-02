package exec

import (
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// createCloudFormationTargetAuthManager is shared by both output-reference forms.
var createCloudFormationTargetAuthManager = auth.CreateAndAuthenticateManagerWithAtmosConfigForStack

// cloudFormationOutputScope preserves the caller's original identity choice while
// resolving credentials in the referenced stack, including stack-scoped emulators.
func cloudFormationOutputScope(stack string, caller *schema.ConfigAndStacksInfo, disabled bool) *schema.ConfigAndStacksInfo {
	info := schema.ConfigAndStacksInfo{}
	if caller != nil {
		info = *caller
	}
	info.Stack, info.AuthDisabled = stack, disabled
	return &info
}

// cloudFormationOutputAuthForSections follows only a default direct stack target.
// Publish-only, Git and StackSet destinations do not describe a single deployed
// stack; their authentication must not affect CloudFormation output references.
func cloudFormationOutputAuthForSections(ac *schema.AtmosConfiguration, sections map[string]any, scope *schema.ConfigAndStacksInfo, parent *schema.AuthContext) (*schema.AuthContext, error) {
	provision, _ := sections[cfg.ProvisionSectionName].(map[string]any)
	selected, err := target.SelectTargetWithDefault(provision, "", "default", cfg.CloudFormationComponentType)
	if err != nil {
		return nil, err
	}
	if selected.Kind != cfg.CloudFormationComponentType {
		return parent, nil
	}
	info := *scope
	info.ComponentSection, info.AuthContext = sections, parent
	requested := info.Identity
	if info.RequestedIdentity != nil {
		requested = *info.RequestedIdentity
	}
	resolved, err := auth.ResolveTargetAuth(&auth.TargetAuthOptions{
		AtmosConfig: ac, Info: &info, TargetConfig: selected.Config,
		RequestedIdentity: requested, CreateManager: createCloudFormationTargetAuthManager,
	})
	if err != nil {
		return nil, err
	}
	return resolved.AuthContext, nil
}
