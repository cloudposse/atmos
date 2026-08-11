package emulator

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	emutarget "github.com/cloudposse/atmos/pkg/emulator/target"
	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// contributorName is the unique provider-config contributor identifier.
const contributorName = "emulator"

// emulatorKindSuffix is the suffix every emulator identity kind shares (the kinds follow
// the `<target>/emulator` convention), used to recover the target from the kind.
const emulatorKindSuffix = "/emulator"

func init() {
	generator.RegisterProviderContributor(providerContributor{})
}

// providerContributor injects the emulator's Terraform provider fragment (endpoints,
// skip-flags, dummy creds) for components bound to an emulator identity. Env vars
// (AWS_ENDPOINT_URL, …) carry the endpoint into the subprocess; this contributor adds
// the provider behavior flags that env cannot set, so no hand-written providers.tf.
type providerContributor struct{}

// Name returns the contributor identifier.
func (providerContributor) Name() string {
	defer perf.Track(nil, "componentemulator.providerContributor.Name")()

	return contributorName
}

// Contribute resolves the component's selected identity; if it binds to an emulator
// with a Terraform provider fragment, returns that fragment keyed by provider name.
// Returns nil when the component is not emulator-bound (or the target has no fragment,
// e.g. kubernetes).
func (providerContributor) Contribute(ctx context.Context, genCtx *generator.GeneratorContext) (map[string]any, error) {
	defer perf.Track(nil, "componentemulator.providerContributor.Contribute")()

	providerName, emulatorRef, ok := emulatorBinding(genCtx)
	if !ok {
		return nil, nil
	}

	profile, err := resolveEmulatorProfile(ctx, emulatorRef)
	if err != nil {
		return nil, err
	}
	if len(profile.Provider) == 0 {
		return nil, nil
	}

	return map[string]any{providerName: profile.Provider}, nil
}

// emulatorBinding reports whether the component in genCtx is bound to an emulator
// identity whose target has a Terraform provider fragment, returning the Terraform
// provider name and the referenced emulator INSTANCE value.
func emulatorBinding(genCtx *generator.GeneratorContext) (providerName, emulatorRef string, ok bool) {
	if genCtx == nil || genCtx.AtmosConfig == nil || genCtx.StackInfo == nil {
		return "", "", false
	}
	identityName := selectedIdentity(genCtx.StackInfo, &genCtx.AtmosConfig.Auth)
	if identityName == "" {
		return "", "", false
	}
	identity, found := genCtx.AtmosConfig.Auth.Identities[identityName]
	if !found || !types.IsEmulatorIdentityKind(identity.Kind) || identity.Emulator == "" {
		return "", "", false
	}
	// Recover the target from the kind (`<target>/emulator`) and ask the target package
	// for its Terraform provider name. Kubernetes (and any non-Terraform-provider target)
	// contributes nothing here.
	target := strings.TrimSuffix(identity.Kind, emulatorKindSuffix)
	providerName, ok = emutarget.TerraformProviderName(target)
	if !ok {
		return "", "", false
	}
	return providerName, identity.Emulator, true
}

// selectedIdentity returns the component's effective identity: the --identity flag
// value (ignoring the interactive-select sentinel), otherwise the configured default
// identity. Components do not declare their identity in stack config — they run as the
// flag-selected or default identity — so there is no per-component identity field to read.
func selectedIdentity(info *schema.ConfigAndStacksInfo, authConfig *schema.AuthConfig) string {
	if info.Identity != "" && info.Identity != cfg.IdentityFlagSelectValue {
		return info.Identity
	}
	if authConfig != nil {
		for name := range authConfig.Identities {
			if authConfig.Identities[name].Default {
				return name
			}
		}
	}
	return ""
}
