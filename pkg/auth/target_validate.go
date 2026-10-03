package auth

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	targetAuthIdentityKey   = "identity"
	targetAuthIdentitiesKey = "identities"
	targetAuthDefaultKey    = "default"
)

// targetAuthKeys are the keys a target `auth` block accepts: everything a
// component `auth` block accepts, plus the `identity` shorthand that selects
// one identity by name.
var targetAuthKeys = map[string]bool{
	targetAuthIdentityKey:   true,
	targetAuthIdentitiesKey: true,
	"providers":             true,
	"integrations":          true,
	"console":               true,
	"logs":                  true,
	"keyring":               true,
	"realm":                 true,
}

// warnTargetAuthBypass reports that disabled authentication ignores a target's identity.
// It is a variable so tests can observe the warning without capturing terminal output.
var warnTargetAuthBypass = func(message string) { ui.Warning(message) }

// bypassWarnings records which targets already produced a bypass warning, so a
// target resolved several times in one run is reported once.
var bypassWarnings sync.Map

// targetAuthError starts an error for a target's auth scope carrying the target,
// component and stack the user needs to locate the configuration.
func targetAuthError(options *TargetAuthOptions, base error) *errUtils.ErrorBuilder {
	builder := errUtils.Build(base)
	if options.TargetName != "" {
		builder = builder.WithContext("target", options.TargetName)
	}
	if info := options.Info; info != nil {
		if component := targetComponentName(options); component != "" {
			builder = builder.WithContext("component", component)
		}
		if info.Stack != "" {
			builder = builder.WithContext("stack", info.Stack)
		}
	}
	return builder
}

// targetComponentName returns the component the target belongs to.
func targetComponentName(options *TargetAuthOptions) string {
	if options.Info == nil {
		return ""
	}
	if options.Info.ComponentFromArg != "" {
		return options.Info.ComponentFromArg
	}
	return options.Info.Component
}

// noIdentityError reports a target auth block that selected no identity.
func noIdentityError(options *TargetAuthOptions, requested string, cause error) error {
	explanation := "The target's `auth` block neither names an identity nor marks one `default: true`, and no default identity is inherited."
	if requested == cfg.IdentityFlagSelectValue {
		explanation = "The identity prompt did not produce an identity for this target."
	}
	return targetAuthError(options, errUtils.ErrProvisionTargetAuthNoIdentity).
		WithCause(cause).
		WithExplanation(explanation).
		WithHint("Set `auth.identity: <name>` on the target, mark one of its `auth.identities` `default: true`, or pass `--identity=<name>`.").
		Err()
}

// validateTargetAuthBlock checks a target's raw `auth` value strictly: a typo or
// an unselected identity must fail loudly rather than run as the component's identity.
func validateTargetAuthBlock(options *TargetAuthOptions, raw any) (map[string]any, error) {
	block, ok := raw.(map[string]any)
	if !ok {
		return nil, targetAuthError(options, errUtils.ErrProvisionTargetAuthInvalid).
			WithCause(errUtils.ErrInvalidAuthConfig).
			WithExplanationf("A target's `auth` value must be a mapping, but it is %T.", raw).
			WithHint("Write the block as `auth: {identity: <name>}` or `auth: {identities: {<name>: {default: true}}}`.").
			Err()
	}
	if err := rejectUnknownTargetAuthKeys(options, block); err != nil {
		return nil, err
	}
	if len(block) == 0 {
		return block, nil
	}
	if err := validateTargetAuthIdentities(options, block); err != nil {
		return nil, err
	}
	return block, nil
}

// rejectUnknownTargetAuthKeys fails on a key a target auth block does not support.
func rejectUnknownTargetAuthKeys(options *TargetAuthOptions, block map[string]any) error {
	var unknown []string
	for key := range block {
		if !targetAuthKeys[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	supported := make([]string, 0, len(targetAuthKeys))
	for key := range targetAuthKeys {
		supported = append(supported, key)
	}
	sort.Strings(supported)
	return targetAuthError(options, errUtils.ErrProvisionTargetAuthUnknownKey).
		WithCause(errUtils.ErrInvalidAuthConfig).
		WithExplanationf("The target's `auth` block has unsupported key(s): %s. An unrecognized key is ignored by the authenticator, so the target would silently run as the component's identity.", strings.Join(unknown, ", ")).
		WithHintf("Supported keys are: %s. Check the spelling, for example `identity` rather than `identitty`.", strings.Join(supported, ", ")).
		Err()
}

// validateTargetAuthIdentities checks the identity selection a target's own block makes.
func validateTargetAuthIdentities(options *TargetAuthOptions, block map[string]any) error {
	if err := validateTargetIdentityName(options, block); err != nil {
		return err
	}
	rawIdentities, exists := block[targetAuthIdentitiesKey]
	if !exists || rawIdentities == nil {
		return nil
	}
	identities, ok := rawIdentities.(map[string]any)
	if !ok {
		return targetAuthError(options, errUtils.ErrProvisionTargetAuthInvalid).
			WithCause(errUtils.ErrInvalidAuthConfig).
			WithExplanationf("`auth.identities` must be a mapping of identity names, but it is %T.", rawIdentities).
			Err()
	}
	defaults := defaultIdentityNames(identities)
	if len(defaults) > 1 {
		return targetAuthError(options, errUtils.ErrProvisionTargetAuthInvalid).
			WithCause(errUtils.ErrMultipleDefaultIdentities).
			WithExplanationf("The target's `auth.identities` marks several identities `default: true`: %s.", strings.Join(defaults, ", ")).
			WithHint("Keep `default: true` on exactly one identity, or select one with `auth.identity`.").
			Err()
	}
	return requireTargetIdentitySelected(options, block, identities, defaults)
}

// validateTargetIdentityName checks that a present `auth.identity` is a nonempty string.
func validateTargetIdentityName(options *TargetAuthOptions, block map[string]any) error {
	raw, exists := block[targetAuthIdentityKey]
	if !exists {
		return nil
	}
	if name, ok := raw.(string); ok && name != "" {
		return nil
	}
	return targetAuthError(options, errUtils.ErrProvisionTargetAuthInvalid).
		WithCause(errUtils.ErrInvalidAuthConfig).
		WithExplanation("`auth.identity` names the identity the target authenticates as, so it must be a nonempty string.").
		WithHint("Set `auth.identity` to the name of a defined identity, or remove the key to use the component's identity.").
		Err()
}

// requireTargetIdentitySelected rejects identities a target declares without selecting any of them,
// unless the caller's explicit --identity makes the selection.
func requireTargetIdentitySelected(options *TargetAuthOptions, block, identities map[string]any, defaults []string) error {
	_, hasIdentity := block[targetAuthIdentityKey]
	explicitRequest := cfg.NormalizeIdentityValue(options.RequestedIdentity) != "" && !options.DeclaredIdentityWins
	if len(identities) == 0 || len(defaults) > 0 || hasIdentity || explicitRequest {
		return nil
	}
	names := make([]string, 0, len(identities))
	for name := range identities {
		names = append(names, name)
	}
	sort.Strings(names)
	return targetAuthError(options, errUtils.ErrProvisionTargetAuthNoIdentity).
		WithCause(errUtils.ErrInvalidAuthConfig).
		WithExplanationf("The target's `auth.identities` declares %s but none is marked `default: true`, and `auth.identity` is not set, so nothing selects an identity for this target.", strings.Join(names, ", ")).
		WithHintf("Add `default: true` under `auth.identities.%s`, or set `auth.identity: %s` on the target.", names[0], names[0]).
		Err()
}

// defaultIdentityNames returns the sorted names of identities marked `default: true`.
func defaultIdentityNames(identities map[string]any) []string {
	var names []string
	for name, raw := range identities {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if isDefault, _ := entry[targetAuthDefaultKey].(bool); isDefault {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// declaredTargetIdentity returns the identity a target's own auth block selects,
// either by name (`identity`) or as the single `default: true` identity it declares.
func declaredTargetIdentity(block map[string]any) string {
	if name, ok := block[targetAuthIdentityKey].(string); ok && name != "" {
		return name
	}
	identities, _ := block[targetAuthIdentitiesKey].(map[string]any)
	if defaults := defaultIdentityNames(identities); len(defaults) > 0 {
		return defaults[0]
	}
	return ""
}

// warnTargetIdentityBypassed tells the user that disabled authentication ignores
// the identity a target declares. Each target is reported once per process.
func warnTargetIdentityBypassed(options *TargetAuthOptions) {
	block, _ := options.TargetConfig[cfg.AuthSectionName].(map[string]any)
	declared := declaredTargetIdentity(block)
	if declared == "" {
		return
	}
	stack := ""
	if options.Info != nil {
		stack = options.Info.Stack
	}
	key := strings.Join([]string{stack, targetComponentName(options), options.TargetName, declared}, "\x00")
	if _, seen := bypassWarnings.LoadOrStore(key, true); seen {
		return
	}
	subject := "A provision target"
	if options.TargetName != "" {
		subject = fmt.Sprintf("Provision target %q", options.TargetName)
	}
	warnTargetAuthBypass(fmt.Sprintf("%s declares identity %q, but Atmos authentication is disabled (--identity=false or ATMOS_IDENTITY=false), so the AWS SDK default credential chain is used instead.", subject, declared))
}
