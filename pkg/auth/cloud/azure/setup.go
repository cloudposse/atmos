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
	if !ok || azureCreds == nil {
		return nil // No Azure credentials to setup.
	}

	// Validate credentials are not expired.
	if azureCreds.IsExpired() {
		return fmt.Errorf("%w: Azure credentials are expired", errUtils.ErrAuthenticationFailed)
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
		// OIDC-specific fields for Terraform ARM_USE_OIDC support.
		UseOIDC:       azureCreds.IsServicePrincipal,
		ClientID:      azureCreds.ClientID,
		TokenFilePath: azureCreds.TokenFilePath,
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
	// Pass OIDC fields from auth context for Terraform ARM_USE_OIDC support.
	environMap = PrepareEnvironment(PrepareEnvironmentConfig{
		Environ:        environMap,
		SubscriptionID: azureAuth.SubscriptionID,
		TenantID:       azureAuth.TenantID,
		Location:       azureAuth.Location,
		UseOIDC:        azureAuth.UseOIDC,
		ClientID:       azureAuth.ClientID,
		TokenFilePath:  azureAuth.TokenFilePath,
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
		// For service principal, use client ID as username.
		if azureCreds.IsServicePrincipal && azureCreds.ClientID != "" {
			username = azureCreds.ClientID
		} else {
			username = "user@unknown" // Fallback username.
		}
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
		ClientID:           azureCreds.ClientID,
		IsServicePrincipal: azureCreds.IsServicePrincipal,
	}); err != nil {
		log.Debug("Failed to update MSAL cache", "error", err)
		// Continue to try azureProfile update.
	}

	// Update azureProfile.json.
	if err := updateAzureProfile(home, username, tenantID, subscriptionID, azureCreds.IsServicePrincipal); err != nil {
		log.Debug("Failed to update Azure profile", "error", err)
		// Non-fatal.
	}

	// For service principal auth, also update service_principal_entries.json.
	// This allows Azure CLI commands to work with OIDC tokens during the CI workflow.
	if azureCreds.IsServicePrincipal && azureCreds.ClientID != "" && azureCreds.FederatedToken != "" {
		if err := updateServicePrincipalEntries(home, azureCreds.ClientID, tenantID, azureCreds.FederatedToken); err != nil {
			log.Debug("Failed to update service principal entries", "error", err)
			// Non-fatal.
		}
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
	// ClientID is set for service principal authentication (OIDC).
	ClientID string
	// IsServicePrincipal indicates this is service principal auth.
	// Service principal tokens use a different MSAL cache format:
	// - home_account_id is empty (cache key starts with "-")
	// - No Account entry is created
	// - AppMetadata entry is added
	// Reference: https://github.com/AzureAD/microsoft-authentication-library-for-python
	IsServicePrincipal bool
}

// updateMSALCache updates the Azure CLI MSAL token cache.
func updateMSALCache(params *msalCacheUpdate) error {
	msalCachePath := filepath.Join(params.Home, ".azure", "msal_token_cache.json")

	// Load existing cache or create new one.
	cache, err := loadMSALCache(msalCachePath)
	if err != nil {
		return err
	}

	// Initialize cache sections and populate with tokens.
	sections := initializeCacheSections(cache)

	if params.IsServicePrincipal {
		// Service principal uses different MSAL cache format.
		addServicePrincipalTokens(sections, params)
	} else {
		// User authentication uses standard format.
		addUserAccountAndTokens(sections, params)
	}

	// Write updated cache.
	updatedData, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MSAL cache: %w", err)
	}

	return writeMSALCacheToFile(msalCachePath, updatedData)
}

// msalCacheSections holds all MSAL cache sections.
type msalCacheSections struct {
	accessToken map[string]interface{}
	account     map[string]interface{}
	appMetadata map[string]interface{}
}

// initializeCacheSections ensures all MSAL cache sections exist.
func initializeCacheSections(cache map[string]interface{}) *msalCacheSections {
	sections := &msalCacheSections{}

	// Ensure AccessToken section exists.
	if at, ok := cache[FieldAccessToken].(map[string]interface{}); ok {
		sections.accessToken = at
	} else {
		sections.accessToken = make(map[string]interface{})
		cache[FieldAccessToken] = sections.accessToken
	}

	// Ensure Account section exists.
	if acc, ok := cache["Account"].(map[string]interface{}); ok {
		sections.account = acc
	} else {
		sections.account = make(map[string]interface{})
		cache["Account"] = sections.account
	}

	// Ensure AppMetadata section exists (used for service principal auth).
	if am, ok := cache["AppMetadata"].(map[string]interface{}); ok {
		sections.appMetadata = am
	} else {
		sections.appMetadata = make(map[string]interface{})
		cache["AppMetadata"] = sections.appMetadata
	}

	return sections
}

// msalIdentifiers holds common MSAL cache identifiers.
type msalIdentifiers struct {
	homeAccountID string
	environment   string
	clientID      string
	realm         string
}

// addUserAccountAndTokens adds account entry and tokens for user authentication.
func addUserAccountAndTokens(sections *msalCacheSections, params *msalCacheUpdate) {
	// Create common MSAL identifiers for user auth.
	ids := msalIdentifiers{
		homeAccountID: fmt.Sprintf("%s.%s", params.UserOID, params.TenantID),
		environment:   "login.microsoftonline.com",
		clientID:      "04b07795-8ddb-461a-bbee-02f9e1bf7b46", // Azure CLI public client.
		realm:         params.TenantID,
	}

	// Add Account entry (required for azuread provider).
	accountKey := fmt.Sprintf("%s-%s-%s", ids.homeAccountID, ids.environment, ids.realm)
	accountEntry := map[string]interface{}{
		FieldHomeAccountID: ids.homeAccountID,
		FieldEnvironment:   ids.environment,
		FieldRealm:         ids.realm,
		"local_account_id": params.UserOID,
		"username":         extractUsernameOrFallback(params.AccessToken),
		"authority_type":   "MSSTS",
		"account_source":   "device_code",
	}
	sections.account[accountKey] = accountEntry

	// Add management API token.
	addTokenToCache(sections.accessToken, &tokenCacheParams{
		Token:         params.AccessToken,
		Expiration:    params.Expiration,
		Scope:         "https://management.azure.com/.default",
		HomeAccountID: ids.homeAccountID,
		Environment:   ids.environment,
		ClientID:      ids.clientID,
		Realm:         ids.realm,
		APIName:       "Management API",
	})

	// Add optional Graph and KeyVault tokens.
	addOptionalTokens(sections.accessToken, params, ids)
}

// addServicePrincipalTokens adds tokens for service principal (OIDC) authentication.
// Service principal auth uses a different MSAL cache format per the MSAL Python reference:
// - home_account_id is empty (cache key starts with "-")
// - No Account entry is created
// - AppMetadata entry is added
// Reference: https://github.com/AzureAD/microsoft-authentication-library-for-python/blob/dev/msal/token_cache.py
func addServicePrincipalTokens(sections *msalCacheSections, params *msalCacheUpdate) {
	environment := "login.microsoftonline.com"

	// For service principal, home_account_id is empty.
	// This results in cache keys starting with "-".
	ids := msalIdentifiers{
		homeAccountID: "", // Empty for service principal.
		environment:   environment,
		clientID:      params.ClientID,
		realm:         params.TenantID,
	}

	// Add AppMetadata entry (required for service principal).
	// Format: appmetadata-{environment}-{client_id} (lowercase).
	appMetadataKey := fmt.Sprintf("appmetadata-%s-%s", strings.ToLower(environment), strings.ToLower(params.ClientID))
	sections.appMetadata[appMetadataKey] = map[string]interface{}{
		FieldEnvironment: environment,
		"client_id":      params.ClientID,
		"family_id":      "", // Empty for non-FOCI apps.
	}
	log.Debug("Added AppMetadata entry to MSAL cache", LogFieldKey, appMetadataKey)

	// Add management API token.
	// For service principal, the cache key format is:
	// -{environment}-accesstoken-{client_id}-{realm}-{target} (all lowercase).
	addTokenToCache(sections.accessToken, &tokenCacheParams{
		Token:         params.AccessToken,
		Expiration:    params.Expiration,
		Scope:         "https://management.azure.com/.default",
		HomeAccountID: ids.homeAccountID,
		Environment:   ids.environment,
		ClientID:      ids.clientID,
		Realm:         ids.realm,
		APIName:       "Management API",
	})

	// Add optional Graph and KeyVault tokens.
	addOptionalTokens(sections.accessToken, params, ids)
}

// addOptionalTokens adds Graph and KeyVault tokens if available.
func addOptionalTokens(accessTokenSection map[string]interface{}, params *msalCacheUpdate, ids msalIdentifiers) {
	// Add Microsoft Graph API token if available.
	if params.GraphToken != "" {
		addTokenToCache(accessTokenSection, &tokenCacheParams{
			Token:         params.GraphToken,
			Expiration:    params.GraphExpiration,
			Scope:         "https://graph.microsoft.com/.default",
			HomeAccountID: ids.homeAccountID,
			Environment:   ids.environment,
			ClientID:      ids.clientID,
			Realm:         ids.realm,
			APIName:       "Graph API",
		})
	} else {
		log.Debug("No Graph API token available, azuread provider may not work")
	}

	// Add Azure KeyVault API token if available.
	if params.KeyVaultToken != "" {
		addTokenToCache(accessTokenSection, &tokenCacheParams{
			Token:         params.KeyVaultToken,
			Expiration:    params.KeyVaultExpiration,
			Scope:         "https://vault.azure.net/.default",
			HomeAccountID: ids.homeAccountID,
			Environment:   ids.environment,
			ClientID:      ids.clientID,
			Realm:         ids.realm,
			APIName:       "KeyVault API",
		})
	} else {
		log.Debug("No KeyVault API token available, KeyVault operations may not work")
	}
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

// writeMSALCacheToFile writes MSAL cache data to file with locking.
func writeMSALCacheToFile(msalCachePath string, data []byte) error {
	// Ensure .azure directory exists.
	azureDir := filepath.Dir(msalCachePath)
	if err := os.MkdirAll(azureDir, DirPermissions); err != nil {
		return fmt.Errorf("failed to create .azure directory: %w", err)
	}

	// Acquire file lock to prevent concurrent writes.
	lockPath := msalCachePath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock MSAL cache file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	if err := os.WriteFile(msalCachePath, data, FilePermissions); err != nil {
		return fmt.Errorf("failed to write MSAL cache: %w", err)
	}

	log.Debug("Updated Azure CLI MSAL token cache", "path", msalCachePath)
	return nil
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
func updateAzureProfile(home, username, tenantID, subscriptionID string, isServicePrincipal bool) error {
	profilePath := filepath.Join(home, ".azure", "azureProfile.json")
	azureDir := filepath.Dir(profilePath)
	if err := os.MkdirAll(azureDir, DirPermissions); err != nil {
		return fmt.Errorf("failed to create .azure directory: %w", err)
	}

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
	profile["subscriptions"] = UpdateSubscriptionsInProfile(profile, username, tenantID, subscriptionID, isServicePrincipal)

	// Write updated profile.
	updatedData, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Azure profile: %w", err)
	}

	// Acquire file lock to prevent concurrent writes.
	lockPath := profilePath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock Azure profile file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	if err := os.WriteFile(profilePath, updatedData, FilePermissions); err != nil {
		return fmt.Errorf("failed to write Azure profile: %w", err)
	}

	log.Debug("Updated Azure profile", "path", profilePath, "subscription", subscriptionID)
	return nil
}

// UpdateSubscriptionsInProfile updates the subscriptions array in an Azure profile.
// It sets the specified subscription as default and marks all others as not default.
func UpdateSubscriptionsInProfile(profile map[string]interface{}, username, tenantID, subscriptionID string, isServicePrincipal bool) []interface{} {
	// Get subscriptions array.
	subscriptionsRaw, ok := profile["subscriptions"].([]interface{})
	if !ok {
		subscriptionsRaw = []interface{}{}
	}

	// Determine user type based on authentication method.
	userType := "user"
	if isServicePrincipal {
		userType = "servicePrincipal"
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
				"type": userType,
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
				"type": userType,
			},
		}
		subscriptionsRaw = append(subscriptionsRaw, newSub)
	}

	return subscriptionsRaw
}

// updateServicePrincipalEntries updates the service_principal_entries.json file for Azure CLI.
// This enables Azure CLI commands to work with OIDC tokens during CI workflows.
// Field names match Azure CLI's ServicePrincipalStore: client_id, tenant, client_assertion.
// Reference: https://github.com/Azure/azure-cli/blob/main/src/azure-cli-core/azure/cli/core/auth/identity.py
func updateServicePrincipalEntries(home, clientID, tenantID, federatedToken string) error {
	entriesPath := filepath.Join(home, ".azure", "service_principal_entries.json")
	azureDir := filepath.Dir(entriesPath)
	if err := os.MkdirAll(azureDir, DirPermissions); err != nil {
		return fmt.Errorf("failed to create .azure directory: %w", err)
	}

	// Load existing entries or create new array.
	var entries []map[string]interface{}
	data, err := os.ReadFile(entriesPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read service principal entries: %w", err)
		}
		entries = []map[string]interface{}{}
	} else {
		// Strip UTF-8 BOM if present.
		data = stripBOM(data)

		if err := json.Unmarshal(data, &entries); err != nil {
			// If file is corrupted, start fresh.
			log.Debug("Failed to parse service principal entries, creating new file", "error", err)
			entries = []map[string]interface{}{}
		}
	}

	// Find or create entry for this service principal.
	// Azure CLI looks up entries by client_id field.
	var found bool
	for i, entry := range entries {
		if cid, ok := entry["client_id"].(string); ok && cid == clientID {
			// Update existing entry.
			entries[i] = createServicePrincipalEntry(clientID, tenantID, federatedToken)
			found = true
			break
		}
	}

	if !found {
		// Add new entry.
		entries = append(entries, createServicePrincipalEntry(clientID, tenantID, federatedToken))
	}

	// Write updated entries.
	updatedData, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal service principal entries: %w", err)
	}

	// Acquire file lock to prevent concurrent writes.
	lockPath := entriesPath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock service principal entries file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	if err := os.WriteFile(entriesPath, updatedData, FilePermissions); err != nil {
		return fmt.Errorf("failed to write service principal entries: %w", err)
	}

	log.Debug("Updated Azure CLI service principal entries", "path", entriesPath, "client_id", clientID)
	return nil
}

// createServicePrincipalEntry creates a service principal entry for OIDC authentication.
// Field names match Azure CLI's ServicePrincipalStore constants:
// - _CLIENT_ID = 'client_id'
// - _TENANT = 'tenant'
// - _CLIENT_ASSERTION = 'client_assertion'
func createServicePrincipalEntry(clientID, tenantID, federatedToken string) map[string]interface{} {
	return map[string]interface{}{
		"client_id":        clientID,
		"tenant":           tenantID,
		"client_assertion": federatedToken,
	}
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
