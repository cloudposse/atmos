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

	// CliConfigPath is the directory containing atmos.yaml, not the file itself.
	atmosConfigFile := filepath.Join(atmosConfig.CliConfigPath, "atmos.yaml")

	migCtx := &migrate.MigrationContext{
		AtmosConfig:     &atmosConfig,
		StacksBasePath:  atmosConfig.Stacks.BasePath,
		ProfilesPath:    "profiles",
		ExistingAuth:    &atmosConfig.Auth,
		AtmosConfigPath: atmosConfigFile,
	}

	// Discover account map.
	migCtx.AccountMap, err = discoverAccountMap(migCtx.StacksBasePath, fs, prompter)
	if err != nil {
		return nil, err
	}

	// Discover SSO config, using existing auth config as a source of defaults.
	migCtx.SSOConfig, err = discoverSSOConfig(migCtx.StacksBasePath, &atmosConfig.Auth, fs, prompter)
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
// If the file is not found or cannot be parsed, returns an empty map rather than failing.
// Individual steps will handle missing account data by prompting the user.
func discoverAccountMap(base string, fs migrate.FileSystem, prompter migrate.Prompter) (map[string]string, error) {
	defer perf.Track(nil, "awssso.discoverAccountMap")()

	found := findExistingFiles(accountMapSearchPaths(base), fs)
	if len(found) == 0 {
		// No account-map file found — return empty map.
		// Steps that need account data will prompt the user.
		return make(map[string]string), nil
	}

	filePath := found[0]
	if len(found) > 1 {
		selected, err := prompter.Select("Multiple account-map files found. Select one", found)
		if err != nil {
			return make(map[string]string), nil
		}
		filePath = selected
	}

	data, err := fs.ReadFile(filePath)
	if err != nil {
		// Cannot read file — return empty map gracefully.
		return make(map[string]string), nil
	}

	result, err := parseAccountMap(data)
	if err != nil {
		// File exists but format doesn't match — return empty map gracefully.
		// The file may use a different structure (e.g., component config vs stack vars).
		return make(map[string]string), nil
	}

	return result, nil
}

// discoverSSOConfig searches known paths for aws-sso.yaml and parses it.
// It also checks the existing auth config in atmos.yaml for SSO provider details
// (start_url, region, provider name) before prompting the user.
func discoverSSOConfig(base string, existingAuth *schema.AuthConfig, fs migrate.FileSystem, prompter migrate.Prompter) (*migrate.SSOConfig, error) {
	defer perf.Track(nil, "awssso.discoverSSOConfig")()

	ssoCfg := &migrate.SSOConfig{
		AccountAssignments: make(map[string]map[string][]string),
	}

	// First, extract defaults from existing auth config in atmos.yaml.
	if existingAuth != nil {
		for name, provider := range existingAuth.Providers {
			if provider.Kind == "aws/iam-identity-center" {
				ssoCfg.ProviderName = name
				ssoCfg.StartURL = provider.StartURL
				ssoCfg.Region = provider.Region
				break
			}
		}
	}

	// Then try to parse aws-sso.yaml for account assignments.
	found := findExistingFiles(ssoConfigSearchPaths(base), fs)
	if len(found) > 0 {
		filePath := found[0]
		if len(found) > 1 {
			selected, selectErr := prompter.Select("Multiple aws-sso.yaml files found. Select one", found)
			if selectErr == nil {
				filePath = selected
			}
		}

		data, readErr := fs.ReadFile(filePath)
		if readErr == nil {
			parsed, parseErr := parseSSOConfig(data)
			if parseErr == nil {
				// Merge: keep existing auth values if parsed file doesn't have them.
				if parsed.StartURL != "" {
					ssoCfg.StartURL = parsed.StartURL
				}
				if parsed.Region != "" {
					ssoCfg.Region = parsed.Region
				}
				if len(parsed.AccountAssignments) > 0 {
					ssoCfg.AccountAssignments = parsed.AccountAssignments
				}
			}
		}
	}

	// Only prompt for values still missing after checking both sources.
	if ssoCfg.StartURL == "" {
		url, promptErr := prompter.Input("Enter your AWS SSO start URL", "")
		if promptErr != nil {
			return nil, fmt.Errorf("prompting for SSO start URL: %w", promptErr)
		}
		ssoCfg.StartURL = url
	}

	if ssoCfg.Region == "" {
		region, promptErr := prompter.Input("Enter your AWS SSO region", "us-east-1")
		if promptErr != nil {
			return nil, fmt.Errorf("prompting for SSO region: %w", promptErr)
		}
		ssoCfg.Region = region
	}

	if ssoCfg.ProviderName == "" {
		providerName, err := prompter.Input("Enter SSO provider name", "sso")
		if err != nil {
			return nil, fmt.Errorf("prompting for provider name: %w", err)
		}
		ssoCfg.ProviderName = providerName
	}

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
// It searches multiple YAML paths for vars containing full_account_map or account_map:
//   - vars.full_account_map / vars.account_map (top-level)
//   - components.terraform.account-map.vars.full_account_map (component-level)
func parseAccountMap(data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("account map file is empty")
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing account map YAML: %w", err)
	}

	// Collect all candidate vars maps to search.
	varsCandidates := findVarsCandidates(raw, "account-map")

	// Search each candidate for account map data.
	for _, varsMap := range varsCandidates {
		for _, key := range []string{"full_account_map", "account_map"} {
			if v, exists := varsMap[key]; exists {
				accountMap, ok := v.(map[string]interface{})
				if !ok {
					continue
				}
				result := make(map[string]string, len(accountMap))
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
		}
	}

	return nil, fmt.Errorf("account map YAML missing 'vars.full_account_map' or 'vars.account_map'")
}

// parseSSOConfig extracts SSO configuration from aws-sso YAML content.
// It searches multiple YAML paths for vars containing account_assignments:
//   - vars.account_assignments (top-level)
//   - components.terraform.aws-sso.vars.account_assignments (component-level)
func parseSSOConfig(data []byte) (*migrate.SSOConfig, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("SSO config file is empty")
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing SSO config YAML: %w", err)
	}

	// Collect all candidate vars maps to search.
	varsCandidates := findVarsCandidates(raw, "aws-sso")

	ssoCfg := &migrate.SSOConfig{
		AccountAssignments: make(map[string]map[string][]string),
	}

	// Search each candidate for SSO-related vars.
	for _, varsMap := range varsCandidates {
		if v, exists := varsMap["start_url"]; exists && ssoCfg.StartURL == "" {
			ssoCfg.StartURL = fmt.Sprintf("%v", v)
		}
		if v, exists := varsMap["region"]; exists && ssoCfg.Region == "" {
			ssoCfg.Region = fmt.Sprintf("%v", v)
		}

		if assignData, exists := varsMap["account_assignments"]; exists && len(ssoCfg.AccountAssignments) == 0 {
			parsed, err := parseAccountAssignments(assignData)
			if err != nil {
				return nil, err
			}
			ssoCfg.AccountAssignments = parsed
		}
	}

	return ssoCfg, nil
}

// parseAccountAssignments parses the account_assignments structure:
// group -> permission-set -> []accounts.
func parseAccountAssignments(assignData interface{}) (map[string]map[string][]string, error) {
	assignMap, ok := assignData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("account_assignments is not a map")
	}

	result := make(map[string]map[string][]string, len(assignMap))

	for group, permData := range assignMap {
		permMap, ok := permData.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("account_assignments[%s] is not a map", group)
		}

		result[group] = make(map[string][]string, len(permMap))

		for perm, accountsData := range permMap {
			accountsList, ok := accountsData.([]interface{})
			if !ok {
				return nil, fmt.Errorf("account_assignments[%s][%s] is not a list", group, perm)
			}

			accounts := make([]string, 0, len(accountsList))
			for _, a := range accountsList {
				accounts = append(accounts, fmt.Sprintf("%v", a))
			}
			result[group][perm] = accounts
		}
	}

	return result, nil
}

// findVarsCandidates returns all vars maps found at known YAML paths.
// It searches:
//   - raw["vars"] (top-level)
//   - raw["components"]["terraform"][componentName]["vars"] (component-level)
func findVarsCandidates(raw map[string]interface{}, componentName string) []map[string]interface{} {
	var candidates []map[string]interface{}

	// Top-level vars.
	if vars, ok := raw["vars"]; ok {
		if varsMap, ok := vars.(map[string]interface{}); ok {
			candidates = append(candidates, varsMap)
		}
	}

	// Component-level vars: components.terraform.<componentName>.vars.
	if components, ok := raw["components"]; ok {
		if compMap, ok := components.(map[string]interface{}); ok {
			if terraform, ok := compMap["terraform"]; ok {
				if tfMap, ok := terraform.(map[string]interface{}); ok {
					if comp, ok := tfMap[componentName]; ok {
						if compCfg, ok := comp.(map[string]interface{}); ok {
							if vars, ok := compCfg["vars"]; ok {
								if varsMap, ok := vars.(map[string]interface{}); ok {
									candidates = append(candidates, varsMap)
								}
							}
						}
					}
				}
			}
		}
	}

	return candidates
}
