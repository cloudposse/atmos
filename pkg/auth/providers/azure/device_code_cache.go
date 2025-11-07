package azure

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
// Returns the management token, Graph API token, and their expirations if valid, or empty values if cache miss or expired.
func (p *deviceCodeProvider) loadCachedToken() (string, time.Time, string, time.Time, error) {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		// If we can't get cache path, just skip caching.
		log.Debug("Failed to get token cache path, skipping cache check", "error", err)
		return "", time.Time{}, "", time.Time{}, nil
	}

	// Check if cache file exists.
	data, err := p.cacheStorage.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("No cached Azure device code token found", "path", tokenPath)
			return "", time.Time{}, "", time.Time{}, nil
		}
		log.Debug("Failed to read cached token", "error", err)
		return "", time.Time{}, "", time.Time{}, nil
	}

	// Parse cached token.
	var cache deviceCodeTokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		log.Debug("Failed to parse cached token, will re-authenticate", "error", err)
		return "", time.Time{}, "", time.Time{}, nil
	}

	// Validate token hasn't expired (with 5 minute buffer).
	if time.Now().Add(5 * time.Minute).After(cache.ExpiresAt) {
		log.Debug("Cached Azure device code token expired", "expiresAt", cache.ExpiresAt)
		return "", time.Time{}, "", time.Time{}, nil
	}

	// Validate token matches current provider config.
	if cache.TenantID != p.tenantID {
		log.Debug("Cached token tenant mismatch", "cachedTenant", cache.TenantID, "configTenant", p.tenantID)
		return "", time.Time{}, "", time.Time{}, nil
	}

	log.Debug("Using cached Azure device code token", "expiresAt", cache.ExpiresAt)
	return cache.AccessToken, cache.ExpiresAt, cache.GraphAPIToken, cache.GraphAPIExpiresAt, nil
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
		log.Debug("Failed to marshal token cache", "error", err)
		return nil // Non-fatal.
	}

	if err := p.cacheStorage.WriteFile(tokenPath, data, deviceCodeTokenCacheFilePerms); err != nil {
		log.Debug("Failed to write token cache", "error", err)
		return nil // Non-fatal.
	}

	log.Debug("Saved Azure device code token to cache", "path", tokenPath, "expiresAt", expiresAt)
	return nil
}

// deleteCachedToken removes the cached device code token.
func (p *deviceCodeProvider) deleteCachedToken() error {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		// If we can't get cache path, nothing to delete.
		return nil
	}

	if err := p.cacheStorage.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		log.Debug("Failed to delete cached token", "error", err)
		return nil // Non-fatal.
	}

	log.Debug("Deleted cached Azure device code token", "path", tokenPath)
	return nil
}

// updateAzureCLICache updates the Azure CLI MSAL token cache so Terraform can use it.
// This makes `atmos auth login` work exactly like `az login`.
// Parameters:
//   - accessToken: Management API access token (for azurerm backend/provider).
//   - expiresAt: Expiration time for management token.
//   - graphToken: Graph API access token (for azuread provider), empty string if not available.
//   - graphExpiresAt: Expiration time for graph token, zero value if not available.
//   - keyVaultToken: KeyVault API access token (for azurerm provider KeyVault operations), empty string if not available.
//   - keyVaultExpiresAt: Expiration time for KeyVault token, zero value if not available.
func (p *deviceCodeProvider) updateAzureCLICache(accessToken string, expiresAt time.Time, graphToken string, graphExpiresAt time.Time, keyVaultToken string, keyVaultExpiresAt time.Time) error {
	// Decode JWT to get user OID and username.
	userOID, err := extractOIDFromToken(accessToken)
	if err != nil {
		log.Debug("Failed to extract OID from token, skipping Azure CLI cache update", "error", err)
		return nil // Non-fatal.
	}

	username, err := extractUsernameFromToken(accessToken)
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
	accessTokenSection, ok := cache["AccessToken"].(map[string]interface{})
	if !ok {
		accessTokenSection = make(map[string]interface{})
		cache["AccessToken"] = accessTokenSection
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
		"home_account_id":  homeAccountID,
		"environment":      environment,
		"realm":            realm,
		"local_account_id": userOID,
		"username":         username,
		"authority_type":   "MSSTS",
		"account_source":   "device_code",
	}
	accountSection[accountKey] = accountEntry
	log.Debug("Added Account entry to MSAL cache", "key", accountKey, "username", username)

	// Management API scope (matches az login format).
	scope := "https://management.azure.com/.default https://management.azure.com/user_impersonation"

	cacheKey := fmt.Sprintf("%s-%s-accesstoken-%s-%s-%s",
		homeAccountID, environment, clientID, realm, scope)

	// Create token entry in MSAL format.
	cachedAt := time.Now().Unix()
	expiresOn := expiresAt.Unix()

	tokenEntry := map[string]interface{}{
		"credential_type":       "AccessToken",
		"secret":                accessToken,
		"home_account_id":       homeAccountID,
		"environment":           environment,
		"client_id":             clientID,
		"target":                scope,
		"realm":                 realm,
		"token_type":            "Bearer",
		"cached_at":             fmt.Sprintf("%d", cachedAt),
		"expires_on":            fmt.Sprintf("%d", expiresOn),
		"extended_expires_on":   fmt.Sprintf("%d", expiresOn),
	}

	accessTokenSection[cacheKey] = tokenEntry

	// Add entry for Microsoft Graph API (used by azuread provider) if available.
	if graphToken != "" {
		graphScope := "https://graph.microsoft.com/.default"
		graphCacheKey := fmt.Sprintf("%s-%s-accesstoken-%s-%s-%s",
			homeAccountID, environment, clientID, realm, graphScope)

		graphCachedAt := time.Now().Unix()
		graphExpiresOnUnix := graphExpiresAt.Unix()

		graphTokenEntry := map[string]interface{}{
			"credential_type":     "AccessToken",
			"secret":              graphToken,
			"home_account_id":     homeAccountID,
			"environment":         environment,
			"client_id":           clientID,
			"target":              graphScope,
			"realm":               realm,
			"token_type":          "Bearer",
			"cached_at":           fmt.Sprintf("%d", graphCachedAt),
			"expires_on":          fmt.Sprintf("%d", graphExpiresOnUnix),
			"extended_expires_on": fmt.Sprintf("%d", graphExpiresOnUnix),
		}

		accessTokenSection[graphCacheKey] = graphTokenEntry
		log.Debug("Added Graph API token to MSAL cache", "key", graphCacheKey)
	} else {
		log.Debug("No Graph API token available, azuread provider may not work")
	}

	// Add entry for Azure KeyVault API (used by azurerm provider for KeyVault operations) if available.
	if keyVaultToken != "" {
		keyVaultScope := "https://vault.azure.net/.default"
		keyVaultCacheKey := fmt.Sprintf("%s-%s-accesstoken-%s-%s-%s",
			homeAccountID, environment, clientID, realm, keyVaultScope)

		keyVaultCachedAt := time.Now().Unix()
		keyVaultExpiresOnUnix := keyVaultExpiresAt.Unix()

		keyVaultTokenEntry := map[string]interface{}{
			"credential_type":     "AccessToken",
			"secret":              keyVaultToken,
			"home_account_id":     homeAccountID,
			"environment":         environment,
			"client_id":           clientID,
			"target":              keyVaultScope,
			"realm":               realm,
			"token_type":          "Bearer",
			"cached_at":           fmt.Sprintf("%d", keyVaultCachedAt),
			"expires_on":          fmt.Sprintf("%d", keyVaultExpiresOnUnix),
			"extended_expires_on": fmt.Sprintf("%d", keyVaultExpiresOnUnix),
		}

		accessTokenSection[keyVaultCacheKey] = keyVaultTokenEntry
		log.Debug("Added KeyVault API token to MSAL cache", "key", keyVaultCacheKey)
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
	if err := os.MkdirAll(azureDir, 0o700); err != nil {
		log.Debug("Failed to create .azure directory", "error", err)
		return nil
	}

	if err := os.WriteFile(msalCachePath, updatedData, 0o600); err != nil {
		log.Debug("Failed to write Azure CLI MSAL cache", "error", err)
		return nil
	}

	log.Debug("Updated Azure CLI MSAL token cache", "path", msalCachePath, "key", cacheKey)

	// Update azureProfile.json to set the correct default subscription.
	// This is required for azuread and azapi providers to work.
	if err := p.updateAzureProfile(home, username); err != nil {
		log.Debug("Failed to update Azure profile", "error", err)
		// Non-fatal - MSAL cache should be sufficient.
	}

	return nil
}

// extractOIDFromToken decodes a JWT token and extracts the user OID claim.
func extractOIDFromToken(token string) (string, error) {
	claims, err := extractJWTClaims(token)
	if err != nil {
		return "", err
	}

	oid, ok := claims["oid"].(string)
	if !ok {
		return "", fmt.Errorf("oid claim not found in token")
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

	return "", fmt.Errorf("no username claim found in token (tried upn, unique_name, email)")
}

// extractJWTClaims decodes a JWT token and returns the claims.
func extractJWTClaims(token string) (map[string]interface{}, error) {
	// JWT has 3 parts separated by dots: header.payload.signature.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
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

	// Get subscriptions array.
	subscriptionsRaw, ok := profile["subscriptions"].([]interface{})
	if !ok {
		subscriptionsRaw = []interface{}{}
	}

	// Find or create subscription entry.
	var found bool
	for i, subRaw := range subscriptionsRaw {
		sub, ok := subRaw.(map[string]interface{})
		if !ok {
			continue
		}

		subID, _ := sub["id"].(string)
		if subID == p.subscriptionID {
			// Update existing subscription.
			sub["tenantId"] = p.tenantID
			sub["isDefault"] = true
			sub["state"] = "Enabled"
			sub["user"] = map[string]interface{}{
				"name": username,
				"type": "user",
			}
			sub["environmentName"] = "AzureCloud"
			subscriptionsRaw[i] = sub
			found = true
			log.Debug("Updated existing subscription in Azure profile", "subscription", p.subscriptionID)
		} else {
			// Mark other subscriptions as not default.
			sub["isDefault"] = false
			subscriptionsRaw[i] = sub
		}
	}

	// Add new subscription if not found.
	if !found && p.subscriptionID != "" {
		newSub := map[string]interface{}{
			"id":              p.subscriptionID,
			"name":            p.subscriptionID, // We don't have the name, use ID.
			"tenantId":        p.tenantID,
			"isDefault":       true,
			"state":           "Enabled",
			"environmentName": "AzureCloud",
			"user": map[string]interface{}{
				"name": username,
				"type": "user",
			},
		}
		subscriptionsRaw = append(subscriptionsRaw, newSub)
		log.Debug("Added new subscription to Azure profile", "subscription", p.subscriptionID)
	}

	profile["subscriptions"] = subscriptionsRaw

	// Write updated profile.
	updatedData, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Azure profile: %w", err)
	}

	if err := os.WriteFile(profilePath, updatedData, 0o600); err != nil {
		return fmt.Errorf("failed to write Azure profile: %w", err)
	}

	log.Debug("Updated Azure profile", "path", profilePath, "subscription", p.subscriptionID)
	return nil
}
