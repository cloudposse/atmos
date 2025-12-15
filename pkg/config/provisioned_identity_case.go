package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/auth/provisioning"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// Log field name constant for provider.
const logFieldProvider = "provider"

// YAML key constants.
const yamlKeyIdentities = "identities"

// preserveProvisionedIdentityCase extracts original case identity names from provisioned identity cache files.
// Provisioned identities are stored in cache files with original case (e.g., "core-artifacts/AdministratorAccess"),
// but when loaded via Viper, the keys are lowercased. This function reads the raw YAML to preserve the original case.
//
// This function scans the cache directory for all provider subdirectories that have cache files,
// rather than relying on config flags. This ensures case is preserved regardless of whether
// auto_provision_identities is set in the current config. The identities themselves are loaded
// separately via imports - this only affects the display case mapping.
func preserveProvisionedIdentityCase(atmosConfig *schema.AtmosConfiguration) error {
	// Get XDG cache directory for provisioned identities.
	const authSubDir = "auth"
	const authDirPerms = 0o700
	baseProvisioningDir, err := xdg.GetXDGCacheDir(authSubDir, authDirPerms)
	if err != nil {
		return fmt.Errorf("failed to get provisioning cache directory: %w", err)
	}

	// Ensure case map exists.
	if atmosConfig.Auth.IdentityCaseMap == nil {
		atmosConfig.Auth.IdentityCaseMap = make(map[string]string)
	}

	// Scan cache directory for all provider subdirectories that have cache files.
	// This reads case mappings regardless of config flags - the identities themselves
	// are loaded separately via imports, this only affects display case.
	entries, err := os.ReadDir(baseProvisioningDir)
	if err != nil {
		// No cache directory or can't read it - nothing to preserve.
		log.Debug("Cannot read provisioning cache directory", "path", baseProvisioningDir, "error", err)
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		providerName := entry.Name()
		cacheFile := filepath.Join(baseProvisioningDir, providerName, provisioning.ProvisionedFileName)
		preserveProviderIdentityCase(atmosConfig, providerName, cacheFile)
	}

	return nil
}

// preserveProviderIdentityCase extracts identity names from a single provider's cache file.
func preserveProviderIdentityCase(atmosConfig *schema.AtmosConfiguration, providerName, cacheFile string) {
	// Check if cache file exists.
	if _, err := os.Stat(cacheFile); err != nil {
		return // No cache file for this provider.
	}

	// Read raw YAML to get original case identity names.
	rawYAML, err := os.ReadFile(cacheFile)
	if err != nil {
		log.Debug("Failed to read provisioned identities cache", logFieldProvider, providerName, "error", err)
		return
	}

	// Parse and extract identities from the cache file.
	identities := extractIdentitiesFromYAML(rawYAML, providerName)
	if identities == nil {
		return
	}

	// Add to case map (don't overwrite user-defined case - user config takes precedence).
	count := 0
	for originalName := range identities {
		lowercaseName := strings.ToLower(originalName)
		// Only add if not already defined (user config takes precedence).
		if _, exists := atmosConfig.Auth.IdentityCaseMap[lowercaseName]; !exists {
			atmosConfig.Auth.IdentityCaseMap[lowercaseName] = originalName
			count++
		}
	}

	if count > 0 {
		log.Debug("Preserved provisioned identity case mapping", logFieldProvider, providerName, "identities", count)
	}
}

// extractIdentitiesFromYAML parses YAML and extracts the auth.identities section.
func extractIdentitiesFromYAML(rawYAML []byte, providerName string) map[string]interface{} {
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(rawYAML, &rawConfig); err != nil {
		log.Debug("Failed to parse provisioned identities YAML", logFieldProvider, providerName, "error", err)
		return nil
	}

	authSection, ok := rawConfig["auth"].(map[string]interface{})
	if !ok {
		return nil
	}

	identitiesSection, ok := authSection[yamlKeyIdentities].(map[string]interface{})
	if !ok {
		return nil
	}

	return identitiesSection
}
