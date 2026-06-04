package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// processIntegrationCache tracks integrations already executed in this process.
// Auto-provisioned integrations (e.g. ECR login, EKS kubeconfig) should run only once per
// process regardless of how many times Authenticate is called or which AuthManager instance
// triggers them. The key is the integration's canonical target (e.g. "aws/ecr:account:region")
// rather than its config name, so two integration entries in a merged config that point to
// the same ECR registry or EKS cluster are deduplicated to a single execution.
var processIntegrationCache sync.Map

// resetProcessIntegrationCache clears the integration execution cache.
// Intended for use in tests to ensure isolation between test cases.
func resetProcessIntegrationCache() {
	processIntegrationCache.Clear()
}

// integrationTargetKey returns a canonical cache key for an integration.
// For ECR integrations: "aws/ecr:<account_id>:<region>" — two configs pointing at the same
// registry (e.g. a global + component-level duplicate) collapse to a single login.
// For EKS integrations: "aws/eks:<cluster_name>:<region>" — same dedup logic for kubeconfig.
// All other integration types fall back to the config name to preserve existing behaviour.
func integrationTargetKey(name string, cfg schema.Integration) string {
	switch cfg.Kind {
	case integrations.KindAWSECR:
		if cfg.Spec != nil && cfg.Spec.Registry != nil {
			return "aws/ecr:" + cfg.Spec.Registry.AccountID + ":" + cfg.Spec.Registry.Region
		}
	case integrations.KindAWSECRPublic:
		// ECR Public is a single global registry — no account or region discriminator needed.
		return integrations.KindAWSECRPublic
	case integrations.KindAWSEKS:
		if cfg.Spec != nil && cfg.Spec.Cluster != nil {
			return "aws/eks:" + cfg.Spec.Cluster.Name + ":" + cfg.Spec.Cluster.Region
		}
	}
	return name
}

// triggerIntegrations executes integrations that reference this identity with auto_provision enabled.
// This is a non-fatal operation - integration failures don't block authentication.
// Skipped when context contains skipIntegrationsKey (used by ExecuteIntegration to avoid duplicate execution).
// Each integration target is executed at most once per process (deduplicated via processIntegrationCache).
func (m *manager) triggerIntegrations(ctx context.Context, identityName string, creds types.ICredentials) {
	defer perf.Track(nil, "auth.Manager.triggerIntegrations")()

	// Check if integrations should be skipped (when called from ExecuteIntegration or eks-token).
	if ctx.Value(skipIntegrationsKey) != nil {
		log.Debug("Skipping auto-triggered integrations (explicit execution)", logKeyIdentity, identityName)
		return
	}

	// Find integrations that reference this identity and have auto_provision enabled.
	linkedIntegrations := m.findIntegrationsForIdentity(identityName, true)
	if len(linkedIntegrations) == 0 {
		return
	}

	log.Debug("Triggering linked integrations", logKeyIdentity, identityName, "count", len(linkedIntegrations))

	// Execute each linked integration, skipping any whose target has already been provisioned.
	// cacheKey is based on the integration's effective target (registry URL, cluster name) rather
	// than its config name so that duplicate entries from merged global+component configs that
	// point to the same endpoint are collapsed to a single execution.
	for _, integrationName := range linkedIntegrations {
		cacheKey := integrationTargetKey(integrationName, m.config.Integrations[integrationName])
		if _, alreadyRan := processIntegrationCache.LoadOrStore(cacheKey, struct{}{}); alreadyRan {
			log.Debug("Skipping already-executed integration", "integration", integrationName, "target", cacheKey)
			continue
		}
		if err := m.executeIntegration(ctx, integrationName, creds); err != nil {
			// Non-fatal: evict the cache entry so a retry on the next command is possible.
			processIntegrationCache.Delete(cacheKey)
			log.Warn("Integration failed", "integration", integrationName, "error", err)
		}
	}
}

// findIntegrationsForIdentity returns integration names that reference the given identity.
// If autoProvisionOnly is true, only returns integrations with auto_provision enabled (defaults to true).
func (m *manager) findIntegrationsForIdentity(identityName string, autoProvisionOnly bool) []string {
	if m.config == nil || m.config.Integrations == nil {
		return nil
	}

	// Resolve the identity's root provider once so integrations can bind via.provider.
	rootProvider := m.resolveProviderForIdentity(identityName)

	var result []string
	for name, integration := range m.config.Integrations {
		if integration.Via == nil {
			continue
		}

		// An integration applies to this identity if it references the identity directly
		// (via.identity) or binds to the identity's root provider (via.provider).
		matchesIdentity := integration.Via.Identity != "" && integration.Via.Identity == identityName
		matchesProvider := integration.Via.Provider != "" && rootProvider != "" && integration.Via.Provider == rootProvider
		if !matchesIdentity && !matchesProvider {
			continue
		}

		// If autoProvisionOnly, check if auto_provision is enabled (defaults to true).
		if autoProvisionOnly {
			autoProvision := true // Default to true when not specified.
			if integration.Spec != nil && integration.Spec.AutoProvision != nil {
				autoProvision = *integration.Spec.AutoProvision
			}
			if !autoProvision {
				continue
			}
		}

		result = append(result, name)
	}
	return result
}

// EnsureIdentityEnvironment authenticates the identity (preferring cached credentials) and
// provisions its auto_provision integrations, then returns the composed integration environment.
//
// Cached credentials are used whenever available — this is critical for the atmos/pro provider,
// whose GitHub Actions OIDC token is single-use server-side: re-running provider authentication
// would fail. When a valid session is cached, integrations are provisioned using those cached
// credentials (no second provider authentication); integrations that already hold fresh persisted
// state short-circuit their own mint. When no cached credentials exist, full authentication runs
// once and triggers the same auto_provision machinery.
//
// This is the generic primitive used by ambient credential brokers (see pkg/auth/broker) to
// provision integrations whose identity no stack claims.
func (m *manager) EnsureIdentityEnvironment(ctx context.Context, identityName string) (map[string]string, error) {
	defer perf.Track(nil, "auth.Manager.EnsureIdentityEnvironment")()

	if identityName == "" {
		return nil, fmt.Errorf(errFormatWithString, errUtils.ErrNilParam, identityNameKey)
	}

	// Resolve the identity name case-insensitively so the integration/env lookups below
	// (which key off the resolved name) match the keyring and config entries.
	if resolved, found := m.resolveIdentityName(identityName); found {
		identityName = resolved
	}

	// Prefer cached credentials; only authenticate when none are valid.
	whoami, err := m.GetCachedCredentials(ctx, identityName)
	if err != nil {
		log.Debug("No valid cached credentials for identity, authenticating", logKeyIdentity, identityName, "error", err)
		// Authenticate runs login-time integration triggers itself, so the returned whoami
		// is not needed here (the cached path below uses it only to trigger integrations).
		if _, err = m.Authenticate(ctx, identityName); err != nil {
			return nil, fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, err)
		}
	} else if whoami != nil && whoami.Credentials != nil {
		// Valid cached session — provision auto_provision integrations using the cached
		// credentials (no second provider authentication). Non-fatal, like login-time triggers.
		m.triggerIntegrations(ctx, identityName, whoami.Credentials)
	}

	// Compose the integration-contributed environment (e.g., github/sts GIT_CONFIG_*).
	return m.GetEnvironmentVariables(identityName)
}

// executeIntegration executes a single integration by name.
func (m *manager) executeIntegration(ctx context.Context, integrationName string, creds types.ICredentials) error {
	defer perf.Track(nil, "auth.Manager.executeIntegration")()

	// Look up integration config.
	if m.config.Integrations == nil {
		return fmt.Errorf("%w: no integrations configured", errUtils.ErrIntegrationNotFound)
	}

	integrationConfig, exists := m.config.Integrations[integrationName]
	if !exists {
		return fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrIntegrationNotFound, integrationName)
	}

	// Create integration instance.
	integration, err := integrations.Create(&integrations.IntegrationConfig{
		Name:   integrationName,
		Config: &integrationConfig,
		Realm:  m.realm.Value,
	})
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrIntegrationFailed, err)
	}

	// Execute the integration.
	if err := integration.Execute(ctx, creds); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrIntegrationFailed, err)
	}

	log.Debug("Integration executed successfully", "integration", integrationName)
	return nil
}

// ExecuteIntegration exposes integration execution for the standalone ecr-login command.
// This authenticates the integration's linked identity first, then executes the integration.
// Auto-triggered integrations are skipped during authentication to avoid duplicate execution.
func (m *manager) ExecuteIntegration(ctx context.Context, integrationName string) error {
	defer perf.Track(nil, "auth.Manager.ExecuteIntegration")()

	// Look up integration config.
	if m.config.Integrations == nil {
		return fmt.Errorf("%w: no integrations configured", errUtils.ErrIntegrationNotFound)
	}

	integrationConfig, exists := m.config.Integrations[integrationName]
	if !exists {
		return fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrIntegrationNotFound, integrationName)
	}

	// Get the identity from via.identity.
	if integrationConfig.Via == nil || integrationConfig.Via.Identity == "" {
		return fmt.Errorf("%w: integration '%s' has no identity configured", errUtils.ErrIntegrationFailed, integrationName)
	}
	identityName := integrationConfig.Via.Identity

	// Authenticate the linked identity with integrations skipped.
	// We skip auto-triggered integrations because we'll execute this specific integration explicitly below.
	// This prevents duplicate execution when the requested integration is also auto-provisioned.
	ctxSkipIntegrations := context.WithValue(ctx, skipIntegrationsKey, true)
	whoami, err := m.Authenticate(ctxSkipIntegrations, identityName)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, err)
	}

	// Use credentials from authentication result.
	if whoami.Credentials == nil {
		return fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, errUtils.ErrIdentityCredentialsNone)
	}

	log.Debug("Authenticated identity for integration", "identity", identityName, "whoami", whoami.Identity)

	// Create and execute the integration.
	integration, err := integrations.Create(&integrations.IntegrationConfig{
		Name:   integrationName,
		Config: &integrationConfig,
		Realm:  m.realm.Value,
	})
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrIntegrationFailed, err)
	}

	return integration.Execute(ctx, whoami.Credentials)
}

// ExecuteIdentityIntegrations executes all integrations that reference this identity.
// This authenticates the identity first, then executes all integrations linked to it.
// Auto-triggered integrations are skipped during authentication to avoid duplicate execution.
func (m *manager) ExecuteIdentityIntegrations(ctx context.Context, identityName string) error {
	defer perf.Track(nil, "auth.Manager.ExecuteIdentityIntegrations")()

	// Verify the identity exists.
	_, exists := m.config.Identities[identityName]
	if !exists {
		return fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrIdentityNotFound, identityName)
	}

	// Find all integrations that reference this identity (not just auto_provision ones).
	linkedIntegrations := m.findIntegrationsForIdentity(identityName, false)
	if len(linkedIntegrations) == 0 {
		return fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrNoLinkedIntegrations, identityName)
	}

	// Authenticate the identity with integrations skipped.
	// We skip auto-triggered integrations because we'll execute all linked integrations explicitly below.
	// This prevents duplicate execution for integrations that are also auto-provisioned.
	ctxSkipIntegrations := context.WithValue(ctx, skipIntegrationsKey, true)
	whoami, err := m.Authenticate(ctxSkipIntegrations, identityName)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, err)
	}

	// Use credentials from authentication result.
	if whoami.Credentials == nil {
		return fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, errUtils.ErrIdentityCredentialsNone)
	}

	log.Debug("Authenticated identity for integrations", "identity", identityName, "whoami", whoami.Identity)

	// Execute each linked integration.
	for _, integrationName := range linkedIntegrations {
		if err := m.executeIntegration(ctx, integrationName, whoami.Credentials); err != nil {
			return fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIntegrationFailed, integrationName, err)
		}
	}

	return nil
}

// RevokeEphemeralIntegrations revokes and cleans up ephemeral integrations (currently
// github/sts) linked to the identity. It is intended for command-end teardown in CI:
// minted tokens are revoked directly against the provider so they don't outlive the command.
//
// Per-integration revoke_on_exit (spec) overrides globalDefault, which overrides the
// built-in default (true). When the effective value is false the integration is skipped,
// allowing users to keep credentials alive for a separate CI step.
//
// Best-effort: failures are logged and returned joined; callers typically ignore the error.
func (m *manager) RevokeEphemeralIntegrations(ctx context.Context, identityName string, globalDefault *bool) error {
	defer perf.Track(nil, "auth.Manager.RevokeEphemeralIntegrations")()

	linkedIntegrations := m.findIntegrationsForIdentity(identityName, false)
	if len(linkedIntegrations) == 0 {
		return nil
	}

	var errs []error
	for _, integrationName := range linkedIntegrations {
		integrationConfig, exists := m.config.Integrations[integrationName]
		if !exists {
			continue
		}

		// Only ephemeral kinds are revoked at command-end. Other kinds (aws/ecr, aws/eks)
		// rely on natural expiry and logout-time cleanup.
		if integrationConfig.Kind != integrations.KindGitHubSTS {
			continue
		}

		if !resolveRevokeOnExit(&integrationConfig, globalDefault) {
			log.Debug("Skipping command-end revoke (revoke_on_exit disabled)", logKeyIntegration, integrationName)
			continue
		}

		integration, err := integrations.Create(&integrations.IntegrationConfig{
			Name:   integrationName,
			Config: &integrationConfig,
			Realm:  m.realm.Value,
		})
		if err != nil {
			log.Warn("Failed to create integration for revoke", logKeyIntegration, integrationName, "error", err)
			continue
		}

		if err := integration.Cleanup(ctx); err != nil {
			log.Warn("Command-end revoke failed", logKeyIntegration, integrationName, "error", err)
			errs = append(errs, err)
		} else {
			log.Debug("Command-end revoke succeeded", logKeyIntegration, integrationName)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// resolveRevokeOnExit resolves the effective revoke_on_exit value:
// integration spec → globalDefault → built-in default (true).
func resolveRevokeOnExit(integrationConfig *schema.Integration, globalDefault *bool) bool {
	if integrationConfig.Spec != nil && integrationConfig.Spec.RevokeOnExit != nil {
		return *integrationConfig.Spec.RevokeOnExit
	}
	if globalDefault != nil {
		return *globalDefault
	}
	return true
}

// GetIntegration returns the integration config by name.
func (m *manager) GetIntegration(integrationName string) (*schema.Integration, error) {
	if m.config.Integrations == nil {
		return nil, fmt.Errorf("%w: no integrations configured", errUtils.ErrIntegrationNotFound)
	}

	integrationConfig, exists := m.config.Integrations[integrationName]
	if !exists {
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrIntegrationNotFound, integrationName)
	}

	return &integrationConfig, nil
}
