package secret

import (
	"reflect"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/secrets/scoped"
)

// loadKeygenScope resolves providers through the same credential-free stack processing used by
// secret discovery. Providers need not have declarations yet: key generation often precedes them.
func loadKeygenScope(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, args []string) error {
	scope, err := parseFacets(cmd)
	if err != nil {
		return err
	}
	if scope.Stack == "" {
		if scope.Component != "" || scope.ComponentType != "" {
			return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).WithExplanation("--stack is required when selecting a component for key generation").Err()
		}
		return nil
	}
	// The global-only loader intentionally doesn't initialize stack paths.
	scopedConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{Stack: scope.Stack}, true)
	if err != nil {
		return err
	}
	var components []string
	if scope.Component != "" {
		components = []string{scope.Component}
	}
	stacks, err := describeSecretStacks(&scopedConfig, scope.Stack, components)
	if err != nil {
		return err
	}
	sections := keygenSections(stacks, scope.ComponentType)
	// Explicit component selectors must match before inheriting a stack provider.
	if len(sections) == 0 && scope.Component == "" && scope.ComponentType == "" {
		sections, err = scoped.StackProviderSections(&scopedConfig, scope.Stack, credentialFreeSkip())
		if err != nil {
			return err
		}
	}
	if len(sections) == 0 {
		return errUtils.Build(ErrNoVault).WithExplanationf("no matching component context found in stack %q", scope.Stack).WithHint("Check --stack, --component and --type").Err()
	}
	definitions, err := keygenProviderDefinitions(sections, scope, args)
	if err != nil {
		return err
	}
	mergeKeygenProviders(atmosConfig, definitions)
	return nil
}

func mergeKeygenProviders(atmosConfig *schema.AtmosConfiguration, definitions map[string]schema.SecretProviderConfig) {
	// Copy the map so a scoped override cannot mutate the global configuration.
	merged := make(map[string]schema.SecretProviderConfig, len(atmosConfig.Secrets.Providers)+len(definitions))
	for name, definition := range atmosConfig.Secrets.Providers {
		merged[name] = definition
	}
	for name, definition := range definitions {
		merged[name] = definition
	}
	atmosConfig.Secrets.Providers = merged
}

// keygenProviderDefinitions refuses to pick an arbitrary component override when no component is selected.
func keygenProviderDefinitions(sections []map[string]any, scope secretScope, args []string) (map[string]schema.SecretProviderConfig, error) {
	definitions := map[string]schema.SecretProviderConfig{}
	for _, section := range sections {
		for name, rawDefinition := range secrets.ExtractProviders(section) {
			if len(args) > 0 && name != args[0] {
				continue
			}
			definition, _ := rawDefinition.(map[string]any)
			provider := schema.SecretProviderConfig{}
			provider.Kind, _ = definition["kind"].(string)
			provider.Identity, _ = definition["identity"].(string)
			provider.Spec, _ = definition["spec"].(map[string]any)
			if previous, exists := definitions[name]; exists && !reflect.DeepEqual(previous, provider) {
				return nil, errUtils.Build(ErrAmbiguousVault).WithExplanationf("vault %q differs between components in stack %q", name, scope.Stack).WithHint("Select the provider's component with --component and, if needed, --type").Err()
			}
			definitions[name] = provider
		}
	}
	return definitions, nil
}

func keygenSections(stacks map[string]any, componentType string) []map[string]any {
	var sections []map[string]any
	for _, raw := range stacks {
		stack, _ := raw.(map[string]any)
		types, _ := stack[cfg.ComponentsSectionName].(map[string]any)
		for kind, rawComponents := range types {
			if componentType != "" && kind != componentType {
				continue
			}
			componentMap, _ := rawComponents.(map[string]any)
			for _, rawSection := range componentMap {
				section, _ := rawSection.(map[string]any)
				sections = append(sections, section)
			}
		}
	}
	return sections
}
