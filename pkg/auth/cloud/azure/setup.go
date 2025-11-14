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

	// Use shared PrepareEnvironment helper to get properly configured environment.
	environMap = PrepareEnvironment(PrepareEnvironmentConfig{
		Environ:        environMap,
		SubscriptionID: azureAuth.SubscriptionID,
		TenantID:       azureAuth.TenantID,
		Location:       azureAuth.Location,
	})

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
	if err := updateMSALCache(&msalCacheUpdate{
		Home:               home,
		AccessToken:        azureCreds.AccessToken,
		Expiration:         azureCreds.Expiration,
		GraphToken:         graphToken,
		GraphExpiration:    graphExpiration,
		KeyVaultToken:      keyVaultToken,
		KeyVaultExpiration: keyVaultExpiration,
		UserOID:            userOID,
		TenantID:           tenantID,
	}); err != nil {
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

// msalCacheUpdate holds parameters for updating MSAL cache.
type msalCacheUpdate struct {
	Home               string
	AccessToken        string
	Expiration         string
	GraphToken         string
	GraphExpiration    string
	KeyVaultToken      string
	KeyVaultExpiration string
	UserOID            string
	TenantID           string
}

// updateMSALCache updates the Azure CLI MSAL token cache.
func updateMSALCache(params *msalCacheUpdate) error {
	home := params.Home
	accessToken := params.AccessToken
	expiration := params.Expiration
	graphToken := params.GraphToken
	graphExpiration := params.GraphExpiration
	keyVaultToken := params.KeyVaultToken
	keyVaultExpiration := params.KeyVaultExpiration
	userOID := params.UserOID
	tenantID := params.TenantID
	msalCachePath := filepath.Join(home, ".azure", "msal_token_cache.json")

	// Load existing cache or create new one.
	cache, err := loadMSALCache(msalCachePath)
	if err != nil {
		return err
	}

	// Ensure AccessToken section exists.
	accessTokenSection, ok := cache[FieldAccessToken].(map[string]interface{})
	if !ok {
		accessTokenSection = make(map[string]interface{})
		cache[FieldAccessToken] = accessTokenSection
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
		FieldHomeAccountID: homeAccountID,
		FieldEnvironment:   environment,
		FieldRealm:         realm,
		"local_account_id": userOID,
		"username":         extractUsernameOrFallback(accessToken),
		"authority_type":   "MSSTS",
		"account_source":   "device_code",
	}
	accountSection[accountKey] = accountEntry

	// Add management API token entry.
	addTokenToCache(accessTokenSection, &tokenCacheParams{
		Token:         accessToken,
		Expiration:    expiration,
		Scope:         "https://management.azure.com/.default https://management.azure.com/user_impersonation",
		HomeAccountID: homeAccountID,
		Environment:   environment,
		ClientID:      clientID,
		Realm:         realm,
		APIName:       "Management API",
	})

	// Add entry for Microsoft Graph API (used by azuread provider) if available.
	if graphToken != "" {
		addTokenToCache(accessTokenSection, &tokenCacheParams{
			Token:         graphToken,
			Expiration:    graphExpiration,
			Scope:         "https://graph.microsoft.com/.default",
			HomeAccountID: homeAccountID,
			Environment:   environment,
			ClientID:      clientID,
			Realm:         realm,
			APIName:       "Graph API",
		})
	} else {
		log.Debug("No Graph API token available, azuread provider may not work")
	}

	// Add entry for Azure KeyVault API (used by azurerm provider for KeyVault operations) if available.
	if keyVaultToken != "" {
		addTokenToCache(accessTokenSection, &tokenCacheParams{
			Token:         keyVaultToken,
			Expiration:    keyVaultExpiration,
			Scope:         "https://vault.azure.net/.default",
			HomeAccountID: homeAccountID,
			Environment:   environment,
			ClientID:      clientID,
			Realm:         realm,
			APIName:       "KeyVault API",
		})
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
	if err := os.MkdirAll(azureDir, DirPermissions); err != nil {
		return fmt.Errorf("failed to create .azure directory: %w", err)
	}

	if err := os.WriteFile(msalCachePath, updatedData, FilePermissions); err != nil {
		return fmt.Errorf("failed to write MSAL cache: %w", err)
	}

	log.Debug("Updated Azure CLI MSAL token cache", "path", msalCachePath)
	return nil
}

// loadMSALCache loads existing MSAL cache from file or creates a new empty cache.
func loadMSALCache(msalCachePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(msalCachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read MSAL cache: %w", err)
		}
		return make(map[string]interface{}), nil
	}

	var cache map[string]interface{}
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse MSAL cache: %w", err)
	}

	return cache, nil
}

// tokenCacheParams holds parameters for adding a token to MSAL cache.
type tokenCacheParams struct {
	Token         string
	Expiration    string
	Scope         string
	HomeAccountID string
	Environment   string
	ClientID      string
	Realm         string
	APIName       string
}

// addTokenToCache adds a token entry to the MSAL cache section.
func addTokenToCache(accessTokenSection map[string]interface{}, params *tokenCacheParams) {
	// Parse token expiration.
	expiresAt, err := time.Parse(time.RFC3339, params.Expiration)
	if err != nil {
		log.Debug("Failed to parse "+params.APIName+" token expiration, skipping cache", "error", err)
		return
	}

	cacheKey := fmt.Sprintf("%s-%s-accesstoken-%s-%s-%s",
		params.HomeAccountID, params.Environment, params.ClientID, params.Realm, params.Scope)

	cachedAt := time.Now().Unix()
	expiresOn := expiresAt.Unix()

	tokenEntry := map[string]interface{}{
		"credential_type":     FieldAccessToken,
		"secret":              params.Token,
		"home_account_id":     params.HomeAccountID,
		"environment":         params.Environment,
		"client_id":           params.ClientID,
		"target":              params.Scope,
		"realm":               params.Realm,
		"token_type":          "Bearer",
		"cached_at":           fmt.Sprintf(IntFormat, cachedAt),
		"expires_on":          fmt.Sprintf(IntFormat, expiresOn),
		"extended_expires_on": fmt.Sprintf(IntFormat, expiresOn),
	}

	accessTokenSection[cacheKey] = tokenEntry
	log.Debug("Added "+params.APIName+" token to MSAL cache", "key", cacheKey)
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

	// Update subscriptions in profile.
	profile["subscriptions"] = UpdateSubscriptionsInProfile(profile, username, tenantID, subscriptionID)

	// Write updated profile.
	updatedData, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Azure profile: %w", err)
	}

	if err := os.WriteFile(profilePath, updatedData, FilePermissions); err != nil {
		return fmt.Errorf("failed to write Azure profile: %w", err)
	}

	log.Debug("Updated Azure profile", "path", profilePath, "subscription", subscriptionID)
	return nil
}

// UpdateSubscriptionsInProfile updates the subscriptions array in an Azure profile.
// It sets the specified subscription as default and marks all others as not default.
func UpdateSubscriptionsInProfile(profile map[string]interface{}, username, tenantID, subscriptionID string) []interface{} {
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
			sub[FieldUser] = map[string]interface{}{
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
			FieldUser: map[string]interface{}{
				"name": username,
				"type": "user",
			},
		}
		subscriptionsRaw = append(subscriptionsRaw, newSub)
	}

	return subscriptionsRaw
}

// extractOIDFromToken extracts the user OID from a JWT token.
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

	return "", errUtils.ErrAzureUsernameClaimNotFound
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
		return nil, errUtils.ErrAzureInvalidJWTFormat
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
	if len(data) >= 3 && data[0] == BomMarker && data[1] == BomSecondByte && data[2] == BomThirdByte {
		return data[3:]
	}
	return data
}
