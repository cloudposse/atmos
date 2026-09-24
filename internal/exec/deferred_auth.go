package exec

import (
	"maps"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// deferredTargetAuth resolves the referenced component, with its own stack and defaults.
// Authentication errors propagate; they must never select an unrelated ambient identity.
func deferredTargetAuth(ac *schema.AtmosConfiguration, component, stack string, parents ...auth.AuthManager) (auth.AuthManager, error) {
	section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		AtmosConfig: ac, Component: component, Stack: stack,
		ProcessTemplates: false, ProcessYamlFunctions: false,
	})
	if err != nil {
		return nil, err
	}
	if GetComponentRemoteStateBackendStaticType(&section) != nil {
		return nil, nil
	}
	section = inheritDeferredAuth(section, parents)
	info := &schema.ConfigAndStacksInfo{Component: component, Stack: stack, ComponentSection: section}
	if err := deferred.ResolveAuth(ac, info); err != nil {
		return nil, err
	}
	if info.AuthDisabled {
		return &authContextWrapper{stackInfo: info}, nil
	}
	manager, _ := info.AuthManager.(auth.AuthManager)
	return manager, nil
}

func inheritDeferredAuth(section map[string]any, parents []auth.AuthManager) map[string]any {
	targetAuth, _ := section[cfg.AuthSectionName].(map[string]any)
	if hasDefaultIdentity(targetAuth) || len(parents) == 0 || parents[0] == nil {
		return section
	}
	if parent := parents[0].GetStackInfo(); parent != nil {
		parentAuth, _ := parent.ComponentSection[cfg.AuthSectionName].(map[string]any)
		if hasDefaultIdentity(parentAuth) {
			section = maps.Clone(section)
			section[cfg.AuthSectionName] = parentAuth
		}
	}
	return section
}

// Only actual credential consumers call this hook. Terraform references resolve
// their target's identity separately, and masked secrets never read a backend.
func prepareDeferredYAMLAuth(ac *schema.AtmosConfiguration, input string, skip []string, info *schema.ConfigAndStacksInfo) error {
	if ac.DeferredAuth == nil {
		return nil
	}
	for _, tag := range []string{
		u.AtmosYamlFuncAwsAccountID, u.AtmosYamlFuncAwsCallerIdentityArn,
		u.AtmosYamlFuncAwsCallerIdentityUserID, u.AtmosYamlFuncAwsRegion, u.AtmosYamlFuncAwsOrganizationID,
	} {
		if input == tag && !skipFunc(skip, tag) {
			return deferred.ResolveAuth(ac, info)
		}
	}
	if strings.HasPrefix(input, u.AtmosYamlFuncSecret+" ") && !skipFunc(skip, u.AtmosYamlFuncSecret) &&
		info != nil && !info.SecretsMaskOnly {
		return deferred.PrepareSecretAuth(ac, input, info)
	}
	return nil
}
