package azure

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SetupFiles sets up Azure credentials files for the given identity.
// BasePath specifies the base directory for Azure files (from provider's files.base_path).
// If empty, uses the default ~/.azure/atmos path.
func SetupFiles(providerName, identityName string, creds types.ICredentials, basePath string) error {
	azureCreds, ok := creds.(*types.AzureCredentials)
	if !ok {
		return nil // No Azure credentials to setup.
	}

	// Create Azure file manager with configured or default path.
	fileManager, err := NewAzureFileManager(basePath)
	if err != nil {
		return errors.Join(errUtils.ErrAuthenticationFailed, err)
	}

	// Write credentials file.
	if err := fileManager.WriteCredentials(providerName, identityName, azureCreds); err != nil {
		return fmt.Errorf("failed to write Azure credentials: %w", err)
	}

	return nil
}

// SetAuthContextParams contains parameters for SetAuthContext.
type SetAuthContextParams struct {
	AuthContext  *schema.AuthContext
	StackInfo    *schema.ConfigAndStacksInfo
	ProviderName string
	IdentityName string
	Credentials  types.ICredentials
	BasePath     string
}

// SetAuthContext populates the Azure auth context with Atmos-managed credential paths.
// This enables in-process Azure SDK calls to use Atmos-managed credentials.
func SetAuthContext(params *SetAuthContextParams) error {
	if params == nil {
		return fmt.Errorf("%w: SetAuthContext parameters cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	authContext := params.AuthContext
	if authContext == nil {
		return nil // No auth context to populate.
	}

	azureCreds, ok := params.Credentials.(*types.AzureCredentials)
	if !ok {
		return nil // No Azure credentials to setup.
	}

	m, err := NewAzureFileManager(params.BasePath)
	if err != nil {
		return errors.Join(errUtils.ErrAuthenticationFailed, err)
	}

	credentialsPath := m.GetCredentialsPath(params.ProviderName)

	// Start with location from credentials.
	location := azureCreds.Location

	// Check for component-level location override from merged auth config.
	if locationOverride := getComponentLocationOverride(params.StackInfo, params.IdentityName); locationOverride != "" {
		location = locationOverride
		log.Debug("Using component-level location override",
			"identity", params.IdentityName,
			"location", location,
		)
	}

	// Populate Azure auth context as the single source of truth.
	authContext.Azure = &schema.AzureAuthContext{
		CredentialsFile: credentialsPath,
		Profile:         params.IdentityName,
		SubscriptionID:  azureCreds.SubscriptionID,
		TenantID:        azureCreds.TenantID,
		Location:        location,
	}

	log.Debug("Set Azure auth context",
		"profile", params.IdentityName,
		"credentials", credentialsPath,
		"subscription", azureCreds.SubscriptionID,
		"tenant", azureCreds.TenantID,
		"location", location,
	)

	return nil
}

// getComponentLocationOverride extracts location override from component auth config.
func getComponentLocationOverride(stackInfo *schema.ConfigAndStacksInfo, identityName string) string {
	if stackInfo == nil || stackInfo.ComponentAuthSection == nil {
		return ""
	}

	identities, ok := stackInfo.ComponentAuthSection["identities"].(map[string]any)
	if !ok {
		return ""
	}

	identityCfg, ok := identities[identityName].(map[string]any)
	if !ok {
		return ""
	}

	locationOverride, ok := identityCfg["location"].(string)
	if !ok {
		return ""
	}

	return locationOverride
}

// SetEnvironmentVariables derives Azure environment variables from AuthContext.
// This populates ComponentEnvSection/ComponentEnvList for spawned processes.
// The auth context is the single source of truth; this function derives from it.
//
// Uses PrepareEnvironment helper to ensure consistent environment setup across all commands.
// This clears conflicting credential env vars and sets Azure subscription/tenant/location.
//
// Parameters:
//   - authContext: Runtime auth context containing Azure credentials
//   - stackInfo: Stack configuration to populate with environment variables
func SetEnvironmentVariables(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo) error {
	if authContext == nil || authContext.Azure == nil {
		return nil // No auth context to derive from.
	}

	if stackInfo == nil {
		return nil // No stack info to populate.
	}

	azureAuth := authContext.Azure

	// Convert existing environment section to map for PrepareEnvironment.
	environMap := make(map[string]string)
	if stackInfo.ComponentEnvSection != nil {
		for k, v := range stackInfo.ComponentEnvSection {
			if str, ok := v.(string); ok {
				environMap[k] = str
			}
		}
	}

	// Load credentials to get access token.
	// Credentials are stored at ~/.azure/atmos/<provider>/credentials.json
	// Extract provider name from the file path to load credentials.
	var accessToken string
	if azureAuth.CredentialsFile != "" {
		fileManager, err := NewAzureFileManager("")
		if err == nil {
			// Extract provider name from credentials file path.
			// Path format: ~/.azure/atmos/<provider>/credentials.json or /full/path/<provider>/credentials.json
			providerName := filepath.Base(filepath.Dir(azureAuth.CredentialsFile))

			if providerName != "" && providerName != "." {
				creds, err := fileManager.LoadCredentials(providerName)
				if err == nil && creds.AccessToken != "" {
					accessToken = creds.AccessToken
					log.Debug("Loaded access token from credentials file",
						"credentials_file", azureAuth.CredentialsFile,
						"provider", providerName,
					)
				} else if err != nil {
					log.Debug("Failed to load credentials from file",
						"error", err,
						"credentials_file", azureAuth.CredentialsFile,
						"provider", providerName,
					)
				}
			}
		}
	}

	// Use shared PrepareEnvironment helper to get properly configured environment.
	environMap = PrepareEnvironment(
		environMap,
		azureAuth.SubscriptionID,
		azureAuth.TenantID,
		azureAuth.Location,
		azureAuth.CredentialsFile,
		accessToken,
	)

	// Replace ComponentEnvSection with prepared environment.
	// IMPORTANT: We must completely replace, not merge, to ensure deleted keys stay deleted.
	stackInfo.ComponentEnvSection = make(map[string]any, len(environMap))
	for k, v := range environMap {
		stackInfo.ComponentEnvSection[k] = v
	}

	return nil
}

// UpdateAzureCLIFiles updates Azure CLI files (MSAL cache and azureProfile.json) so Terraform providers can use them.
// This makes Atmos authentication work exactly like `az login`.
// This should be called from PostAuthenticate to ensure CLI compatibility.
func UpdateAzureCLIFiles(creds types.ICredentials, tenantID, subscriptionID string) error {
	azureCreds, ok := creds.(*types.AzureCredentials)
	if !ok {
		return nil // Not Azure credentials, nothing to do.
	}

	// Extract user OID and username from token.
	userOID, err := extractOIDFromToken(azureCreds.AccessToken)
	if err != nil {
		log.Debug("Failed to extract OID from token, skipping Azure CLI cache update", "error", err)
		return nil // Non-fatal.
	}

	username, err := extractUsernameFromToken(azureCreds.AccessToken)
	if err != nil {
		log.Debug("Failed to extract username from token, using fallback", "error", err)
		username = "user@unknown" // Fallback username.
	}

	// Get home directory.
	home, err := os.UserHomeDir()
	if err != nil {
		log.Debug("Failed to get home directory", "error", err)
		return nil
	}

	// Extract Graph API token and expiration from credentials if available.
	graphToken := azureCreds.GraphAPIToken
	graphExpiration := azureCreds.GraphAPIExpiration

	// Extract KeyVault token and expiration from credentials if available.
	keyVaultToken := azureCreds.KeyVaultToken
	keyVaultExpiration := azureCreds.KeyVaultExpiration

	// Update MSAL token cache with management, Graph API, and KeyVault tokens.
	if err := updateMSALCache(home, azureCreds.AccessToken, azureCreds.Expiration, graphToken, graphExpiration, keyVaultToken, keyVaultExpiration, userOID, tenantID); err != nil {
		log.Debug("Failed to update MSAL cache", "error", err)
		// Continue to try azureProfile update.
	}

	// Update azureProfile.json.
	if err := updateAzureProfile(home, username, tenantID, subscriptionID); err != nil {
		log.Debug("Failed to update Azure profile", "error", err)
		// Non-fatal.
	}

	return nil
}

// updateMSALCache updates the Azure CLI MSAL token cache.
func updateMSALCache(home, accessToken, expiration, graphToken, graphExpiration, keyVaultToken, keyVaultExpiration, userOID, tenantID string) error {
	msalCachePath := filepath.Join(home, ".azure", "msal_token_cache.json")

	// Load existing cache or create new one.
	var cache map[string]interface{}
	data, err := os.ReadFile(msalCachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read MSAL cache: %w", err)
		}
		cache = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &cache); err != nil {
			return fmt.Errorf("failed to parse MSAL cache: %w", err)
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
	homeAccountID := fmt.Sprintf("%s.%s", userOID, tenantID)
	environment := "login.microsoftonline.com"
	clientID := "04b07795-8ddb-461a-bbee-02f9e1bf7b46" // Azure CLI public client.
	realm := tenantID

	// Add Account entry (required for azuread provider).
	accountKey := fmt.Sprintf("%s-%s-%s", homeAccountID, environment, realm)
	accountEntry := map[string]interface{}{
		"home_account_id":  homeAccountID,
		"environment":      environment,
		"realm":            realm,
		"local_account_id": userOID,
		"username":         extractUsernameOrFallback(accessToken),
		"authority_type":   "MSSTS",
		"account_source":   "device_code",
	}
	accountSection[accountKey] = accountEntry

	// Parse expiration time.
	expiresAt, err := time.Parse(time.RFC3339, expiration)
	if err != nil {
		return fmt.Errorf("failed to parse expiration time: %w", err)
	}

	// Management API scope.
	scope := "https://management.azure.com/.default https://management.azure.com/user_impersonation"
	cacheKey := fmt.Sprintf("%s-%s-accesstoken-%s-%s-%s",
		homeAccountID, environment, clientID, realm, scope)

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

		// Parse Graph API token expiration.
		graphExpiresAt, err := time.Parse(time.RFC3339, graphExpiration)
		if err != nil {
			log.Debug("Failed to parse Graph API token expiration, skipping Graph token cache", "error", err)
		} else {
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
		}
	} else {
		log.Debug("No Graph API token available, azuread provider may not work")
	}

	// Add entry for Azure KeyVault API (used by azurerm provider for KeyVault operations) if available.
	if keyVaultToken != "" {
		keyVaultScope := "https://vault.azure.net/.default"
		keyVaultCacheKey := fmt.Sprintf("%s-%s-accesstoken-%s-%s-%s",
			homeAccountID, environment, clientID, realm, keyVaultScope)

		// Parse KeyVault token expiration.
		keyVaultExpiresAt, err := time.Parse(time.RFC3339, keyVaultExpiration)
		if err != nil {
			log.Debug("Failed to parse KeyVault token expiration, skipping KeyVault token cache", "error", err)
		} else {
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
		}
	} else {
		log.Debug("No KeyVault API token available, KeyVault operations may not work")
	}

	// Write updated cache.
	updatedData, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MSAL cache: %w", err)
	}

	// Ensure .azure directory exists.
	azureDir := filepath.Join(home, ".azure")
	if err := os.MkdirAll(azureDir, 0o700); err != nil {
		return fmt.Errorf("failed to create .azure directory: %w", err)
	}

	if err := os.WriteFile(msalCachePath, updatedData, 0o600); err != nil {
		return fmt.Errorf("failed to write MSAL cache: %w", err)
	}

	log.Debug("Updated Azure CLI MSAL token cache", "path", msalCachePath)
	return nil
}

// updateAzureProfile updates the azureProfile.json file with the current subscription.
func updateAzureProfile(home, username, tenantID, subscriptionID string) error {
	profilePath := filepath.Join(home, ".azure", "azureProfile.json")

	// Load existing profile or create new one.
	var profile map[string]interface{}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read Azure profile: %w", err)
		}
		profile = map[string]interface{}{
			"installationId": "",
			"subscriptions":  []interface{}{},
		}
	} else {
		// Strip UTF-8 BOM if present (Azure CLI sometimes writes files with BOM).
		data = stripBOM(data)

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
		if subID == subscriptionID {
			// Update existing subscription.
			sub["tenantId"] = tenantID
			sub["isDefault"] = true
			sub["state"] = "Enabled"
			sub["user"] = map[string]interface{}{
				"name": username,
				"type": "user",
			}
			sub["environmentName"] = "AzureCloud"
			subscriptionsRaw[i] = sub
			found = true
		} else {
			// Mark other subscriptions as not default.
			sub["isDefault"] = false
			subscriptionsRaw[i] = sub
		}
	}

	// Add new subscription if not found.
	if !found && subscriptionID != "" {
		newSub := map[string]interface{}{
			"id":              subscriptionID,
			"name":            subscriptionID,
			"tenantId":        tenantID,
			"isDefault":       true,
			"state":           "Enabled",
			"environmentName": "AzureCloud",
			"user": map[string]interface{}{
				"name": username,
				"type": "user",
			},
		}
		subscriptionsRaw = append(subscriptionsRaw, newSub)
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

	log.Debug("Updated Azure profile", "path", profilePath, "subscription", subscriptionID)
	return nil
}

// extractOIDFromToken extracts the user OID from a JWT token.
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

// extractUsernameFromToken extracts the username from a JWT token.
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

	return "", fmt.Errorf("no username claim found in token")
}

// extractUsernameOrFallback extracts username from token or returns fallback.
func extractUsernameOrFallback(token string) string {
	username, err := extractUsernameFromToken(token)
	if err != nil {
		return "user@unknown"
	}
	return username
}

// extractJWTClaims decodes a JWT token and returns the claims.
func extractJWTClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	return claims, nil
}

// stripBOM removes UTF-8 BOM (Byte Order Mark) from the beginning of data.
// Azure CLI sometimes writes JSON files with BOM which causes JSON parsing to fail.
func stripBOM(data []byte) []byte {
	// UTF-8 BOM is EF BB BF.
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}
