package auth

import (
	"context"
	"errors"
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

var (
	ErrTerraformPreHook  = errors.New("terraform pre-hook failed")
	ErrNoDefaultIdentity = errors.New("no default identity configured for authentication")
)

// TerraformPreHook runs before Terraform commands to set up authentication.
func TerraformPreHook(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	if stackInfo == nil {
		errUtils.CheckErrorAndPrint(fmt.Errorf("%w: stack info is nil", errUtils.ErrInvalidAuthConfig), "TerraformPreHook", "")
		return fmt.Errorf("%w: stack info is nil", errUtils.ErrInvalidAuthConfig)
	}
	if atmosConfig == nil {
		errUtils.CheckErrorAndPrint(fmt.Errorf("%w: atmos configuration is nil", errUtils.ErrInvalidAuthConfig), "TerraformPreHook", "")
		return fmt.Errorf("%w: atmos configuration is nil", errUtils.ErrInvalidAuthConfig)
	}

	atmosLevel, authLevel := getConfigLogLevels(atmosConfig)
	log.SetLevel(authLevel)
	defer log.SetLevel(atmosLevel)
	log.SetPrefix("atmos-auth")
	defer log.SetPrefix("")

	// TODO: verify if we need to use Decode, or if we can use the merged auth config directly.
	// Use the merged auth configuration from stackInfo.
	// ComponentAuthSection already contains the deep-merged auth config from component + inherits + atmos.yaml.
	// Converted to typed struct when needed.
	var authConfig schema.AuthConfig
	err := mapstructure.Decode(stackInfo.ComponentAuthSection, &authConfig)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrInvalidAuthConfig, "TerraformPreHook", "failed to decode component auth config - check atmos.yaml or component auth section")
		return errUtils.ErrInvalidAuthConfig
	}

	// Skip if no auth config (check the merged config, not the original).
	if len(authConfig.Providers) == 0 && len(authConfig.Identities) == 0 {
		log.Debug("No auth configuration found, skipping authentication")
		return nil
	}

	authManager, err := newAuthManager(&authConfig, stackInfo)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrAuthManager, "TerraformPreHook", "failed to create auth manager")
		return errUtils.ErrAuthManager
	}

	// Determine target identity: stack info identity (CLI flag) or default identity.
	ctx := context.Background()
	var targetIdentityName string

	if stackInfo.Identity != "" {
		// This is set by the CLI Flag.
		targetIdentityName = stackInfo.Identity
	} else {
		targetIdentityName, err = authManager.GetDefaultIdentity()
		if err != nil {
			errUtils.CheckErrorAndPrint(errUtils.ErrDefaultIdentity, "TerraformPreHook", "failed to get default identity")
			return errUtils.ErrDefaultIdentity
		}
	}
	if targetIdentityName == "" {
		errUtils.CheckErrorAndPrint(ErrNoDefaultIdentity, "TerraformPreHook", "Use the identity flag or specify an identity as default.")
		return ErrNoDefaultIdentity
	}

	log.Info("Authenticating with identity", "identity", targetIdentityName)

	// Authenticate with target identity.
	whoami, err := authManager.Authenticate(ctx, targetIdentityName)
	if err != nil {
		errUtils.CheckErrorAndPrint(errUtils.ErrAuthenticationFailed, "TerraformPreHook", "failed to authenticate with identity")
		return errUtils.ErrAuthenticationFailed
	}

	log.Debug("Authentication successful", "identity", whoami.Identity, "expiration", whoami.Expiration)

	err = utils.PrintAsYAMLToFileDescriptor(atmosConfig, stackInfo.ComponentEnvSection)
	errUtils.CheckErrorAndPrint(err, "TerraformPreHook", "failed to print component env section")

	return nil
}

func validateAuthConfig(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) bool {
	if atmosConfig == nil {
		return false
	}
	if stackInfo == nil {
		return false
	}
	if len(atmosConfig.Auth.Identities) == 0 && len(atmosConfig.Auth.Providers) == 0 {
		return false
	}
	return true
}

func newAuthManager(authConfig *schema.AuthConfig, stackInfo *schema.ConfigAndStacksInfo) (types.AuthManager, error) {
	// Create auth manager components.
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	// Create auth manager with merged configuration and stack info (so identities can mutate it).
	authManager, err := NewAuthManager(
		authConfig,
		credStore,
		validator,
		stackInfo,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create auth manager: %v", errUtils.ErrAuthManager, err)
	}
	return authManager, nil
}

func getConfigLogLevels(atmosConfig *schema.AtmosConfiguration) (log.Level, log.Level) {
	if atmosConfig == nil {
		return log.InfoLevel, log.InfoLevel
	}
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
