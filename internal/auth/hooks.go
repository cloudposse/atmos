package auth

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/auth/cloud"
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
	AWSFileManager  = types.AWSFileManager
	Validator       = types.Validator
)

// TerraformPreHook runs before Terraform commands to set up authentication
func TerraformPreHook(atmosConfig schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	// Set up authentication logging
	log.SetPrefix("[atmos-auth]")
	defer log.SetPrefix("")

	atmosLogLevel, _ := log.ParseLevel(atmosConfig.Logs.Level)
	authLogLevel, _ := log.ParseLevel(atmosConfig.Auth.Logs.Level)
	log.SetLevel(authLogLevel)
	defer log.SetLevel(atmosLogLevel)

	// Use the merged auth configuration from stackInfo
	// ComponentAuthSection already contains the deep-merged auth config from component + inherits + atmos.yaml
	// Converted to typed struct when needed
	var authConfig schema.AuthConfig
	err := mapstructure.Decode(stackInfo.ComponentAuthSection, &authConfig)
	if err != nil {
		return fmt.Errorf("failed to decode component auth config: %w", err)
	}

	// Skip if no auth config (check the merged config, not the original)
	if len(authConfig.Providers) == 0 && len(authConfig.Identities) == 0 {
		log.Debug("No auth configuration found, skipping authentication")
		return nil
	}

	// Create auth manager components
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	// Create cloud provider manager
	cloudProviderManager := cloud.NewCloudProviderManager()

	// Create auth manager with merged configuration
	authManager, err := NewAuthManager(
		&authConfig,
		credStore,
		validator,
		cloudProviderManager,
	)
	if err != nil {
		return fmt.Errorf("failed to create auth manager: %w", err)
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
			return fmt.Errorf("failed to get default identity: %w", err)
		}
	}
	if targetIdentityName == "" {
		return fmt.Errorf("no default identity configured for authentication")
	}

	log.Info("Authenticating with identity", "identity", targetIdentityName)

	// Authenticate with target identity
	whoami, err := authManager.Authenticate(ctx, targetIdentityName)
	if err != nil {
		return fmt.Errorf("failed to authenticate with identity %q: %w", targetIdentityName, err)
	}

	log.Debug("Authentication successful", "identity", whoami.Identity, "expiration", whoami.Expiration)

	utils.PrintAsYAMLToFileDescriptor(&atmosConfig, stackInfo.ComponentEnvSection)
	return nil
}
