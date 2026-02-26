package okta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	oktaCloud "github.com/cloudposse/atmos/pkg/auth/cloud/okta"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Default scopes for Okta device code authentication.
	defaultScopes = "openid profile offline_access"

	// Default authorization server.
	defaultAuthorizationServer = "default"

	// Default timeout for device code authentication.
	deviceCodeTimeout = 15 * time.Minute

	// Default polling interval.
	defaultPollingInterval = 5 * time.Second

	// HTTP client timeout.
	httpClientTimeout = 30 * time.Second
)

// isInteractive checks if we're running in an interactive terminal.
// For device code flow, we need stderr to be a TTY so the user can see the authentication URL.
func isInteractive() bool {
	return term.IsTTYSupportForStderr()
}

// truncateDeviceCode safely truncates a device code for logging.
func truncateDeviceCode(code string) string {
	if len(code) <= 8 {
		return code + "..."
	}
	return code[:8] + "..."
}

// deviceCodeProvider implements Okta device code authentication.
type deviceCodeProvider struct {
	name                string
	config              *schema.Provider
	orgURL              string
	clientID            string
	scopes              []string
	authorizationServer string
	basePath            string // Custom base path for file storage.
	fileManager         *oktaCloud.OktaFileManager
	httpClient          *http.Client
	realm               string // Credential isolation realm set by auth manager.
}

// deviceCodeConfig holds extracted Okta configuration from provider spec.
type deviceCodeConfig struct {
	OrgURL              string
	ClientID            string
	Scopes              []string
	AuthorizationServer string
	BasePath            string
}

// extractDeviceCodeConfig extracts Okta config from provider spec.
func extractDeviceCodeConfig(spec map[string]any) deviceCodeConfig {
	config := deviceCodeConfig{
		AuthorizationServer: defaultAuthorizationServer,
	}

	if spec == nil {
		return config
	}

	if orgURL, ok := spec["org_url"].(string); ok {
		config.OrgURL = orgURL
	}
	if clientID, ok := spec["client_id"].(string); ok {
		config.ClientID = clientID
	}
	if authServer, ok := spec["authorization_server"].(string); ok && authServer != "" {
		config.AuthorizationServer = authServer
	}

	// Parse scopes - can be string (space-separated) or array.
	if scopesStr, ok := spec["scopes"].(string); ok {
		config.Scopes = strings.Fields(scopesStr)
	} else if scopesArr, ok := spec["scopes"].([]any); ok {
		for _, s := range scopesArr {
			if str, ok := s.(string); ok {
				config.Scopes = append(config.Scopes, str)
			}
		}
	}

	// Parse files.base_path.
	if files, ok := spec["files"].(map[string]any); ok {
		if basePath, ok := files["base_path"].(string); ok {
			config.BasePath = basePath
		}
	}

	return config
}

// NewDeviceCodeProvider creates a new Okta device code provider.
func NewDeviceCodeProvider(name string, config *schema.Provider) (*deviceCodeProvider, error) {
	defer perf.Track(nil, "okta.NewDeviceCodeProvider")()

	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if config.Kind != "okta/device-code" {
		return nil, fmt.Errorf("%w: invalid provider kind for Okta device code provider: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}

	// Extract Okta-specific config from Spec.
	cfg := extractDeviceCodeConfig(config.Spec)

	// Org URL is required.
	if cfg.OrgURL == "" {
		return nil, fmt.Errorf("%w: org_url is required in spec for Okta device code provider", errUtils.ErrInvalidProviderConfig)
	}

	// Client ID is required.
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("%w: client_id is required in spec for Okta device code provider", errUtils.ErrInvalidProviderConfig)
	}

	// Default scopes if not specified.
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = strings.Fields(defaultScopes)
	}

	// Create HTTP client.
	httpClient := &http.Client{
		Timeout: httpClientTimeout,
	}

	return &deviceCodeProvider{
		name:                name,
		config:              config,
		orgURL:              cfg.OrgURL,
		clientID:            cfg.ClientID,
		scopes:              scopes,
		authorizationServer: cfg.AuthorizationServer,
		basePath:            cfg.BasePath,
		httpClient:          httpClient,
	}, nil
}

// Kind returns the provider kind.
func (p *deviceCodeProvider) Kind() string {
	return "okta/device-code"
}

// Name returns the configured provider name.
func (p *deviceCodeProvider) Name() string {
	return p.name
}

// SetRealm sets the credential isolation realm for this provider.
func (p *deviceCodeProvider) SetRealm(realm string) {
	p.realm = realm
	// Reinitialize file manager with new realm.
	p.fileManager = nil
}

// PreAuthenticate is a no-op for device code provider.
func (p *deviceCodeProvider) PreAuthenticate(_ authTypes.AuthManager) error {
	return nil
}

// getFileManager returns the file manager, creating it if needed.
func (p *deviceCodeProvider) getFileManager() (*oktaCloud.OktaFileManager, error) {
	if p.fileManager != nil {
		return p.fileManager, nil
	}

	fileManager, err := oktaCloud.NewOktaFileManager(p.basePath, p.realm)
	if err != nil {
		return nil, err
	}
	p.fileManager = fileManager
	return fileManager, nil
}

// getDeviceAuthorizationEndpoint returns the Okta device authorization endpoint URL.
func (p *deviceCodeProvider) getDeviceAuthorizationEndpoint() string {
	return fmt.Sprintf("%s/oauth2/%s/v1/device/authorize", p.orgURL, p.authorizationServer)
}

// getTokenEndpoint returns the Okta token endpoint URL.
func (p *deviceCodeProvider) getTokenEndpoint() string {
	return fmt.Sprintf("%s/oauth2/%s/v1/token", p.orgURL, p.authorizationServer)
}

// startDeviceAuthorization initiates the device authorization flow with Okta.
func (p *deviceCodeProvider) startDeviceAuthorization(ctx context.Context) (*oktaCloud.DeviceAuthorizationResponse, error) {
	defer perf.Track(nil, "okta.startDeviceAuthorization")()

	// Build request body.
	data := url.Values{}
	data.Set("client_id", p.clientID)
	data.Set("scope", strings.Join(p.scopes, " "))

	// Create request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.getDeviceAuthorizationEndpoint(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create device authorization request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Send request.
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to send device authorization request: %w", errUtils.ErrAuthenticationFailed, err)
	}
	defer resp.Body.Close()

	// Read response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read device authorization response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Check for errors.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: device authorization failed with status %d: %s", errUtils.ErrAuthenticationFailed, resp.StatusCode, string(body))
	}

	// Parse response.
	var deviceAuth oktaCloud.DeviceAuthorizationResponse
	if err := json.Unmarshal(body, &deviceAuth); err != nil {
		return nil, fmt.Errorf("%w: failed to parse device authorization response: %w", errUtils.ErrAuthenticationFailed, err)
	}

	log.Debug("Received device authorization response",
		"device_code", truncateDeviceCode(deviceAuth.DeviceCode),
		"user_code", deviceAuth.UserCode,
		"verification_uri", deviceAuth.VerificationURI,
		"expires_in", deviceAuth.ExpiresIn,
		"interval", deviceAuth.Interval,
	)

	return &deviceAuth, nil
}

// Authenticate performs Okta device code authentication.
func (p *deviceCodeProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "okta.deviceCodeProvider.Authenticate")()

	// Try cached tokens first.
	cachedTokens, err := p.tryCachedTokens(ctx)
	if err != nil {
		log.Debug("Error checking cached tokens", "error", err)
	}
	if cachedTokens != nil {
		return p.tokensToCredentials(cachedTokens)
	}

	// Check if we're in a headless environment.
	if !isInteractive() {
		return nil, fmt.Errorf("%w: Okta device code flow requires an interactive terminal (no TTY detected). Use API token authentication in headless environments", errUtils.ErrAuthenticationFailed)
	}

	log.Debug("Starting Okta device code authentication",
		"provider", p.name,
		"org_url", p.orgURL,
		"client_id", p.clientID,
	)

	// Start device authorization.
	deviceAuth, err := p.startDeviceAuthorization(ctx)
	if err != nil {
		return nil, err
	}

	// Display device code to user.
	displayDeviceCodePrompt(deviceAuth.UserCode, deviceAuth.VerificationURI)

	// Create a context with timeout.
	authCtx, cancel := context.WithTimeout(ctx, deviceCodeTimeout)
	defer cancel()

	// Poll for token with spinner UI (we already verified isInteractive() above).
	tokens, err := pollForTokenWithSpinner(authCtx, p, deviceAuth)
	if err != nil {
		return nil, err
	}

	// Save tokens to cache.
	fileManager, err := p.getFileManager()
	if err != nil {
		log.Debug("Failed to get file manager", "error", err)
	} else {
		if err := fileManager.WriteTokens(p.name, tokens); err != nil {
			log.Debug("Failed to save tokens", "error", err)
		}
	}

	return p.tokensToCredentials(tokens)
}

// tokensToCredentials converts OktaTokens to OktaCredentials.
func (p *deviceCodeProvider) tokensToCredentials(tokens *oktaCloud.OktaTokens) (*authTypes.OktaCredentials, error) {
	return &authTypes.OktaCredentials{
		OrgURL:                p.orgURL,
		AccessToken:           tokens.AccessToken,
		IDToken:               tokens.IDToken,
		RefreshToken:          tokens.RefreshToken,
		ExpiresAt:             tokens.ExpiresAt,
		RefreshTokenExpiresAt: tokens.RefreshTokenExpiresAt,
		Scope:                 tokens.Scope,
	}, nil
}

// Validate checks the provider configuration.
func (p *deviceCodeProvider) Validate() error {
	defer perf.Track(nil, "okta.deviceCodeProvider.Validate")()

	if p.orgURL == "" {
		return fmt.Errorf("%w: org_url is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.clientID == "" {
		return fmt.Errorf("%w: client_id is required", errUtils.ErrInvalidProviderConfig)
	}

	// Validate org URL format.
	if _, err := url.Parse(p.orgURL); err != nil {
		return fmt.Errorf("%w: invalid org_url: %w", errUtils.ErrInvalidProviderConfig, err)
	}

	return nil
}

// Environment returns Okta-specific environment variables for this provider.
func (p *deviceCodeProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	if p.orgURL != "" {
		env["OKTA_ORG_URL"] = p.orgURL
		env["OKTA_BASE_URL"] = p.orgURL
	}
	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes (Terraform, etc.).
func (p *deviceCodeProvider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	_ = ctx // Context available for future use.

	fileManager, err := p.getFileManager()
	if err != nil {
		return nil, err
	}

	configDir := fileManager.GetProviderDir(p.name)

	// Use shared Okta environment preparation.
	return oktaCloud.PrepareEnvironment(oktaCloud.PrepareEnvironmentConfig{
		Environ:   environ,
		OrgURL:    p.orgURL,
		ConfigDir: configDir,
	}), nil
}

// Logout removes cached tokens from disk.
func (p *deviceCodeProvider) Logout(ctx context.Context) error {
	_ = ctx // Context available for future use.

	log.Debug("Logout Okta device code provider", "provider", p.name)

	fileManager, err := p.getFileManager()
	if err != nil {
		return err
	}

	return fileManager.Cleanup(p.name)
}

// Paths returns credential files/directories used by this provider.
func (p *deviceCodeProvider) Paths() ([]authTypes.Path, error) {
	fileManager, err := p.getFileManager()
	if err != nil {
		return nil, err
	}

	paths := []authTypes.Path{
		{
			Location: fileManager.GetTokensPath(p.name),
			Type:     authTypes.PathTypeFile,
			Required: true,
			Purpose:  fmt.Sprintf("Okta tokens file for provider %s", p.name),
			Metadata: map[string]string{
				"read_only": "true",
			},
		},
	}

	return paths, nil
}

// GetFilesDisplayPath returns the user-facing display path for credential files.
func (p *deviceCodeProvider) GetFilesDisplayPath() string {
	fileManager, err := p.getFileManager()
	if err != nil {
		return "~/.config/atmos/okta/" + p.name
	}
	return fileManager.GetDisplayPath() + "/" + p.name
}

// Ensure deviceCodeProvider implements Provider interface.
var _ authTypes.Provider = (*deviceCodeProvider)(nil)
