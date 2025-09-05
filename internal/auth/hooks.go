package auth

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/auth/cloud"
	"github.com/cloudposse/atmos/internal/auth/config"
	"github.com/cloudposse/atmos/internal/auth/credentials"
	"github.com/cloudposse/atmos/internal/auth/environment"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/internal/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

type Provider = types.Provider
type Identity = types.Identity
type AuthManager = types.AuthManager
type CredentialStore = types.CredentialStore
type AWSFileManager = types.AWSFileManager
type ConfigMerger = types.ConfigMerger
type Validator = types.Validator

// TerraformPreHook runs before Terraform commands to set up authentication
func TerraformPreHook(atmosConfig schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
	// Set up authentication logging
	log.SetPrefix("[atmos-auth]")
	defer log.SetPrefix("")

	atmosLogLevel, _ := log.ParseLevel(atmosConfig.Logs.Level)
	authLogLevel, _ := log.ParseLevel(atmosConfig.Auth.Logs.Level)
	log.SetLevel(authLogLevel)
	defer log.SetLevel(atmosLogLevel)

	// Skip if no auth config
	if len(atmosConfig.Auth.Providers) == 0 && len(atmosConfig.Auth.Identities) == 0 {
		log.Debug("No auth configuration found, skipping authentication")
		return nil
	}

	// Create auth manager components
	credStore := credentials.NewCredentialStore()
	awsFileManager := environment.NewAWSFileManager()
	configMerger := config.NewConfigMerger()
	validator := validation.NewValidator()
	
	// Create cloud provider manager
	cloudProviderManager := cloud.NewCloudProviderManager()

	// Merge component auth config with global auth config
	mergedAuthConfig := &atmosConfig.Auth
	if stackInfo != nil && stackInfo.ComponentIdentitiesSection != nil {
		log.Debug("Merging component auth configuration")
		// Convert ComponentIdentitiesSection to ComponentAuthConfig
		componentConfig := &schema.ComponentAuthConfig{
			Identities: make(map[string]schema.Identity),
		}

		// Parse component identities
		for name, identityData := range stackInfo.ComponentIdentitiesSection {
			if identityMap, ok := identityData.(map[string]interface{}); ok {
				// Start with global identity if it exists
				identity := schema.Identity{}
				if globalIdentity, exists := atmosConfig.Auth.Identities[name]; exists {
					identity = globalIdentity
				}

				// Apply component overrides
				if defaultVal, exists := identityMap["default"]; exists {
					if defaultBool, ok := defaultVal.(bool); ok {
						identity.Default = defaultBool
					}
				}

				componentConfig.Identities[name] = identity
			}
		}

		// Merge configurations
		var err error
		mergedAuthConfig, err = configMerger.MergeAuthConfig(&atmosConfig.Auth, componentConfig)
		if err != nil {
			return fmt.Errorf("failed to merge component auth config: %w", err)
		}
	}

	// Create auth manager with merged configuration
	authManager, err := NewAuthManager(
		mergedAuthConfig,
		credStore,
		awsFileManager,
		configMerger,
		validator,
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

	// Always set up environment variables and AWS files
	// Get identity environment variables and merge into component environment section
	if identity, exists := mergedAuthConfig.Identities[targetIdentityName]; exists {
		if len(identity.Env) > 0 {
			environment.MergeIdentityEnvOverrides(stackInfo, identity.Env)
		}
	}

	log.Debug("Authentication successful", "identity", whoami.Identity, "expiration", whoami.Expiration)

	// Get root provider name - use "aws-user" for AWS user identities, otherwise get from identity chain
	rootProviderName := ""
	if identity, exists := mergedAuthConfig.Identities[targetIdentityName]; exists {
		if identity.Kind == "aws/user" && identity.Via == nil {
			rootProviderName = "aws-user"
		} else {
			// Use AuthManager's method to recursively resolve provider through identity chains
			rootProviderName = authManager.GetProviderForIdentity(targetIdentityName)
		}
	}

	// Setup cloud provider environment if provider is configured
	if rootProviderName != "" {
		var providerKind string
		
		// Handle AWS User identities (standalone, no provider config)
		if rootProviderName == "aws-user" {
			providerKind = "aws/user"
		} else if provider, exists := mergedAuthConfig.Providers[rootProviderName]; exists {
			providerKind = provider.Kind
		}
		
		// Setup cloud provider environment if we have a provider kind
		if providerKind != "" {
			// Setup cloud provider environment (files, credentials, etc.)
			if err := cloudProviderManager.SetupEnvironment(ctx, providerKind, rootProviderName, targetIdentityName, whoami.Credentials); err != nil {
				return fmt.Errorf("failed to setup cloud provider environment: %w", err)
			}
			
			// Get cloud provider environment variables
			cloudEnvVars, err := cloudProviderManager.GetEnvironmentVariables(providerKind, rootProviderName, targetIdentityName)
			if err != nil {
				return fmt.Errorf("failed to get cloud provider environment variables: %w", err)
			}
			
			// Convert map[string]string to []schema.EnvironmentVariable
			var envVars []schema.EnvironmentVariable
			for key, value := range cloudEnvVars {
				envVars = append(envVars, schema.EnvironmentVariable{
					Key:   key,
					Value: value,
				})
			}
			
			// Merge cloud provider environment variables
			environment.MergeIdentityEnvOverrides(stackInfo, envVars)
		}
	}

	log.Debug("Auth hook completed successfully")

	utils.PrintAsYAMLToFileDescriptor(&atmosConfig, stackInfo.ComponentEnvSection)
	return nil
}

