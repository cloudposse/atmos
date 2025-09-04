package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
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

	// Store original log level and set auth log level if configured
	originalLevel := log.GetLevel()
	if atmosConfig.Auth.Logs != nil && atmosConfig.Auth.Logs.Level != "" {
		if authLevel, err := log.ParseLevel(atmosConfig.Auth.Logs.Level); err == nil {
			log.SetLevel(authLevel)
			defer log.SetLevel(originalLevel)
		}
	}

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

	// Create auth manager with merged config
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

	// Try to get current session
	ctx := context.Background()
	whoami, err := authManager.Whoami(ctx)
	log.Debug("Current session check", "whoami", whoami, "error", err)
	
	var needsAuthentication = true
	var defaultIdentityName string
	
	if err == nil && whoami != nil {
		// Check if credentials are still valid (at least 5 minutes remaining)
		if whoami.Expiration != nil && whoami.Expiration.After(time.Now().Add(5*time.Minute)) {
			log.Debug("Using existing valid session", "identity", whoami.Identity, "expiration", whoami.Expiration, "environment", whoami.Environment)
			needsAuthentication = false
			defaultIdentityName = whoami.Identity
		}
	}

	if needsAuthentication {
		// Need to authenticate - find default identity
		defaultIdentityName, err = authManager.GetDefaultIdentity()
		if err != nil {
			return fmt.Errorf("failed to get default identity: %w", err)
		}
		if defaultIdentityName == "" {
			return fmt.Errorf("no default identity configured for authentication")
		}

		log.Info("Authenticating with default identity", "identity", defaultIdentityName)
		
		// Authenticate with default identity
		_, err = authManager.Authenticate(ctx, defaultIdentityName)
		if err != nil {
			return fmt.Errorf("failed to authenticate with default identity: %w", err)
		}

		// Get updated session info
		whoami, err = authManager.Whoami(ctx)
		if err != nil {
			return fmt.Errorf("failed to get session info after authentication: %w", err)
		}
	}

	// Always set up environment variables and AWS files (whether from cache or fresh auth)
	// Get identity environment variables and merge into component environment section
	if identity, exists := atmosConfig.Auth.Identities[defaultIdentityName]; exists {
		if len(identity.Env) > 0 {
			environment.MergeIdentityEnvOverrides(stackInfo, identity.Env)
		}
	}

	log.Debug("Authentication successful", "identity", whoami.Identity, "expiration", whoami.Expiration)

	utils.PrintAsYAMLToFileDescriptor(&atmosConfig, stackInfo.ComponentEnvSection)
	utils.PrintAsYAMLToFileDescriptor(&atmosConfig, stackInfo.ComponentEnvList)

	// Find root provider for chained identities to determine if AWS files are needed
	rootProviderName := findRootProvider(&atmosConfig.Auth, defaultIdentityName)
	
	// Add AWS file environment variables to component environment if this is an AWS provider
	if rootProviderName != "" {
		if provider, exists := atmosConfig.Auth.Providers[rootProviderName]; exists {
			// Check if this is an AWS provider
			if provider.Kind == "aws/iam-identity-center" || provider.Kind == "aws/assume-role" || provider.Kind == "aws/user" {
				awsFileManager := environment.NewAWSFileManager()
				// Use provider name for AWS file paths (files are organized by provider)
				awsEnvVars := awsFileManager.GetEnvironmentVariables(rootProviderName)
				environment.MergeIdentityEnvOverrides(stackInfo, awsEnvVars)
			}
		}
	}

	log.Debug("Auth hook completed successfully")
	return nil
}

// findRootProvider traverses the identity chain to find the root provider
func findRootProvider(authConfig *schema.AuthConfig, identityName string) string {
	visited := make(map[string]bool)
	
	for identityName != "" {
		// Prevent infinite loops
		if visited[identityName] {
			return ""
		}
		visited[identityName] = true
		
		identity, exists := authConfig.Identities[identityName]
		if !exists {
			return ""
		}
		
		if identity.Via == nil {
			return ""
		}
		
		// If this identity points to a provider, we found the root
		if identity.Via.Provider != "" {
			return identity.Via.Provider
		}
		
		// If this identity points to another identity, continue traversing
		if identity.Via.Identity != "" {
			identityName = identity.Via.Identity
			continue
		}
		
		// No provider or identity found
		return ""
	}
	
	return ""
}
