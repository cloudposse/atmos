package auth

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// triggerIntegrations executes integrations that reference this identity with auto_provision enabled.
// This is a non-fatal operation - integration failures don't block authentication.
// Skipped when context contains skipIntegrationsKey (used by ExecuteIntegration to avoid duplicate execution).
func (m *manager) triggerIntegrations(ctx context.Context, identityName string, creds types.ICredentials) {
	defer perf.Track(nil, "auth.Manager.triggerIntegrations")()

	// Check if integrations should be skipped (when called from ExecuteIntegration).
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

	// Execute each linked integration.
	for _, integrationName := range linkedIntegrations {
		if err := m.executeIntegration(ctx, integrationName, creds); err != nil {
			// Non-fatal: log warning and continue.
			log.Warn("Integration failed", "integration", integrationName, "error", err)
		}
	}
}

// findIntegrationsForIdentity returns integration names that reference the given identity.
// If autoProvisionOnly is true, only returns integrations with auto_provision enabled (defaults to true).
func (m *manager) findIntegrationsForIdentity(identityName string, autoProvisionOnly bool) []string {
	if m.config.Integrations == nil {
		return nil
	}

	var result []string
	for name, integration := range m.config.Integrations {
		// Check if this integration references the given identity.
		if integration.Via == nil || integration.Via.Identity != identityName {
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
			return fmt.Errorf("integration '%s' failed: %w", integrationName, err)
		}
	}

	return nil
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
