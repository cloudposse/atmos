package auth

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TargetAuthOptions describes a delivery scope independently of its parent auth.
type TargetAuthOptions struct {
	AtmosConfig       *schema.AtmosConfiguration
	Info              *schema.ConfigAndStacksInfo
	TargetConfig      map[string]any
	RequestedIdentity string
	// CreateManager defaults to the stack-aware production authenticator.
	CreateManager func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, string) (AuthManager, error)
}

// ResolveTargetAuth applies global, component, then target auth without changing
// the component's active manager or credentials. RequestedIdentity is the original
// CLI/environment selection, not an identity auto-selected for the component.
func ResolveTargetAuth(options *TargetAuthOptions) (*schema.ConfigAndStacksInfo, error) {
	atmosConfig, info := options.AtmosConfig, options.Info
	targetConfig, requestedIdentity := options.TargetConfig, options.RequestedIdentity
	defer perf.Track(atmosConfig, "auth.ResolveTargetAuth")()

	requestedIdentity = cfg.NormalizeIdentityValue(requestedIdentity)
	if info.AuthDisabled || requestedIdentity == cfg.IdentityFlagDisabledValue {
		resolved := *info
		resolved.Identity, resolved.AuthManager, resolved.AuthContext = "", nil, nil
		resolved.AuthDisabled = true
		return &resolved, nil
	}
	raw, exists := targetConfig[cfg.AuthSectionName]
	if !exists || raw == nil {
		return info, nil
	}
	targetAuth, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: provision target auth must be a mapping", errUtils.ErrInvalidAuthConfig)
	}
	if len(targetAuth) == 0 {
		return info, nil
	}

	return authenticateTarget(options, targetAuth, requestedIdentity)
}

// authenticateTarget creates an independent manager from the merged target scope.
func authenticateTarget(options *TargetAuthOptions, targetAuth map[string]any, requestedIdentity string) (*schema.ConfigAndStacksInfo, error) {
	atmosConfig, info, targetConfig := options.AtmosConfig, options.Info, options.TargetConfig
	merged, err := MergeComponentAuthFromConfig(&atmosConfig.Auth, info.ComponentSection, atmosConfig, cfg.AuthSectionName)
	if err != nil {
		return nil, err
	}
	merged, err = MergeComponentAuthFromConfig(merged, targetConfig, atmosConfig, cfg.AuthSectionName)
	if err != nil {
		return nil, err
	}
	identity, err := targetIdentity(requestedIdentity, info, targetAuth)
	if err != nil {
		return nil, err
	}
	createManager := options.CreateManager
	if createManager == nil {
		createManager = CreateAndAuthenticateManagerWithAtmosConfigForStack
	}
	manager, err := createManager(identity, merged, cfg.IdentityFlagSelectValue, atmosConfig, info.Stack)
	if err != nil {
		return nil, fmt.Errorf("%w: provision target: %w", errUtils.ErrFailedToInitializeAuthManager, err)
	}
	if manager == nil {
		return nil, fmt.Errorf("%w: provision target auth did not select an identity", errUtils.ErrFailedToInitializeAuthManager)
	}
	resolved := *info
	resolved.Identity, resolved.AuthManager, resolved.AuthContext = identity, manager, nil
	if chain := manager.GetChain(); len(chain) > 0 {
		resolved.Identity = chain[len(chain)-1]
	}
	if stackInfo := manager.GetStackInfo(); stackInfo != nil {
		resolved.AuthContext = stackInfo.AuthContext
	}
	return &resolved, nil
}

// targetIdentity preserves a user-selected identity, including a choice made by
// a bare --identity prompt, ahead of the target's explicit or default identity.
func targetIdentity(requested string, info *schema.ConfigAndStacksInfo, targetAuth map[string]any) (string, error) {
	if requested == cfg.IdentityFlagSelectValue {
		if info.Identity != "" && info.Identity != cfg.IdentityFlagSelectValue {
			return info.Identity, nil
		}
		if info.AuthContext != nil && info.AuthContext.AWS != nil {
			return info.AuthContext.AWS.Profile, nil
		}
		return "", fmt.Errorf("%w: selected identity is unavailable", errUtils.ErrFailedToInitializeAuthManager)
	}
	if requested != "" {
		return requested, nil
	}
	if raw, exists := targetAuth["identity"]; exists {
		identity, ok := raw.(string)
		if !ok || identity == "" {
			return "", fmt.Errorf("%w: provision target auth.identity must be a nonempty string", errUtils.ErrInvalidAuthConfig)
		}
		return identity, nil
	}
	return "", nil
}
