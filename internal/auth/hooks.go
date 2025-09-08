package auth

import (
	"context"
	"fmt"

	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/credentials"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/internal/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/go-viper/mapstructure/v2"
)

type (
	Provider        = types.Provider
	Identity        = types.Identity
	AuthManager     = types.AuthManager
	CredentialStore = types.CredentialStore
	Validator       = types.Validator
)

// TerraformPreHook runs before Terraform commands to set up authentication.
func TerraformPreHook(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	atmosLevel, authLevel := getConfigLogLevels(atmosConfig)
	log.SetLevel(authLevel)
	defer log.SetLevel(atmosLevel)
	log.SetPrefix("atmos-auth")
	defer log.SetPrefix("")

	// TODO: verify if we need to use Decode, or if we can use the merged auth config directly
	// Use the merged auth configuration from stackInfo
	// ComponentAuthSection already contains the deep-merged auth config from component + inherits + atmos.yaml
	// Converted to typed struct when needed
	var authConfig schema.AuthConfig
	err := mapstructure.Decode(stackInfo.ComponentAuthSection, &authConfig)
	if err != nil {
		return fmt.Errorf("%w: failed to decode component auth config: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	// Skip if no auth config (check the merged config, not the original)
	if len(authConfig.Providers) == 0 && len(authConfig.Identities) == 0 {
		log.Debug("No auth configuration found, skipping authentication")
		return nil
	}

	// Create auth manager components
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	// Create auth manager with merged configuration and stack info (so identities can mutate it)
	authManager, err := NewAuthManager(
		&authConfig,
		credStore,
		validator,
		stackInfo,
	)
	if err != nil {
		return fmt.Errorf("%w: failed to create auth manager: %v", errUtils.ErrAuthManager, err)
	}

	// Determine target identity: stack info identity (CLI flag) or default identity
	ctx := context.Background()
	var targetIdentityName string

	if stackInfo.Identity != "" {
		// This is set by the CLI Flag
		targetIdentityName = stackInfo.Identity
	} else {
		targetIdentityName, err = authManager.GetDefaultIdentity()
		if err != nil {
			return fmt.Errorf("%w: failed to get default identity: %v", errUtils.ErrDefaultIdentity, err)
		}
	}
	if targetIdentityName == "" {
		return fmt.Errorf("%w: no default identity configured for authentication", errUtils.ErrDefaultIdentity)
	}

	log.Info("Authenticating with identity", "identity", targetIdentityName)

	// Authenticate with target identity
	whoami, err := authManager.Authenticate(ctx, targetIdentityName)
	if err != nil {
		return fmt.Errorf("%w: failed to authenticate with identity %q: %v", errUtils.ErrAuthenticationFailed, targetIdentityName, err)
	}

	log.Debug("Authentication successful", "identity", whoami.Identity, "expiration", whoami.Expiration)

	_ = utils.PrintAsYAMLToFileDescriptor(atmosConfig, stackInfo.ComponentEnvSection)
	return nil
}

func getConfigLogLevels(atmosConfig *schema.AtmosConfiguration) (log.Level, log.Level) {
	atmosLevel := log.GetLevel()
	if atmosConfig.Logs.Level != "" {
		if l, err := log.ParseLevel(atmosConfig.Logs.Level); err == nil {
			atmosLevel = l
		}
	}
	// Determine auth log level (fallback to atmos level).
	authLevel := atmosLevel
	if atmosConfig.Auth.Logs.Level != "" {
		if l, err := log.ParseLevel(atmosConfig.Auth.Logs.Level); err == nil {
			authLevel = l
		}
	}
	return atmosLevel, authLevel
}
