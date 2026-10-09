package auth

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TargetAuthOptions describes a delivery scope independently of its parent auth.
type TargetAuthOptions struct {
	AtmosConfig *schema.AtmosConfiguration
	Info        *schema.ConfigAndStacksInfo
	// TargetName names the delivery target in diagnostics; it has no effect on resolution.
	TargetName   string
	TargetConfig map[string]any
	// RequestedIdentity is the caller's original CLI/environment selection.
	RequestedIdentity string
	// DeclaredIdentityWins makes an identity the target itself declares (`auth.identity`
	// or a `default: true` entry in `auth.identities`) take precedence over
	// RequestedIdentity. Output references use it: a producer's deployed stack lives
	// in the account its own target selects, whichever identity the consumer was run as.
	DeclaredIdentityWins bool
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
		warnTargetIdentityBypassed(options)
		resolved := *info
		resolved.Identity, resolved.AuthManager, resolved.AuthContext = "", nil, nil
		resolved.AuthDisabled = true
		return &resolved, nil
	}
	raw, exists := targetConfig[cfg.AuthSectionName]
	if !exists || raw == nil {
		return info, nil
	}
	targetAuth, err := validateTargetAuthBlock(options, raw)
	if err != nil {
		return nil, err
	}
	if len(targetAuth) == 0 {
		return info, nil
	}

	return authenticateTarget(options, targetAuth, requestedIdentity)
}

// ValidateTargetAuth statically validates a delivery target's auth block without
// authenticating anything, so dry runs and pre-flight checks reject the same
// misconfigurations a real run would.
func ValidateTargetAuth(options *TargetAuthOptions) error {
	defer perf.Track(options.AtmosConfig, "auth.ValidateTargetAuth")()

	raw, exists := options.TargetConfig[cfg.AuthSectionName]
	if !exists || raw == nil {
		return nil
	}
	_, err := validateTargetAuthBlock(options, raw)
	return err
}

// authenticateTarget creates an independent manager from the merged target scope.
func authenticateTarget(options *TargetAuthOptions, targetAuth map[string]any, requestedIdentity string) (*schema.ConfigAndStacksInfo, error) {
	atmosConfig, info, targetConfig := options.AtmosConfig, options.Info, options.TargetConfig
	merged, err := MergeComponentAuthFromConfig(&atmosConfig.Auth, info.ComponentSection, atmosConfig, cfg.AuthSectionName)
	if err != nil {
		return nil, targetAuthError(options, errUtils.ErrProvisionTargetAuthInvalid).WithCause(err).Err()
	}
	merged, err = MergeComponentAuthFromConfig(merged, targetConfig, atmosConfig, cfg.AuthSectionName)
	if err != nil {
		return nil, targetAuthError(options, errUtils.ErrProvisionTargetAuthInvalid).WithCause(err).Err()
	}
	identity, err := targetIdentity(options, requestedIdentity, targetAuth)
	if err != nil {
		return nil, err
	}
	createManager := options.CreateManager
	if createManager == nil {
		createManager = CreateAndAuthenticateManagerWithAtmosConfigForStack
	}
	manager, err := createManager(identity, merged, cfg.IdentityFlagSelectValue, atmosConfig, info.Stack)
	if err != nil {
		return nil, targetAuthFailure(options, identity, err)
	}
	if manager == nil {
		return nil, noIdentityError(options, requestedIdentity, errUtils.ErrFailedToInitializeAuthManager)
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
// a bare --identity prompt, ahead of the target's explicit or default identity,
// unless the caller asked for the target's own declaration to win.
func targetIdentity(options *TargetAuthOptions, requested string, targetAuth map[string]any) (string, error) {
	info := options.Info
	if declared := declaredTargetIdentity(targetAuth); options.DeclaredIdentityWins && declared != "" {
		return declared, nil
	}
	if options.DeclaredIdentityWins {
		requested = ""
	}
	if requested == cfg.IdentityFlagSelectValue {
		if info.Identity != "" && info.Identity != cfg.IdentityFlagSelectValue {
			return info.Identity, nil
		}
		if info.AuthContext != nil && info.AuthContext.AWS != nil {
			return info.AuthContext.AWS.Profile, nil
		}
		return "", noIdentityError(options, requested, errUtils.ErrFailedToInitializeAuthManager)
	}
	if requested != "" {
		return requested, nil
	}
	// The block was validated, so a present `identity` is a nonempty string.
	identity, _ := targetAuth[targetAuthIdentityKey].(string)
	return identity, nil
}

// targetAuthFailure explains why the identity a target selected could not be used.
func targetAuthFailure(options *TargetAuthOptions, identity string, cause error) error {
	builder := targetAuthError(options, errUtils.ErrProvisionTargetAuthFailed).
		WithCause(fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, cause))
	if identity != "" {
		builder = builder.WithContext("identity", identity)
	}
	switch {
	case errors.Is(cause, errUtils.ErrIdentityNotFound):
		builder = builder.WithHint("Define the identity under `auth.identities` in atmos.yaml or the stack, or correct the identity name in the target's `auth` block.")
	case errors.Is(cause, errUtils.ErrMultipleDefaultIdentities):
		builder = builder.WithHint("Mark exactly one identity `default: true`, or set `auth.identity` on the target.")
	default:
		builder = builder.WithHint("The target's own identity is used for this destination; Atmos does not fall back to the component's credentials. Run `atmos auth login` for that identity or fix the target's `auth` block.")
	}
	return builder.Err()
}
