package exec

import (
	"maps"

	"github.com/cloudposse/atmos/pkg/auth"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
	stackdeferred "github.com/cloudposse/atmos/pkg/stack/deferred"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// deferredTargetAuth resolves the referenced component, with its own stack and defaults.
// Authentication errors propagate; they must never select an unrelated ambient identity.
func deferredTargetAuth(ac *schema.AtmosConfiguration, component, stack string, parents ...auth.AuthManager) (auth.AuthManager, error) {
	manager, _, err := deferredTargetAuthAndCache(ac, component, stack, parents...)
	return manager, err
}

// deferredTargetAuthAndCache binds values to the target's full configuration, not
// just its identity name. No cache is returned when authentication fails.
func deferredTargetAuthAndCache(ac *schema.AtmosConfiguration, component, stack string, parents ...auth.AuthManager) (auth.AuthManager, *deferred.ValueCache, error) {
	section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		AtmosConfig: ac, Component: component, Stack: stack,
		ProcessTemplates: false, ProcessYamlFunctions: false,
	})
	if err != nil {
		return nil, nil, err
	}
	section = inheritDeferredAuth(section, parents)
	info := &schema.ConfigAndStacksInfo{Component: component, Stack: stack, ComponentSection: section}
	if len(parents) > 0 && parents[0] != nil {
		if parent := parents[0].GetStackInfo(); parent != nil {
			info.AuthDisabled = parent.AuthDisabled
		}
	}
	if GetComponentRemoteStateBackendStaticType(&section) != nil {
		return nil, stackdeferred.CacheFor(ac, info), nil
	}
	if err := authdeferred.ResolveAuth(ac, info); err != nil {
		return nil, nil, err
	}
	cache := stackdeferred.CacheFor(ac, info)
	if info.AuthDisabled {
		return &authContextWrapper{stackInfo: info}, cache, nil
	}
	manager, _ := info.AuthManager.(auth.AuthManager)
	return manager, cache, nil
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

// Deferred resolution is authoritative; a missing result must not resurrect a
// previous caller's credentials. Eager execution retains its existing fallback.
func resolvedTargetAuthContext(ac *schema.AtmosConfiguration, manager auth.AuthManager, fallback *schema.AuthContext, disabled bool) *schema.AuthContext {
	if disabled {
		return nil
	}
	if manager != nil {
		if info := manager.GetStackInfo(); info != nil && info.AuthContext != nil {
			return info.AuthContext
		}
	}
	if authdeferred.IsDeferred(ac.AuthManager) {
		return nil
	}
	return fallback
}

// Only actual credential consumers call this hook. Terraform references resolve
// their target's identity separately. Secret and store adapters own their dependencies.
func prepareDeferredYAMLAuth(ac *schema.AtmosConfiguration, input string, skip []string, info *schema.ConfigAndStacksInfo) error {
	if !authdeferred.IsDeferred(ac.AuthManager) {
		return nil
	}
	for _, tag := range []string{
		u.AtmosYamlFuncAwsAccountID, u.AtmosYamlFuncAwsCallerIdentityArn,
		u.AtmosYamlFuncAwsCallerIdentityUserID, u.AtmosYamlFuncAwsRegion, u.AtmosYamlFuncAwsOrganizationID,
	} {
		if input == tag && !skipFunc(skip, tag) {
			return authdeferred.ResolveAuth(ac, info)
		}
	}
	return nil
}
