package github

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// HTTP client timeout for GitHub API requests.
	httpClientTimeout = 30 * time.Second
)

// appProvider implements GitHub App authentication.
type appProvider struct {
	name           string
	config         *schema.Provider
	appID          string
	installationID string
	privateKey     *rsa.PrivateKey
	permissions    map[string]string
	repositories   []string
	httpClient     *http.Client
}

// NewAppProvider creates a new GitHub App authentication provider.
func NewAppProvider(name string, config *schema.Provider) (types.Provider, error) {
	defer perf.Track(nil, "github.NewAppProvider")()

	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}

	if name == "" {
		return nil, fmt.Errorf("%w: provider name is required", errUtils.ErrInvalidProviderConfig)
	}

	spec := config.Spec
	if spec == nil {
		return nil, fmt.Errorf("%w: provider spec is required", errUtils.ErrInvalidProviderConfig)
	}

	// Extract app_id (required).
	appID, ok := spec["app_id"].(string)
	if !ok || appID == "" {
		return nil, fmt.Errorf("%w: app_id is required", errUtils.ErrInvalidProviderConfig)
	}

	// Extract installation_id (required).
	installationID, ok := spec["installation_id"].(string)
	if !ok || installationID == "" {
		return nil, fmt.Errorf("%w: installation_id is required", errUtils.ErrInvalidProviderConfig)
	}

	// Load private key from file or environment variable.
	privateKey, err := loadPrivateKey(spec)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUtils.ErrInvalidProviderConfig, err)
	}

	// Extract optional permissions and repositories.
	permissions := extractPermissions(spec)
	repositories := extractRepositories(spec)

	return &appProvider{
		name:           name,
		config:         config,
		appID:          appID,
		installationID: installationID,
		privateKey:     privateKey,
		permissions:    permissions,
		repositories:   repositories,
		httpClient: &http.Client{
			Timeout: httpClientTimeout,
		},
	}, nil
}

// extractPermissions extracts GitHub App permissions from the provider spec.
func extractPermissions(spec map[string]interface{}) map[string]string {
	permissions := make(map[string]string)
	permsInterface, ok := spec["permissions"]
	if !ok {
		return permissions
	}

	permsMap, ok := permsInterface.(map[string]interface{})
	if !ok {
		return permissions
	}

	for k, v := range permsMap {
		if vStr, ok := v.(string); ok {
			permissions[k] = vStr
		}
	}

	return permissions
}

// extractRepositories extracts GitHub App repositories from the provider spec.
func extractRepositories(spec map[string]interface{}) []string {
	var repositories []string
	reposInterface, ok := spec["repositories"]
	if !ok {
		return repositories
	}

	reposList, ok := reposInterface.([]interface{})
	if !ok {
		return repositories
	}

	for _, repo := range reposList {
		if repoStr, ok := repo.(string); ok {
			repositories = append(repositories, repoStr)
		}
	}

	return repositories
}

// loadPrivateKeyPEM loads PEM data from file or environment variable.
func loadPrivateKeyPEM(spec map[string]interface{}) ([]byte, error) {
	// Try private_key_path first.
	if keyPath, ok := spec["private_key_path"].(string); ok && keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key from %s: %w", keyPath, err)
		}
		return data, nil
	}

	// Try private_key_env.
	if keyEnv, ok := spec["private_key_env"].(string); ok && keyEnv != "" {
		data := os.Getenv(keyEnv) //nolint:forbidigo // GitHub App private key from environment is expected pattern.
		if data == "" {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrPrivateKeyEnvNotSet, keyEnv)
		}
		return []byte(data), nil
	}

	return nil, errUtils.ErrPrivateKeyConfigMissing
}

// parseRSAPrivateKey parses an RSA private key from PEM bytes, trying both PKCS1 and PKCS8 formats.
func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	// Try PKCS1 format first.
	privateKey, err := x509.ParsePKCS1PrivateKey(pemBytes)
	if err == nil {
		return privateKey, nil
	}

	// Try PKCS8 format.
	key, err := x509.ParsePKCS8PrivateKey(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errUtils.ErrPrivateKeyNotRSA
	}

	return privateKey, nil
}

// loadPrivateKey loads the GitHub App private key from file or environment variable.
func loadPrivateKey(spec map[string]interface{}) (*rsa.PrivateKey, error) {
	defer perf.Track(nil, "github.loadPrivateKey")()

	pemData, err := loadPrivateKeyPEM(spec)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errUtils.ErrPEMDecodeFailed
	}

	return parseRSAPrivateKey(block.Bytes)
}

// Name returns the provider name.
func (p *appProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for GitHub App provider.
func (p *appProvider) PreAuthenticate(_ types.AuthManager) error {
	return nil
}

// Kind returns the provider kind.
func (p *appProvider) Kind() string {
	return KindApp
}

// Authenticate performs GitHub App authentication.
func (p *appProvider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "github.appProvider.Authenticate")()

	// Generate JWT for GitHub App.
	jwtToken, err := p.generateJWT()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate JWT: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Request installation access token.
	installationToken, expiresAt, err := p.getInstallationToken(ctx, jwtToken)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get installation token: %v", errUtils.ErrAuthenticationFailed, err)
	}

	return &types.GitHubAppCredentials{
		Token:          installationToken,
		AppID:          p.appID,
		InstallationID: p.installationID,
		Provider:       p.name,
		Expiration:     expiresAt,
	}, nil
}

// generateJWT generates a JWT for GitHub App authentication.
func (p *appProvider) generateJWT() (string, error) {
	defer perf.Track(nil, "github.appProvider.generateJWT")()

	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)), // GitHub accepts up to 10 minutes.
		Issuer:    p.appID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signedToken, err := token.SignedString(p.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return signedToken, nil
}

// getInstallationToken requests an installation access token from GitHub.
func (p *appProvider) getInstallationToken(ctx context.Context, jwtToken string) (string, time.Time, error) {
	defer perf.Track(nil, "github.appProvider.getInstallationToken")()

	url := fmt.Sprintf("https://api.github.com/app/installations/%s/access_tokens", p.installationID)

	// Build request body.
	requestBody := make(map[string]interface{})
	if len(p.permissions) > 0 {
		requestBody["permissions"] = p.permissions
	}
	if len(p.repositories) > 0 {
		requestBody["repositories"] = p.repositories
	}

	var body io.Reader
	if len(requestBody) > 0 {
		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = &readSeeker{data: jsonData}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to request installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("%w: status %d: %s", errUtils.ErrInstallationTokenRequest, resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode installation token response: %w", err)
	}

	return result.Token, result.ExpiresAt, nil
}

// Validate validates the provider configuration.
func (p *appProvider) Validate() error {
	if p.appID == "" {
		return fmt.Errorf("%w: app_id is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.installationID == "" {
		return fmt.Errorf("%w: installation_id is required", errUtils.ErrInvalidProviderConfig)
	}
	if p.privateKey == nil {
		return fmt.Errorf("%w: private key is required", errUtils.ErrInvalidProviderConfig)
	}
	return nil
}

// Environment returns environment variables for this provider.
func (p *appProvider) Environment() (map[string]string, error) {
	// GitHub App provider doesn't set additional environment variables.
	// The installation token is passed to downstream identities via credentials.
	return map[string]string{}, nil
}

// readSeeker is a helper to make []byte compatible with io.Reader.
type readSeeker struct {
	data []byte
	pos  int
}

func (r *readSeeker) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
