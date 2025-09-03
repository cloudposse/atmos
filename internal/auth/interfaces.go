package auth

// Re-export types from the types package to maintain backward compatibility
// This allows existing code to continue using auth.Provider, auth.Identity, etc.

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloudposse/atmos/internal/auth/config"
	"github.com/cloudposse/atmos/internal/auth/credentials"
	"github.com/cloudposse/atmos/internal/auth/environment"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/internal/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
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
	// Skip if no auth config
	if len(atmosConfig.Auth.Providers) == 0 && len(atmosConfig.Auth.Identities) == 0 {
		return nil
	}

	// Create auth manager components
	credStore := credentials.NewCredentialStore()
	awsFileManager := environment.NewAWSFileManager()
	configMerger := config.NewConfigMerger()
	validator := validation.NewValidator()

	// Create auth manager
	authManager, err := NewAuthManager(
		&atmosConfig.Auth,
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
	if err == nil && whoami != nil {
		// Check if credentials are still valid (at least 5 minutes remaining)
		if whoami.Expiration != nil && whoami.Expiration.After(time.Now().Add(5*time.Minute)) {
			// Set environment variables for existing session
			if whoami.Environment != nil {
				for key, value := range whoami.Environment {
					if err := os.Setenv(key, value); err != nil {
						return fmt.Errorf("failed to set environment variable %s: %w", key, err)
					}
				}
			}
			return nil // Already authenticated
		}
	}

	// Need to authenticate - find default identity
	defaultIdentityName, err := authManager.GetDefaultIdentity()
	if err != nil {
		return fmt.Errorf("failed to get default identity: %w", err)
	}
	if defaultIdentityName == "" {
		return fmt.Errorf("no default identity configured for authentication")
	}

	// Authenticate with default identity
	_, err = authManager.Authenticate(ctx, defaultIdentityName)
	if err != nil {
		return fmt.Errorf("failed to authenticate with default identity: %w", err)
	}

	// Get updated session info and set environment variables
	whoami, err = authManager.Whoami(ctx)
	if err != nil {
		return fmt.Errorf("failed to get session info after authentication: %w", err)
	}

	// Set process environment variables
	if whoami.Environment != nil {
		for key, value := range whoami.Environment {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("failed to set environment variable %s: %w", key, err)
			}
		}
	}

	// Get identity environment variables and merge into component environment section
	if identity, exists := atmosConfig.Auth.Identities[defaultIdentityName]; exists {
		if len(identity.Environment) > 0 {
			environment.MergeIdentityEnvOverrides(stackInfo, identity.Environment)
		}
	}

	// Get provider name for AWS file environment variables
	providerName := ""
	if identity, exists := atmosConfig.Auth.Identities[defaultIdentityName]; exists {
		if identity.Via != nil && identity.Via.Provider != "" {
			providerName = identity.Via.Provider
		}
	}

	// Add AWS file environment variables to component environment
	if providerName != "" {
		if provider, exists := atmosConfig.Auth.Providers[providerName]; exists {
			// Check if this is an AWS provider
			if provider.Kind == "aws/iam-identity-center" || provider.Kind == "aws/assume-role" || provider.Kind == "aws/user" {
				awsFileManager := environment.NewAWSFileManager()
				awsEnvVars := awsFileManager.GetEnvironmentVariables(providerName)
				environment.MergeIdentityEnvOverrides(stackInfo, awsEnvVars)
			}
		}
	}

	return nil
}
