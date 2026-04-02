// Package awssso implements the AWS SSO migration steps and factory.
package awssso

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/migrate"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewAWSSSOSteps returns the ordered list of migration steps for AWS SSO migration.
func NewAWSSSOSteps(migCtx *migrate.MigrationContext, fs migrate.FileSystem) []migrate.MigrationStep {
	defer perf.Track(nil, "awssso.NewAWSSSOSteps")()

	return []migrate.MigrationStep{
		NewDetectPrerequisites(migCtx, fs),
		NewConfigureProvider(migCtx, fs),
		NewGenerateProfiles(migCtx, fs),
		NewUpdateStackDefaults(migCtx, fs),
		NewUpdateTfstateBackend(migCtx, fs),
		NewCleanupLegacyAuth(migCtx, fs),
	}
}

// BuildMigrationContext loads atmos config and discovers SSO/account-map files.
func BuildMigrationContext(ctx context.Context, fs migrate.FileSystem, prompter migrate.Prompter) (*migrate.MigrationContext, error) {
	defer perf.Track(nil, "awssso.BuildMigrationContext")()

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitConfig, err)
	}

	migCtx := &migrate.MigrationContext{
		AtmosConfig:     &atmosConfig,
		StacksBasePath:  atmosConfig.Stacks.BasePath,
		ProfilesPath:    "profiles",
		ExistingAuth:    &atmosConfig.Auth,
		AtmosConfigPath: atmosConfig.CliConfigPath,
	}

	// Discover account map.
	migCtx.AccountMap, err = discoverAccountMap(migCtx.StacksBasePath, fs, prompter)
	if err != nil {
		return nil, err
	}

	// Discover SSO config.
	migCtx.SSOConfig, err = discoverSSOConfig(migCtx.StacksBasePath, fs, prompter)
	if err != nil {
		return nil, err
	}

	return migCtx, nil
}

// accountMapSearchPaths returns the candidate paths for account-map.yaml discovery.
func accountMapSearchPaths(base string) []string {
	return []string{
		filepath.Join(base, "mixins", "account-map.yaml"),
		filepath.Join(base, "catalog", "account-map.yaml"),
		filepath.Join(base, "catalog", "account-map", "account-map.yaml"),
	}
}

// ssoConfigSearchPaths returns the candidate paths for aws-sso.yaml discovery.
func ssoConfigSearchPaths(base string) []string {
	return []string{
		filepath.Join(base, "catalog", "aws-sso.yaml"),
		filepath.Join(base, "catalog", "aws-sso", "aws-sso.yaml"),
	}
}

// discoverAccountMap searches known paths for account-map.yaml and parses it.
func discoverAccountMap(base string, fs migrate.FileSystem, prompter migrate.Prompter) (map[string]string, error) {
	defer perf.Track(nil, "awssso.discoverAccountMap")()

	found := findExistingFiles(accountMapSearchPaths(base), fs)

	filePath, err := resolveFilePath(found, prompter, "account-map.yaml")
	if err != nil {
		return nil, err
	}

	data, err := fs.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading account map file %s: %w", filePath, err)
	}

	return parseAccountMap(data)
}

// discoverSSOConfig searches known paths for aws-sso.yaml and parses it.
func discoverSSOConfig(base string, fs migrate.FileSystem, prompter migrate.Prompter) (*migrate.SSOConfig, error) {
	defer perf.Track(nil, "awssso.discoverSSOConfig")()

	found := findExistingFiles(ssoConfigSearchPaths(base), fs)

	filePath, err := resolveFilePath(found, prompter, "aws-sso.yaml")
	if err != nil {
		return nil, err
	}

	data, err := fs.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading SSO config file %s: %w", filePath, err)
	}

	ssoCfg, err := parseSSOConfig(data)
	if err != nil {
		return nil, err
	}

	// Prompt for SSO start URL if not found in the file.
	if ssoCfg.StartURL == "" {
		url, promptErr := prompter.Input("Enter your AWS SSO start URL", "")
		if promptErr != nil {
			return nil, fmt.Errorf("prompting for SSO start URL: %w", promptErr)
		}
		ssoCfg.StartURL = url
	}

	// Prompt for SSO region if not found in the file.
	if ssoCfg.Region == "" {
		region, promptErr := prompter.Input("Enter your AWS SSO region", "us-east-1")
		if promptErr != nil {
			return nil, fmt.Errorf("prompting for SSO region: %w", promptErr)
		}
		ssoCfg.Region = region
	}

	// Prompt for provider name with a default.
	providerName, err := prompter.Input("Enter SSO provider name", "sso")
	if err != nil {
		return nil, fmt.Errorf("prompting for provider name: %w", err)
	}
	ssoCfg.ProviderName = providerName

	return ssoCfg, nil
}

// findExistingFiles returns the subset of paths that exist on the filesystem.
func findExistingFiles(paths []string, fs migrate.FileSystem) []string {
	var found []string
	for _, p := range paths {
		if fs.Exists(p) {
			found = append(found, p)
		}
	}
	return found
}

// resolveFilePath picks a single file path from candidates or prompts the user.
func resolveFilePath(found []string, prompter migrate.Prompter, fileName string) (string, error) {
	switch len(found) {
	case 0:
		path, err := prompter.Input(fmt.Sprintf("Could not find %s. Enter the path", fileName), "")
		if err != nil {
			return "", fmt.Errorf("prompting for %s path: %w", fileName, err)
		}
		if path == "" {
			return "", fmt.Errorf("no path provided for %s", fileName)
		}
		return path, nil
	case 1:
		return found[0], nil
	default:
		selected, err := prompter.Select(fmt.Sprintf("Multiple %s files found. Select one", fileName), found)
		if err != nil {
			return "", fmt.Errorf("selecting %s: %w", fileName, err)
		}
		return selected, nil
	}
}

// parseAccountMap extracts account name-to-ID mappings from account-map YAML content.
// It looks for vars.full_account_map first, then falls back to vars.account_map.
func parseAccountMap(data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("account map file is empty")
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing account map YAML: %w", err)
	}

	vars, ok := raw["vars"]
	if !ok {
		return nil, fmt.Errorf("account map YAML missing 'vars' key")
	}

	varsMap, ok := vars.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("account map 'vars' is not a map")
	}

	// Try full_account_map first, then account_map.
	var accountData interface{}
	for _, key := range []string{"full_account_map", "account_map"} {
		if v, exists := varsMap[key]; exists {
			accountData = v
			break
		}
	}

	if accountData == nil {
		return nil, fmt.Errorf("account map YAML missing 'vars.full_account_map' or 'vars.account_map'")
	}

	accountMap, ok := accountData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("account map data is not a map")
	}

	result := make(map[string]string, len(accountMap))
	// Sort keys for deterministic output.
	keys := make([]string, 0, len(accountMap))
	for k := range accountMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		result[k] = fmt.Sprintf("%v", accountMap[k])
	}

	return result, nil
}

// parseSSOConfig extracts SSO configuration from aws-sso YAML content.
// It reads vars.account_assignments and optionally vars.start_url and vars.region.
func parseSSOConfig(data []byte) (*migrate.SSOConfig, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("SSO config file is empty")
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing SSO config YAML: %w", err)
	}

	vars, ok := raw["vars"]
	if !ok {
		return nil, fmt.Errorf("SSO config YAML missing 'vars' key")
	}

	varsMap, ok := vars.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("SSO config 'vars' is not a map")
	}

	ssoCfg := &migrate.SSOConfig{}

	// Extract optional start_url and region.
	if v, exists := varsMap["start_url"]; exists {
		ssoCfg.StartURL = fmt.Sprintf("%v", v)
	}
	if v, exists := varsMap["region"]; exists {
		ssoCfg.Region = fmt.Sprintf("%v", v)
	}

	// Parse account_assignments.
	assignData, ok := varsMap["account_assignments"]
	if !ok {
		// No assignments is valid; return config with empty assignments.
		ssoCfg.AccountAssignments = make(map[string]map[string][]string)
		return ssoCfg, nil
	}

	assignMap, ok := assignData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("account_assignments is not a map")
	}

	ssoCfg.AccountAssignments = make(map[string]map[string][]string, len(assignMap))

	for group, permData := range assignMap {
		permMap, ok := permData.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("account_assignments[%s] is not a map", group)
		}

		ssoCfg.AccountAssignments[group] = make(map[string][]string, len(permMap))

		for perm, accountsData := range permMap {
			accountsList, ok := accountsData.([]interface{})
			if !ok {
				return nil, fmt.Errorf("account_assignments[%s][%s] is not a list", group, perm)
			}

			accounts := make([]string, 0, len(accountsList))
			for _, a := range accountsList {
				accounts = append(accounts, fmt.Sprintf("%v", a))
			}
			ssoCfg.AccountAssignments[group][perm] = accounts
		}
	}

	return ssoCfg, nil
}
