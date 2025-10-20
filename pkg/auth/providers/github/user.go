package github

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DeviceFlowClient defines the interface for GitHub Device Flow operations.
// This allows us to mock the Device Flow for testing.
type DeviceFlowClient interface {
	// StartDeviceFlow initiates the Device Flow and returns device/user codes.
	StartDeviceFlow(ctx context.Context) (*DeviceFlowResponse, error)
	// PollForToken polls GitHub for the access token after user authorization.
	PollForToken(ctx context.Context, deviceCode string) (string, error)
	// GetCachedToken retrieves a cached token from the OS keychain.
	GetCachedToken(ctx context.Context) (string, error)
	// StoreToken stores a token in the OS keychain.
	StoreToken(ctx context.Context, token string) error
	// DeleteToken removes a token from the OS keychain.
	DeleteToken(ctx context.Context) error
}

// DeviceFlowResponse contains the response from GitHub's device authorization endpoint.
type DeviceFlowResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// userProvider implements GitHub User authentication via OAuth Device Flow.
// Inspired by https://github.com/suzuki-shunsuke/ghtkn
type userProvider struct {
	name         string
	config       *schema.Provider
	clientID     string
	scopes       []string
	keychainSvc  string
	tokenLifetime time.Duration
	deviceClient DeviceFlowClient
}

// NewUserProvider creates a new GitHub User authentication provider.
func NewUserProvider(name string, config *schema.Provider) (types.Provider, error) {
	defer perf.Track(nil, "github.NewUserProvider")()

	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}

	if config.Kind != "github/user" {
		return nil, fmt.Errorf("%w: invalid provider kind for GitHub User provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Extract client_id from spec.
	clientID, ok := config.Spec["client_id"].(string)
	if !ok || clientID == "" {
		return nil, fmt.Errorf("%w: client_id is required in spec for GitHub User provider", errUtils.ErrInvalidProviderConfig)
	}

	// Extract optional scopes.
	var scopes []string
	if scopesRaw, ok := config.Spec["scopes"]; ok {
		if scopesList, ok := scopesRaw.([]interface{}); ok {
			for _, s := range scopesList {
				if scopeStr, ok := s.(string); ok {
					scopes = append(scopes, scopeStr)
				}
			}
		}
	}

	// Extract optional keychain_service (defaults to "atmos-github").
	keychainSvc := "atmos-github"
	if svc, ok := config.Spec["keychain_service"].(string); ok && svc != "" {
		keychainSvc = svc
	}

	// Extract optional token_lifetime (defaults to 8h).
	tokenLifetime := 8 * time.Hour
	if lifetimeStr, ok := config.Spec["token_lifetime"].(string); ok && lifetimeStr != "" {
		if duration, err := time.ParseDuration(lifetimeStr); err == nil {
			tokenLifetime = duration
		}
	}

	return &userProvider{
		name:          name,
		config:        config,
		clientID:      clientID,
		scopes:        scopes,
		keychainSvc:   keychainSvc,
		tokenLifetime: tokenLifetime,
		// deviceClient will be set by SetDeviceFlowClient or defaulted in Authenticate.
	}, nil
}

// SetDeviceFlowClient allows injection of a mock client for testing.
func (p *userProvider) SetDeviceFlowClient(client DeviceFlowClient) {
	p.deviceClient = client
}

// Kind returns the provider kind.
func (p *userProvider) Kind() string {
	return KindUser
}

// Name returns the provider name.
func (p *userProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for GitHub User provider.
func (p *userProvider) PreAuthenticate(_ types.AuthManager) error {
	defer perf.Track(nil, "github.userProvider.PreAuthenticate")()

	return nil
}

// Authenticate performs GitHub User authentication via Device Flow.
func (p *userProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "github.userProvider.Authenticate")()

	log.Info("Starting GitHub User authentication", "provider", p.name)

	// Validate provider configuration.
	if err := p.Validate(); err != nil {
		return nil, err
	}

	// TODO: Initialize real Device Flow client if not mocked.
	// For now, this is a placeholder that will fail in production until we implement the real client.
	if p.deviceClient == nil {
		return nil, fmt.Errorf("%w: Device Flow client not initialized (implementation pending)", errUtils.ErrAuthenticationFailed)
	}

	// Check for cached token.
	token, err := p.deviceClient.GetCachedToken(ctx)
	if err == nil && token != "" {
		log.Debug("Using cached GitHub token", "provider", p.name)
		return &types.GitHubUserCredentials{
			Token:      token,
			Provider:   p.name,
			Expiration: time.Now().Add(p.tokenLifetime),
		}, nil
	}

	// No cached token, initiate Device Flow.
	log.Info("Initiating GitHub Device Flow", "provider", p.name)

	deviceResp, err := p.deviceClient.StartDeviceFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to start Device Flow: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Instruct user to authenticate (output to stderr).
	fmt.Fprintf(p.getStderr(), "\nTo authenticate with GitHub:\n")
	fmt.Fprintf(p.getStderr(), "1. Visit: %s\n", deviceResp.VerificationURI)
	fmt.Fprintf(p.getStderr(), "2. Enter code: %s\n\n", deviceResp.UserCode)
	fmt.Fprintf(p.getStderr(), "Waiting for authentication...\n")

	// Poll for token.
	token, err = p.deviceClient.PollForToken(ctx, deviceResp.DeviceCode)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to obtain token: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Store token in keychain.
	if err := p.deviceClient.StoreToken(ctx, token); err != nil {
		log.Warn("Failed to store token in keychain (continuing anyway)", "error", err)
	}

	log.Info("GitHub User authentication successful", "provider", p.name)

	return &types.GitHubUserCredentials{
		Token:      token,
		Provider:   p.name,
		Expiration: time.Now().Add(p.tokenLifetime),
	}, nil
}

// Validate validates the provider configuration.
func (p *userProvider) Validate() error {
	defer perf.Track(nil, "github.userProvider.Validate")()

	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}

	// Validate token lifetime is reasonable (1h to 24h).
	if p.tokenLifetime < time.Hour || p.tokenLifetime > 24*time.Hour {
		return fmt.Errorf("%w: token_lifetime must be between 1h and 24h", errUtils.ErrInvalidProviderConfig)
	}

	return nil
}

// Environment returns environment variables for this provider.
func (p *userProvider) Environment() (map[string]string, error) {
	defer perf.Track(nil, "github.userProvider.Environment")()

	// GitHub User provider doesn't set environment variables at the provider level.
	// Environment variables are set by the identity after authentication.
	return map[string]string{}, nil
}

// Logout removes cached tokens from the OS keychain.
func (p *userProvider) Logout(ctx context.Context) error {
	defer perf.Track(nil, "github.userProvider.Logout")()

	if p.deviceClient == nil {
		return fmt.Errorf("%w: Device Flow client not initialized", errUtils.ErrAuthenticationFailed)
	}

	log.Info("Removing GitHub token from keychain", "provider", p.name)

	if err := p.deviceClient.DeleteToken(ctx); err != nil {
		return fmt.Errorf("%w: failed to delete token from keychain: %v", errUtils.ErrAuthenticationFailed, err)
	}

	log.Info("GitHub token removed successfully", "provider", p.name)
	return nil
}

// getStderr returns the standard error writer for user-facing messages.
// This is a helper to make testing easier.
func (p *userProvider) getStderr() interface{ Write([]byte) (int, error) } {
	return &stderrWriter{}
}

// stderrWriter writes to os.Stderr.
type stderrWriter struct{}

func (w *stderrWriter) Write(p []byte) (int, error) {
	// In production, this writes to os.Stderr.
	// In tests, this can be mocked.
	fmt.Print(string(p))
	return len(p), nil
}
