package azure

import (
	"encoding/json"
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// CopyAtmosRefreshTokensInto copies refresh-token entries for the given account
// from the Atmos realm MSAL cache into an Azure CLI MSAL cache map.
//
// The Atmos providers authenticate with the Azure CLI public client, so the
// refresh token MSAL persists in the realm cache is directly usable by az.
// Seeding it lets az self-mint tokens for ANY audience (including the legacy
// ARM audience azidentity/azapi request) and survive access-token expiry —
// without it, az fails with "Can't find token from MSAL cache" as soon as a
// lookup misses the seeded access tokens.
func CopyAtmosRefreshTokensInto(azCache map[string]interface{}, home, realm, homeAccountID string) {
	if realm == "" || homeAccountID == "" {
		// Without a realm the Atmos cache path IS the az cache (self-copy);
		// without a home account ID entries can't be matched safely.
		return
	}

	source := loadAtmosRefreshTokens(home, realm)
	if len(source) == 0 {
		return
	}

	dest, ok := azCache["RefreshToken"].(map[string]interface{})
	if !ok {
		dest = map[string]interface{}{}
		azCache["RefreshToken"] = dest
	}

	if copied := copyMatchingRefreshTokens(dest, source, homeAccountID); copied > 0 {
		log.Debug("Copied refresh tokens into Azure CLI cache", "count", copied)
	}
}

// loadAtmosRefreshTokens reads the RefreshToken section of the Atmos realm
// MSAL cache; missing or unparsable caches are non-fatal (empty result).
func loadAtmosRefreshTokens(home, realm string) map[string]interface{} {
	atmosCachePath := filepath.Join(home, ".azure", "atmos", realm, "msal_token_cache.json")
	data, err := os.ReadFile(atmosCachePath)
	if err != nil {
		log.Debug("No Atmos MSAL cache to copy refresh tokens from", "path", atmosCachePath, "error", err)
		return nil
	}

	var atmosCache map[string]interface{}
	if err := json.Unmarshal(data, &atmosCache); err != nil {
		log.Debug("Failed to parse Atmos MSAL cache", "error", err)
		return nil
	}

	source, _ := atmosCache["RefreshToken"].(map[string]interface{})
	return source
}

// copyMatchingRefreshTokens copies entries whose home_account_id matches.
func copyMatchingRefreshTokens(dest, source map[string]interface{}, homeAccountID string) int {
	copied := 0
	for key, raw := range source {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if hid, _ := entry["home_account_id"].(string); hid != homeAccountID {
			continue
		}
		dest[key] = entry
		copied++
	}
	return copied
}
