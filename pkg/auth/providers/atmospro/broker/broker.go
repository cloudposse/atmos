// Package broker registers the Atmos Pro credential broker, which lazily provisions the
// github/sts integration in CI so Atmos can read private GitHub repositories (Terraform
// modules, `source:` components, vendored artifacts, remote imports) even though no stack
// claims the atmos/pro identity.
//
// It implements pkg/auth/broker.Provider and registers itself via init(). The command layer
// blank-imports this package so the broker is available before the first remote read. It is a
// separate package from the atmos/pro provider so it can depend on pkg/auth (to build a manager)
// without creating an import cycle.
package broker

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth"
	authbroker "github.com/cloudposse/atmos/pkg/auth/broker"
	atmosproIdentities "github.com/cloudposse/atmos/pkg/auth/identities/atmospro"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	atmosproProviders "github.com/cloudposse/atmos/pkg/auth/providers/atmospro"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func init() {
	authbroker.Register(&proBroker{})
}

// proBroker provisions the Atmos Pro github/sts integration from the ambient CI environment.
type proBroker struct{}

// Name returns the broker identifier (used only for logging).
func (proBroker) Name() string { return "atmos-pro/github-sts" }

// Enabled reports whether the broker should run: only in CI, and only when the auth config
// defines an auto-provisionable github/sts integration bound to an atmos/pro identity.
func (proBroker) Enabled(atmosConfig *schema.AtmosConfiguration) bool {
	return telemetry.IsCI() && findProGitHubSTSIdentity(atmosConfig) != ""
}

// Provision authenticates the atmos/pro identity (cached-session-first) and provisions its
// github/sts integration, returning the GIT_CONFIG_* environment to export. Reuse of fresh
// persisted tokens is handled inside the integration's Execute.
func (proBroker) Provision(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (map[string]string, error) {
	defer perf.Track(atmosConfig, "atmospro.broker.Provision")()

	identity := findProGitHubSTSIdentity(atmosConfig)
	if identity == "" {
		return nil, nil
	}

	mgr, err := auth.NewDefaultManager(&atmosConfig.Auth, atmosConfig.CliConfigPath)
	if err != nil {
		return nil, err
	}

	return mgr.EnsureIdentityEnvironment(ctx, identity)
}

// findProGitHubSTSIdentity returns the name of the atmos/pro identity bound to an
// auto-provisionable github/sts integration, or "" if none is configured. It resolves the
// integration's binding (via.identity directly, or via.provider → the atmos/pro identity
// routing through that provider) and verifies the kinds so unrelated integrations are ignored.
func findProGitHubSTSIdentity(atmosConfig *schema.AtmosConfiguration) string {
	authConfig := &atmosConfig.Auth
	if len(authConfig.Integrations) == 0 {
		return ""
	}

	for _, integration := range authConfig.Integrations {
		if integration.Kind != integrations.KindGitHubSTS || !autoProvisionEnabled(integration.Spec) || integration.Via == nil {
			continue
		}
		if name := proIdentityForBinding(authConfig, integration.Via); name != "" {
			return name
		}
	}

	return ""
}

// proIdentityForBinding resolves an integration's via binding to the name of the atmos/pro
// identity it provisions, or "" if the binding does not point at an atmos/pro identity.
func proIdentityForBinding(authConfig *schema.AuthConfig, via *schema.IntegrationVia) string {
	// Bound directly to an atmos/pro identity.
	if via.Identity != "" {
		if isProIdentity(authConfig, via.Identity) {
			return via.Identity
		}
		return ""
	}

	// Bound to an atmos/pro provider: find the atmos/pro identity routing through it.
	if via.Provider != "" && isProProvider(authConfig, via.Provider) {
		return identityForProvider(authConfig, via.Provider)
	}

	return ""
}

// autoProvisionEnabled reports whether an integration's auto_provision is enabled (default true).
func autoProvisionEnabled(spec *schema.IntegrationSpec) bool {
	if spec != nil && spec.AutoProvision != nil {
		return *spec.AutoProvision
	}
	return true
}

// isProIdentity reports whether the named identity exists and is of kind atmos/pro.
func isProIdentity(authConfig *schema.AuthConfig, name string) bool {
	identity, ok := authConfig.Identities[name]
	return ok && identity.Kind == atmosproIdentities.IdentityKind
}

// isProProvider reports whether the named provider exists and is of kind atmos/pro.
func isProProvider(authConfig *schema.AuthConfig, name string) bool {
	provider, ok := authConfig.Providers[name]
	return ok && provider.Kind == atmosproProviders.Kind
}

// identityForProvider returns the name of the atmos/pro identity routing through the given
// provider (via.provider), or "" if none is defined.
func identityForProvider(authConfig *schema.AuthConfig, providerName string) string {
	for name, identity := range authConfig.Identities {
		if identity.Kind != atmosproIdentities.IdentityKind || identity.Via == nil {
			continue
		}
		if identity.Via.Provider == providerName {
			return name
		}
	}
	return ""
}
