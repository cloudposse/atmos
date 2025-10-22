package credentials

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// cachedCredential holds validated credentials with expiration info.
type cachedCredential struct {
	creds      types.ICredentials
	expiration *time.Time
	validatedAt time.Time
}

// noopKeyringStore implements a credential store for containerized environments.
// When system keyring is unavailable (e.g., no dbus in containers):
// - Loads credentials from AWS files on-demand
// - Validates credentials with provider APIs
// - Caches validated credentials in memory (including expiration)
// This allows Atmos to work in containers using host-authenticated credentials.
type noopKeyringStore struct {
	// In-memory cache of validated credentials
	// Key: alias (identity name), Value: validated credentials with expiration
	cache map[string]cachedCredential
}

// newNoopKeyringStore creates a new no-op keyring store with in-memory cache.
func newNoopKeyringStore() *noopKeyringStore {
	return &noopKeyringStore{
		cache: make(map[string]cachedCredential),
	}
}

// Store always succeeds but doesn't persist anything.
// This allows the auth flow to continue even when keyring is unavailable.
func (s *noopKeyringStore) Store(alias string, creds types.ICredentials) error {
	defer perf.Track(nil, "credentials.noopKeyringStore.Store")()

	// No-op: credentials won't be persisted to keyring.
	// They will still be written to credentials files by the auth system.
	return nil
}

// Retrieve validates credentials using the AWS SDK and caches validation results.
// The SDK automatically loads credentials from files based on environment variables.
// Returns ErrCredentialsNotFound after validation - signals to use credentials from SDK.
func (s *noopKeyringStore) Retrieve(alias string) (types.ICredentials, error) {
	defer perf.Track(nil, "credentials.noopKeyringStore.Retrieve")()

	// Check if we recently validated (within last 5 minutes).
	if cached, ok := s.cache[alias]; ok {
		timeSinceValidation := time.Since(cached.validatedAt)
		if timeSinceValidation < 5*time.Minute {
			// Recently validated, check if expired.
			if cached.expiration != nil && time.Now().After(*cached.expiration) {
				return nil, fmt.Errorf("%w: credentials expired, please refresh on host machine", errUtils.ErrExpiredCredentials)
			}
			log.Debug("Credentials recently validated", "alias", alias, "validatedAgo", timeSinceValidation.Round(time.Second))
			return nil, ErrCredentialsNotFound
		}
	}

	// Validate credentials by calling AWS STS GetCallerIdentity.
	// SDK loads credentials automatically from AWS_SHARED_CREDENTIALS_FILE.
	expiration, err := s.validateAWSCredentials(context.Background())
	if err != nil {
		return nil, fmt.Errorf("%w: please refresh credentials on host machine: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Warn if expiring soon (< 15 minutes).
	if expiration != nil {
		timeUntilExpiry := time.Until(*expiration)
		if timeUntilExpiry < 15*time.Minute && timeUntilExpiry > 0 {
			log.Warn("Credentials expire soon", "alias", alias, "expiresIn", timeUntilExpiry.Round(time.Minute))
		}
		log.Debug("Credentials validated successfully", "alias", alias, "expiration", *expiration)
	} else {
		log.Debug("Credentials validated successfully (no expiration)", "alias", alias)
	}

	// Cache validation result.
	s.cache[alias] = cachedCredential{
		creds:       nil, // Not storing actual credentials
		expiration:  expiration,
		validatedAt: time.Now(),
	}

	// Return "not found" - signals that credentials should come from AWS SDK/environment.
	return nil, ErrCredentialsNotFound
}

// Delete always succeeds (no-op).
func (s *noopKeyringStore) Delete(alias string) error {
	defer perf.Track(nil, "credentials.noopKeyringStore.Delete")()

	return nil
}

// List returns empty list (no credentials stored).
func (s *noopKeyringStore) List() ([]string, error) {
	defer perf.Track(nil, "credentials.noopKeyringStore.List")()

	return []string{}, nil
}

// IsExpired always returns true (no credentials available).
func (s *noopKeyringStore) IsExpired(alias string) (bool, error) {
	defer perf.Track(nil, "credentials.noopKeyringStore.IsExpired")()

	return true, ErrCredentialsNotFound
}

// GetAny always returns "not found" error.
func (s *noopKeyringStore) GetAny(key string, dest interface{}) error {
	defer perf.Track(nil, "credentials.noopKeyringStore.GetAny")()

	return ErrCredentialsNotFound
}

// SetAny always succeeds (no-op).
func (s *noopKeyringStore) SetAny(key string, value interface{}) error {
	defer perf.Track(nil, "credentials.noopKeyringStore.SetAny")()

	return nil
}

// validateAWSCredentials validates AWS credentials by calling STS GetCallerIdentity.
// The AWS SDK automatically loads credentials from environment variables:
// - AWS_SHARED_CREDENTIALS_FILE
// - AWS_CONFIG_FILE
// - AWS_PROFILE
// Returns expiration time if available (temporary credentials), nil otherwise.
func (s *noopKeyringStore) validateAWSCredentials(ctx context.Context) (*time.Time, error) {
	defer perf.Track(nil, "credentials.noopKeyringStore.validateAWSCredentials")()

	// Load AWS config from environment (SDK handles everything).
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create STS client.
	stsClient := sts.NewFromConfig(cfg)

	// Call GetCallerIdentity to validate credentials.
	_, err = stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to validate credentials: %w", err)
	}

	// Try to get expiration from the credentials provider.
	// For temporary credentials, the SDK may expose this.
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err == nil && !creds.Expires.IsZero() {
		return &creds.Expires, nil
	}

	// No expiration available (long-term credentials).
	return nil, nil
}
