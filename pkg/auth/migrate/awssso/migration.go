// Package awssso implements the AWS SSO migration steps and factory.
package awssso

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth/migrate"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
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

// BuildMigrationContext loads atmos config and uses atmos stack resolution
// to discover SSO and account-map component configuration.
func BuildMigrationContext(ctx context.Context, fs migrate.FileSystem, prompter migrate.Prompter) (*migrate.MigrationContext, error) {
	defer perf.Track(nil, "awssso.BuildMigrationContext")()

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToInitConfig, err)
	}

	// CliConfigPath is the directory containing atmos.yaml, not the file itself.
	atmosConfigFile := filepath.Join(atmosConfig.CliConfigPath, "atmos.yaml")
	log.Debug("Loaded atmos config", "config_path", atmosConfigFile, "stacks_base", atmosConfig.Stacks.BasePath)

	migCtx := &migrate.MigrationContext{
		AtmosConfig:     &atmosConfig,
		StacksBasePath:  atmosConfig.Stacks.BasePath,
		ProfilesPath:    "profiles",
		ExistingAuth:    &atmosConfig.Auth,
		AtmosConfigPath: atmosConfigFile,
	}

	// Use atmos stack resolution to discover component configs.
	migCtx.AccountMap, err = discoverAccountMapViaStacks(&atmosConfig)
	if err != nil {
		log.Debug("Failed to discover account map via stacks", "error", err)
		migCtx.AccountMap = make(map[string]string)
	}
	log.Debug("Discovered account map", "accounts", len(migCtx.AccountMap))

	// Discover SSO config via stacks, with existing auth config as defaults.
	migCtx.SSOConfig, err = discoverSSOConfigViaStacks(&atmosConfig, &atmosConfig.Auth, prompter)
	if err != nil {
		return nil, err
	}
	log.Debug("Discovered SSO config",
		"start_url", migCtx.SSOConfig.StartURL,
		"region", migCtx.SSOConfig.Region,
		"provider", migCtx.SSOConfig.ProviderName,
		"groups", len(migCtx.SSOConfig.AccountAssignments))

	return migCtx, nil
}

// discoverAccountMapViaStacks uses atmos describe stacks to find an account-map
// component and extract the full_account_map or account_map from its vars.
func discoverAccountMapViaStacks(atmosConfig *schema.AtmosConfiguration) (map[string]string, error) {
	defer perf.Track(nil, "awssso.discoverAccountMapViaStacks")()

	componentName := "account-map"
	stack, err := findStackWithComponent(atmosConfig, componentName)
	if err != nil {
		return nil, fmt.Errorf("finding stack with %s component: %w", componentName, err)
	}
	if stack == "" {
		log.Debug("No stack found with account-map component")
		return nil, fmt.Errorf("no stack contains component %q", componentName)
	}

	log.Debug("Found account-map component", "stack", stack)

	componentSection, err := describeComponentRaw(atmosConfig, componentName, stack)
	if err != nil {
		return nil, fmt.Errorf("describing %s in stack %s: %w", componentName, stack, err)
	}

	return extractAccountMap(componentSection)
}

// discoverSSOConfigViaStacks uses atmos describe stacks to find an aws-sso
// component and extract account_assignments from its vars.
func discoverSSOConfigViaStacks(atmosConfig *schema.AtmosConfiguration, existingAuth *schema.AuthConfig, prompter migrate.Prompter) (*migrate.SSOConfig, error) {
	defer perf.Track(nil, "awssso.discoverSSOConfigViaStacks")()

	ssoCfg := &migrate.SSOConfig{
		AccountAssignments: make(map[string]map[string][]string),
	}

	// First, extract defaults from existing auth config in atmos.yaml.
	if existingAuth != nil {
		log.Debug("Checking existing auth config for SSO providers", "provider_count", len(existingAuth.Providers))
		for name, provider := range existingAuth.Providers {
			log.Debug("Found auth provider", "name", name, "kind", provider.Kind)
			if provider.Kind == "aws/iam-identity-center" {
				ssoCfg.ProviderName = name
				ssoCfg.StartURL = provider.StartURL
				ssoCfg.Region = provider.Region
				log.Debug("Extracted SSO defaults from existing auth config",
					"provider", name, "start_url", provider.StartURL, "region", provider.Region)
				break
			}
		}
	}

	// Find aws-sso component via stacks.
	componentName := "aws-sso"
	stack, err := findStackWithComponent(atmosConfig, componentName)
	if err != nil {
		log.Debug("Error finding stack with aws-sso component", "error", err)
	}

	if stack != "" {
		log.Debug("Found aws-sso component", "stack", stack)

		componentSection, descErr := describeComponentRaw(atmosConfig, componentName, stack)
		if descErr != nil {
			log.Debug("Failed to describe aws-sso component", "stack", stack, "error", descErr)
		} else {
			extractSSOFromComponent(componentSection, ssoCfg)
		}
	} else {
		log.Debug("No stack found with aws-sso component")
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
		providerName, inputErr := prompter.Input("Enter SSO provider name", "sso")
		if inputErr != nil {
			return nil, fmt.Errorf("prompting for provider name: %w", inputErr)
		}
		ssoCfg.ProviderName = providerName
	}

	return ssoCfg, nil
}

// findStackWithComponent uses ExecuteDescribeStacks to find the first stack
// that contains the given component.
func findStackWithComponent(atmosConfig *schema.AtmosConfiguration, component string) (string, error) {
	defer perf.Track(nil, "awssso.findStackWithComponent")()

	stacks, err := exec.ExecuteDescribeStacks(
		atmosConfig,
		"",                    // filterByStack — search all.
		[]string{component},   // components — filter to this component.
		[]string{"terraform"}, // componentTypes.
		[]string{"vars"},      // sections — we only need vars.
		true,                  // ignoreMissingFiles.
		false,                 // processTemplates.
		false,                 // processYamlFunctions.
		false,                 // includeEmptyStacks.
		nil,                   // skip.
		nil,                   // authManager.
	)
	if err != nil {
		return "", err
	}

	// Return the first stack that has this component.
	for stackName := range stacks {
		log.Debug("Stack contains component", "stack", stackName, "component", component)
		return stackName, nil
	}

	return "", nil
}

// describeComponentRaw uses ExecuteDescribeComponent to get the fully resolved
// component config without processing templates or YAML functions.
func describeComponentRaw(atmosConfig *schema.AtmosConfiguration, component, stack string) (map[string]any, error) {
	defer perf.Track(nil, "awssso.describeComponentRaw")()

	return exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})
}

// extractAccountMap extracts account name-to-ID mappings from a described
// account-map component section.
func extractAccountMap(componentSection map[string]any) (map[string]string, error) {
	vars, ok := componentSection["vars"]
	if !ok {
		return nil, fmt.Errorf("account-map component has no vars")
	}

	varsMap, ok := vars.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("account-map vars is not a map")
	}

	// Log available var keys for debugging.
	varKeys := make([]string, 0, len(varsMap))
	for k := range varsMap {
		varKeys = append(varKeys, k)
	}
	log.Debug("account-map vars keys", "keys", varKeys)

	// Try full_account_map first, then account_map.
	for _, key := range []string{"full_account_map", "account_map"} {
		if v, exists := varsMap[key]; exists {
			accountMap, ok := v.(map[string]any)
			if !ok {
				log.Debug("Account map key is not a map", "key", key)
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

			log.Debug("Extracted account map", "key", key, "accounts", len(result))
			return result, nil
		}
	}

	return nil, fmt.Errorf("account-map vars missing full_account_map or account_map")
}

// extractSSOFromComponent extracts SSO config from a described aws-sso component.
func extractSSOFromComponent(componentSection map[string]any, ssoCfg *migrate.SSOConfig) {
	vars, ok := componentSection["vars"]
	if !ok {
		log.Debug("aws-sso component has no vars")
		return
	}

	varsMap, ok := vars.(map[string]any)
	if !ok {
		log.Debug("aws-sso vars is not a map")
		return
	}

	// Log available var keys for debugging.
	varKeys := make([]string, 0, len(varsMap))
	for k := range varsMap {
		varKeys = append(varKeys, k)
	}
	log.Debug("aws-sso vars keys", "keys", varKeys)

	// Extract optional start_url and region.
	if v, exists := varsMap["start_url"]; exists && ssoCfg.StartURL == "" {
		ssoCfg.StartURL = fmt.Sprintf("%v", v)
	}
	if v, exists := varsMap["region"]; exists && ssoCfg.Region == "" {
		ssoCfg.Region = fmt.Sprintf("%v", v)
	}

	// Extract account_assignments.
	if assignData, exists := varsMap["account_assignments"]; exists && len(ssoCfg.AccountAssignments) == 0 {
		parsed, err := parseAccountAssignments(assignData)
		if err != nil {
			log.Debug("Failed to parse account_assignments from component", "error", err)
			return
		}
		ssoCfg.AccountAssignments = parsed
		log.Debug("Extracted account assignments from aws-sso component", "groups", len(parsed))
	}
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
