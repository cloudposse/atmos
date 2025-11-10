package devcontainer

//go:generate go run go.uber.org/mock/mockgen@latest -source=identity_interface.go -destination=mock_identity_test.go -package=devcontainer

import (
	"context"
)

// IdentityManager handles identity-related operations for devcontainers.
type IdentityManager interface {
	// InjectIdentityEnvironment injects identity environment variables into config.
	InjectIdentityEnvironment(ctx context.Context, config *Config, identityName string) error
}

// identityManagerImpl implements IdentityManager using existing functions.
type identityManagerImpl struct{}

// NewIdentityManager creates a new IdentityManager.
func NewIdentityManager() IdentityManager {
	return &identityManagerImpl{}
}

// InjectIdentityEnvironment injects identity environment variables into the config.
func (i *identityManagerImpl) InjectIdentityEnvironment(ctx context.Context, config *Config, identityName string) error {
	return injectIdentityEnvironment(ctx, config, identityName)
}
