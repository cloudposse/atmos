package auth

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// buildWhoamiInfo creates a WhoamiInfo struct from identity and credentials.
func (m *manager) buildWhoamiInfo(identityName string, creds types.ICredentials) *types.WhoamiInfo {
	providerName := m.getProviderForIdentity(identityName)

	info := &types.WhoamiInfo{
		Provider:    providerName,
		Identity:    identityName,
		LastUpdated: time.Now(),
	}

	// Populate high-level fields from the concrete credential type.
	info.Credentials = creds
	creds.BuildWhoamiInfo(info)
	if expTime, err := creds.GetExpiration(); err == nil && expTime != nil {
		info.Expiration = expTime
	}
	// Get environment variables.
	if identity, exists := m.identities[identityName]; exists {
		if env, err := identity.Environment(); err == nil {
			info.Environment = env
		}
	}

	// Store credentials in the keystore and set a reference handle.
	// Use the identity name as the opaque handle for retrieval.
	if err := m.credentialStore.Store(identityName, creds); err == nil {
		info.CredentialsRef = identityName
		// Note: We keep info.Credentials populated for validation purposes.
		// The Credentials field is marked with json:"-" yaml:"-" tags to prevent
		// accidental serialization, so there's no security risk in keeping it.
	}

	return info
}

// buildWhoamiInfoFromEnvironment creates a WhoamiInfo struct when using noop keyring.
// This is used when credentials are managed externally (e.g., in containers with mounted files).
// Instead of retrieving credentials from the keyring, it gets information from the identity's
// environment configuration and loads credentials from identity storage if available.
func (m *manager) buildWhoamiInfoFromEnvironment(identityName string) *types.WhoamiInfo {
	providerName := m.getProviderForIdentity(identityName)

	info := &types.WhoamiInfo{
		Provider:    providerName,
		Identity:    identityName,
		LastUpdated: time.Now(),
	}

	log.Debug("buildWhoamiInfoFromEnvironment called", logKeyIdentity, identityName)

	// Get environment variables and try to load credentials from identity storage.
	identity, exists := m.identities[identityName]
	log.Debug("Identity lookup", logKeyIdentity, identityName, "exists", exists)
	if exists {
		// Get environment variables.
		if env, err := identity.Environment(); err == nil {
			info.Environment = env
		}

		// Try to load credentials from identity-managed storage (files, etc.).
		// This enables credential validation in whoami when using noop keyring.
		ctx := context.Background()
		creds, err := identity.LoadCredentials(ctx)
		log.Debug("LoadCredentials result",
			logKeyIdentity, identityName,
			"creds_nil", creds == nil,
			"error", err,
		)
		if err == nil && creds != nil {
			info.Credentials = creds
			// Populate whoami info fields (expiration, region, etc.) from credentials.
			creds.BuildWhoamiInfo(info)
			log.Debug("Loaded credentials from identity storage",
				logKeyIdentity, identityName,
			)
		} else if err != nil {
			log.Debug("Failed to load credentials from identity storage",
				logKeyIdentity, identityName,
				"error", err,
			)
		}
	}

	return info
}
