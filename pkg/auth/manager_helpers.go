package auth

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

// CreateAndAuthenticateManager creates and authenticates an AuthManager from an identity name.
// Returns nil AuthManager if identityName is empty (no authentication requested).
// Returns error if identityName is provided but auth is not configured.
//
// This helper is used by both CLI commands and internal execution logic to ensure
// consistent authentication behavior across the codebase.
//
// Parameters:
//   - identityName: The identity to authenticate (can be "__SELECT__" for interactive selection)
//   - authConfig: The auth configuration from atmos.yaml
//   - selectValue: The special value that triggers interactive identity selection (typically "__SELECT__")
//
// Returns:
//   - AuthManager with populated AuthContext after successful authentication
//   - nil if identityName is empty
//   - error if authentication fails or auth is not configured
func CreateAndAuthenticateManager(
	identityName string,
	authConfig *schema.AuthConfig,
	selectValue string,
) (AuthManager, error) {
	if identityName == "" {
		return nil, nil
	}

	// Check if auth is configured when identity is provided.
	if authConfig == nil || len(authConfig.Identities) == 0 {
		return nil, fmt.Errorf("%w: authentication requires at least one identity configured in atmos.yaml", errUtils.ErrAuthNotConfigured)
	}

	// Create a ConfigAndStacksInfo for the auth manager to populate with AuthContext.
	// This enables YAML template functions to access authenticated credentials.
	authStackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	authManager, err := NewAuthManager(authConfig, credStore, validator, authStackInfo)
	if err != nil {
		return nil, errors.Join(errUtils.ErrFailedToInitializeAuthManager, err)
	}

	// Handle interactive selection if identity matches the select value.
	forceSelect := identityName == selectValue
	if forceSelect {
		identityName, err = authManager.GetDefaultIdentity(forceSelect)
		if err != nil {
			return nil, err
		}
	}

	// Authenticate to populate AuthContext with credentials.
	// This is critical for YAML functions like !terraform.state and !terraform.output
	// to access cloud resources with the proper credentials.
	_, err = authManager.Authenticate(context.Background(), identityName)
	if err != nil {
		return nil, err
	}

	return authManager, nil
}
