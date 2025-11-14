package azure

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	deviceCodeTokenCacheSubdir    = "azure-device-code"
	deviceCodeTokenCacheFilename  = "token.json"
	deviceCodeTokenCacheDirPerms  = 0o700
	deviceCodeTokenCacheFilePerms = 0o600
)

// deviceCodeTokenCache represents a cached Azure device code access token.
type deviceCodeTokenCache struct {
	AccessToken       string    `json:"accessToken"`
	TokenType         string    `json:"tokenType"`
	ExpiresAt         time.Time `json:"expiresAt"`
	TenantID          string    `json:"tenantId"`
	SubscriptionID    string    `json:"subscriptionId,omitempty"`
	Location          string    `json:"location,omitempty"`
	GraphAPIToken     string    `json:"graphApiToken,omitempty"`
	GraphAPIExpiresAt time.Time `json:"graphApiExpiresAt,omitempty"`
}

// cachedTokenResult holds the result of loading a cached token.
type cachedTokenResult struct {
	AccessToken       string
	ExpiresAt         time.Time
	GraphAPIToken     string
	GraphAPIExpiresAt time.Time
}

// CacheStorage defines interface for token cache storage operations.
// This interface enables testing without filesystem dependencies.
type CacheStorage interface {
	// ReadFile reads the cache file at the given path.
	ReadFile(path string) ([]byte, error)
	// WriteFile writes data to the cache file at the given path.
	WriteFile(path string, data []byte, perm os.FileMode) error
	// Remove deletes the cache file at the given path.
	Remove(path string) error
	// MkdirAll creates directory path with permissions.
	MkdirAll(path string, perm os.FileMode) error
	// GetXDGCacheDir returns the XDG cache directory for the given subdirectory.
	GetXDGCacheDir(subdir string, perm os.FileMode) (string, error)
}

// defaultCacheStorage implements CacheStorage using real filesystem operations.
type defaultCacheStorage struct{}

func (d *defaultCacheStorage) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (d *defaultCacheStorage) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (d *defaultCacheStorage) Remove(path string) error {
	return os.Remove(path)
}

func (d *defaultCacheStorage) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (d *defaultCacheStorage) GetXDGCacheDir(subdir string, perm os.FileMode) (string, error) {
	return xdg.GetXDGCacheDir(subdir, perm)
}

// getTokenCachePath returns the XDG-compliant cache path for device code token.
// Path format: ~/.cache/atmos/azure-device-code/<provider-name>/token.json.
func (p *deviceCodeProvider) getTokenCachePath() (string, error) {
	cacheDir, err := p.cacheStorage.GetXDGCacheDir(deviceCodeTokenCacheSubdir, deviceCodeTokenCacheDirPerms)
	if err != nil {
		return "", fmt.Errorf("failed to get XDG cache directory: %w", err)
	}

	// Create provider-specific subdirectory.
	providerCacheDir := filepath.Join(cacheDir, p.name)
	if err := p.cacheStorage.MkdirAll(providerCacheDir, deviceCodeTokenCacheDirPerms); err != nil {
		return "", fmt.Errorf("failed to create provider cache directory: %w", err)
	}

	return filepath.Join(providerCacheDir, deviceCodeTokenCacheFilename), nil
}

// loadCachedToken loads and validates a cached device code token.
// Returns the cached token result or empty values if cache miss or expired.
// Cache failures are treated as non-fatal and result in empty values being returned.
func (p *deviceCodeProvider) loadCachedToken() cachedTokenResult {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		// If we can't get cache path, just skip caching.
		log.Debug("Failed to get token cache path, skipping cache check", "error", err)
		return cachedTokenResult{}
	}

	// Check if cache file exists.
	data, err := p.cacheStorage.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("No cached Azure device code token found", "path", tokenPath)
			return cachedTokenResult{}
		}
		log.Debug("Failed to read cached token", "error", err)
		return cachedTokenResult{}
	}

	// Parse cached token.
	var cache deviceCodeTokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		log.Debug("Failed to parse cached token, will re-authenticate", "error", err)
		return cachedTokenResult{}
	}

	// Validate token hasn't expired (with 5 minute buffer).
	if time.Now().Add(5 * time.Minute).After(cache.ExpiresAt) {
		log.Debug("Cached Azure device code token expired", "expiresAt", cache.ExpiresAt)
		return cachedTokenResult{}
	}

	// Validate token matches current provider config.
	if cache.TenantID != p.tenantID {
		log.Debug("Cached token tenant mismatch", "cachedTenant", cache.TenantID, "configTenant", p.tenantID)
		return cachedTokenResult{}
	}

	log.Debug("Using cached Azure device code token", "expiresAt", cache.ExpiresAt)
	return cachedTokenResult{
		AccessToken:       cache.AccessToken,
		ExpiresAt:         cache.ExpiresAt,
		GraphAPIToken:     cache.GraphAPIToken,
		GraphAPIExpiresAt: cache.GraphAPIExpiresAt,
	}
}

// saveCachedToken saves an Azure device code access token to the cache.
func (p *deviceCodeProvider) saveCachedToken(accessToken, tokenType string, expiresAt time.Time, graphToken string, graphExpiresAt time.Time) error {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		// If we can't get cache path, just skip caching (non-fatal).
		log.Debug("Failed to get token cache path, skipping cache save", "error", err)
		return nil
	}

	cache := deviceCodeTokenCache{
		AccessToken:       accessToken,
		TokenType:         tokenType,
		ExpiresAt:         expiresAt,
		TenantID:          p.tenantID,
		SubscriptionID:    p.subscriptionID,
		Location:          p.location,
		GraphAPIToken:     graphToken,
		GraphAPIExpiresAt: graphExpiresAt,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token cache: %w", err)
	}

	if err := p.cacheStorage.WriteFile(tokenPath, data, deviceCodeTokenCacheFilePerms); err != nil {
		return fmt.Errorf("failed to write token cache: %w", err)
	}

	log.Debug("Saved Azure device code token to cache", "path", tokenPath, "expiresAt", expiresAt)
	return nil
}

// deleteCachedToken removes the cached device code token.
func (p *deviceCodeProvider) deleteCachedToken() error {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		// If we can't get cache path, log and return error.
		log.Debug("Failed to get token cache path for deletion", "error", err)
		return err
	}

	if err := p.cacheStorage.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		log.Debug("Failed to delete cached token", "error", err)
		return nil // Non-fatal.
	}

	log.Debug("Deleted cached Azure device code token", "path", tokenPath)
	return nil
}

// tokenCacheUpdate holds tokens and expiration times for updating Azure CLI MSAL cache.
type tokenCacheUpdate struct {
	AccessToken       string    // Management API access token (for azurerm backend/provider)
	ExpiresAt         time.Time // Expiration time for management token
	GraphToken        string    // Graph API access token (for azuread provider), empty string if not available
	GraphExpiresAt    time.Time // Expiration time for graph token, zero value if not available
	KeyVaultToken     string    // KeyVault API access token (for azurerm provider KeyVault operations), empty string if not available
	KeyVaultExpiresAt time.Time // Expiration time for KeyVault token, zero value if not available
}

// updateAzureCLICache updates the Azure CLI MSAL token cache so Terraform can use it.
// This makes `atmos auth login` work exactly like `az login`.
func (p *deviceCodeProvider) updateAzureCLICache(update tokenCacheUpdate) error {
	// Decode JWT to get user OID and username.
	userOID, err := extractOIDFromToken(update.AccessToken)
	if err != nil {
		log.Debug("Failed to extract OID from token, skipping Azure CLI cache update", "error", err)
		return nil // Non-fatal.
	}

	username, err := extractUsernameFromToken(update.AccessToken)
	if err != nil {
		log.Debug("Failed to extract username from token, using fallback", "error", err)
		username = "user@unknown" // Fallback username.
	}

	// Azure CLI MSAL cache path.
	home, err := os.UserHomeDir()
	if err != nil {
		log.Debug("Failed to get home directory", "error", err)
		return nil
	}

	msalCachePath := filepath.Join(home, ".azure", "msal_token_cache.json")

	// Load existing cache or create new one.
	var cache map[string]interface{}
	data, err := os.ReadFile(msalCachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debug("Failed to read Azure CLI MSAL cache", "error", err)
		}
		// Create new cache structure.
		cache = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &cache); err != nil {
			log.Debug("Failed to parse Azure CLI MSAL cache", "error", err)
			cache = make(map[string]interface{})
		}
	}

	// Ensure AccessToken section exists.
	accessTokenSection, ok := cache[azureCloud.FieldAccessToken].(map[string]interface{})
	if !ok {
		accessTokenSection = make(map[string]interface{})
		cache[azureCloud.FieldAccessToken] = accessTokenSection
	}

	// Ensure Account section exists.
	accountSection, ok := cache["Account"].(map[string]interface{})
	if !ok {
		accountSection = make(map[string]interface{})
		cache["Account"] = accountSection
	}

	// Create common MSAL identifiers.
	homeAccountID := fmt.Sprintf("%s.%s", userOID, p.tenantID)
	environment := "login.microsoftonline.com"
	clientID := "04b07795-8ddb-461a-bbee-02f9e1bf7b46" // Azure CLI public client.
	realm := p.tenantID

	// Add Account entry (required for azuread provider).
	// The Account entry tells the provider which user/account is authenticated.
	accountKey := fmt.Sprintf("%s-%s-%s", homeAccountID, environment, realm)
	accountEntry := map[string]interface{}{
		azureCloud.FieldHomeAccountID: homeAccountID,
		azureCloud.FieldEnvironment:   environment,
		azureCloud.FieldRealm:         realm,
		"local_account_id":            userOID,
		"username":                    username,
		"authority_type":              "MSSTS",
		"account_source":              "device_code",
	}
	accountSection[accountKey] = accountEntry
	log.Debug("Added Account entry to MSAL cache", azureCloud.LogFieldKey, accountKey, "username", username)

	// Management API scope (matches az login format).
	scope := "https://management.azure.com/.default https://management.azure.com/user_impersonation"

	// Create and add management token entry.
	cacheKey, tokenEntry := createMSALTokenEntry(&msalTokenParams{
		Token:         update.AccessToken,
		ExpiresAt:     update.ExpiresAt,
		Scope:         scope,
		HomeAccountID: homeAccountID,
		Environment:   environment,
		ClientID:      clientID,
		Realm:         realm,
	})
	accessTokenSection[cacheKey] = tokenEntry

	// Add entry for Microsoft Graph API (used by azuread provider) if available.
	if update.GraphToken != "" {
		graphScope := "https://graph.microsoft.com/.default"
		graphCacheKey, graphTokenEntry := createMSALTokenEntry(&msalTokenParams{
			Token:         update.GraphToken,
			ExpiresAt:     update.GraphExpiresAt,
			Scope:         graphScope,
			HomeAccountID: homeAccountID,
			Environment:   environment,
			ClientID:      clientID,
			Realm:         realm,
		})
		accessTokenSection[graphCacheKey] = graphTokenEntry
		log.Debug("Added Graph API token to MSAL cache", azureCloud.LogFieldKey, graphCacheKey)
	} else {
		log.Debug("No Graph API token available, azuread provider may not work")
	}

	// Add entry for Azure KeyVault API (used by azurerm provider for KeyVault operations) if available.
	if update.KeyVaultToken != "" {
		keyVaultScope := "https://vault.azure.net/.default"
		keyVaultCacheKey, keyVaultTokenEntry := createMSALTokenEntry(&msalTokenParams{
			Token:         update.KeyVaultToken,
			ExpiresAt:     update.KeyVaultExpiresAt,
			Scope:         keyVaultScope,
			HomeAccountID: homeAccountID,
			Environment:   environment,
			ClientID:      clientID,
			Realm:         realm,
		})
		accessTokenSection[keyVaultCacheKey] = keyVaultTokenEntry
		log.Debug("Added KeyVault API token to MSAL cache", azureCloud.LogFieldKey, keyVaultCacheKey)
	} else {
		log.Debug("No KeyVault API token available, KeyVault operations may not work")
	}

	// Write updated cache.
	updatedData, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		log.Debug("Failed to marshal Azure CLI MSAL cache", "error", err)
		return nil
	}

	// Ensure .azure directory exists.
	azureDir := filepath.Join(home, ".azure")
	if err := os.MkdirAll(azureDir, azureCloud.DirPermissions); err != nil {
		log.Debug("Failed to create .azure directory", "error", err)
		return nil
	}

	if err := os.WriteFile(msalCachePath, updatedData, azureCloud.FilePermissions); err != nil {
		log.Debug("Failed to write Azure CLI MSAL cache", "error", err)
		return nil
	}

	log.Debug("Updated Azure CLI MSAL token cache", "path", msalCachePath, azureCloud.LogFieldKey, cacheKey)

	// Update azureProfile.json to set the correct default subscription.
	// This is required for azuread and azapi providers to work.
	if err := p.updateAzureProfile(home, username); err != nil {
		log.Debug("Failed to update Azure profile", "error", err)
		// Non-fatal - MSAL cache should be sufficient.
	}

	return nil
}

// msalTokenParams holds parameters for creating an MSAL token cache entry.
type msalTokenParams struct {
	Token         string
	ExpiresAt     time.Time
	Scope         string
	HomeAccountID string
	Environment   string
	ClientID      string
	Realm         string
}

// createMSALTokenEntry creates an MSAL token cache entry in Azure CLI format.
func createMSALTokenEntry(params *msalTokenParams) (string, map[string]interface{}) {
	cachedAt := time.Now().Unix()
	expiresOn := params.ExpiresAt.Unix()

	cacheKey := fmt.Sprintf("%s-%s-accesstoken-%s-%s-%s",
		params.HomeAccountID, params.Environment, params.ClientID, params.Realm, params.Scope)

	tokenEntry := map[string]interface{}{
		"credential_type":             "AccessToken",
		"secret":                      params.Token,
		azureCloud.FieldHomeAccountID: params.HomeAccountID,
		azureCloud.FieldEnvironment:   params.Environment,
		"client_id":                   params.ClientID,
		"target":                      params.Scope,
		azureCloud.FieldRealm:         params.Realm,
		"token_type":                  "Bearer",
		"cached_at":                   fmt.Sprintf(azureCloud.IntFormat, cachedAt),
		"expires_on":                  fmt.Sprintf(azureCloud.IntFormat, expiresOn),
		"extended_expires_on":         fmt.Sprintf(azureCloud.IntFormat, expiresOn),
	}

	return cacheKey, tokenEntry
}

// extractOIDFromToken decodes a JWT token and extracts the user OID claim.
func extractOIDFromToken(token string) (string, error) {
	claims, err := extractJWTClaims(token)
	if err != nil {
		return "", err
	}

	oid, ok := claims["oid"].(string)
	if !ok {
		return "", errUtils.ErrAzureOIDClaimNotFound
	}

	return oid, nil
}

// extractUsernameFromToken decodes a JWT token and extracts the username (UPN).
func extractUsernameFromToken(token string) (string, error) {
	claims, err := extractJWTClaims(token)
	if err != nil {
		return "", err
	}

	// Try upn first, then unique_name, then email.
	if upn, ok := claims["upn"].(string); ok && upn != "" {
		return upn, nil
	}
	if uniqueName, ok := claims["unique_name"].(string); ok && uniqueName != "" {
		return uniqueName, nil
	}
	if email, ok := claims["email"].(string); ok && email != "" {
		return email, nil
	}

	return "", errUtils.ErrAzureUsernameClaimNotFound
}

// extractJWTClaims decodes a JWT token and returns the claims.
func extractJWTClaims(token string) (map[string]interface{}, error) {
	// JWT has 3 parts separated by dots: header.payload.signature.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errUtils.ErrAzureInvalidJWTFormat
	}

	// Decode payload (second part).
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	// Parse JSON to get claims.
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	return claims, nil
}

// updateAzureProfile updates the azureProfile.json file with the current subscription.
// This file is required by some Azure providers (azuread, azapi) to discover the default subscription.
func (p *deviceCodeProvider) updateAzureProfile(home, username string) error {
	profilePath := filepath.Join(home, ".azure", "azureProfile.json")

	// Load existing profile or create new one.
	var profile map[string]interface{}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read Azure profile: %w", err)
		}
		// Create new profile structure.
		profile = map[string]interface{}{
			"installationId": "",
			"subscriptions":  []interface{}{},
		}
	} else {
		if err := json.Unmarshal(data, &profile); err != nil {
			return fmt.Errorf("failed to parse Azure profile: %w", err)
		}
	}

	// Update subscriptions in profile.
	profile["subscriptions"] = azureCloud.UpdateSubscriptionsInProfile(profile, username, p.tenantID, p.subscriptionID)

	// Write updated profile.
	updatedData, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Azure profile: %w", err)
	}

	if err := os.WriteFile(profilePath, updatedData, azureCloud.FilePermissions); err != nil {
		return fmt.Errorf("failed to write Azure profile: %w", err)
	}

	log.Debug("Updated Azure profile", "path", profilePath, "subscription", p.subscriptionID)
	return nil
}
