package hooks

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/internal/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TerraformPreHook performs authentication before Terraform commands
func TerraformPreHook(ctx context.Context, authManager auth.AuthManager, componentConfig *schema.ComponentAuthConfig) error {
	// Check if authentication is needed
	if authManager == nil {
		return nil // No auth manager configured
	}

	// Try to get current session
	whoami, err := authManager.Whoami(ctx)
	if err == nil && whoami != nil {
		// Check if credentials are still valid (at least 5 minutes remaining)
		if whoami.Expiration != nil && whoami.Expiration.After(time.Now().Add(5*time.Minute)) {
			// Set environment variables for existing session
			if whoami.Environment != nil {
				for key, value := range whoami.Environment {
					if err := setEnvVar(key, value); err != nil {
						return fmt.Errorf("failed to set environment variable %s: %w", key, err)
					}
				}
			}
			return nil // Session is still valid
		}
	}

	// Need to authenticate - get default identity
	defaultIdentity, err := authManager.GetDefaultIdentity()
	if err != nil {
		return fmt.Errorf("no default identity configured for automatic authentication: %w", err)
	}

	// Perform authentication
	whoami, err = authManager.Authenticate(ctx, defaultIdentity)
	if err != nil {
		return fmt.Errorf("failed to authenticate with default identity %q: %w", defaultIdentity, err)
	}

	// Set environment variables
	if whoami.Environment != nil {
		for key, value := range whoami.Environment {
			if err := setEnvVar(key, value); err != nil {
				return fmt.Errorf("failed to set environment variable %s: %w", key, err)
			}
		}
	}

	return nil
}

// setEnvVar sets an environment variable (placeholder - actual implementation would use os.Setenv)
func setEnvVar(key, value string) error {
	// In a real implementation, this would set the environment variable
	// for the current process and potentially for child processes
	fmt.Printf("Setting %s=%s\n", key, value)
	return nil
}
