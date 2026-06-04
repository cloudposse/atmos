package component

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	ComponentInfoKey       = "component_info"
	ComponentEnvelopeKey   = "component_envelope"
	ComponentTypeInfoKey   = "component_type"
	componentAliasesKey    = "aliases"
	componentTypeTerraform = "terraform"
	componentTypeHelmfile  = "helmfile"
	componentTypePacker    = "packer"
	componentTypeAnsible   = "ansible"
)

var builtInComponentTypes = map[string]struct{}{
	componentTypeTerraform: {},
	componentTypeHelmfile:  {},
	componentTypePacker:    {},
	componentTypeAnsible:   {},
}

var (
	ErrInvalidComponentAlias   = errors.New("invalid component alias")
	ErrInvalidComponentSection = errors.New("invalid component section")
)

type componentAliasSource struct {
	canonical string
	aliases   []string
}

// AliasMap returns a normalized alias -> canonical component type map.
func AliasMap(atmosConfig *schema.AtmosConfiguration) (map[string]string, error) {
	defer perf.Track(atmosConfig, "component.AliasMap")()

	aliases := make(map[string]string)
	if atmosConfig == nil {
		return aliases, nil
	}

	canonicalTypes := knownComponentTypes(atmosConfig)
	for _, source := range componentAliasSources(atmosConfig) {
		if err := registerComponentAliases(aliases, canonicalTypes, source); err != nil {
			return nil, err
		}
	}

	return aliases, nil
}

// CanonicalType resolves an input or display component type to its canonical type.
func CanonicalType(atmosConfig *schema.AtmosConfiguration, componentType string) string {
	defer perf.Track(atmosConfig, "component.CanonicalType")()

	normalized := normalizeComponentType(componentType)
	aliases, err := AliasMap(atmosConfig)
	if err != nil {
		return normalized
	}
	if canonical, ok := aliases[normalized]; ok {
		return canonical
	}
	return normalized
}

// AliasesByCanonical returns normalized aliases grouped by canonical component type.
func AliasesByCanonical(atmosConfig *schema.AtmosConfiguration) (map[string][]string, error) {
	defer perf.Track(atmosConfig, "component.AliasesByCanonical")()

	aliasMap, err := AliasMap(atmosConfig)
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]string)
	for alias, canonical := range aliasMap {
		grouped[canonical] = append(grouped[canonical], alias)
	}
	for canonical := range grouped {
		sort.Strings(grouped[canonical])
	}
	return grouped, nil
}

// NormalizeComponentSections merges alias component sections into canonical sections.
// It returns an envelope map keyed by canonical type and component name.
func NormalizeComponentSections(
	atmosConfig *schema.AtmosConfiguration,
	components map[string]any,
) (map[string]any, map[string]map[string]string, error) {
	defer perf.Track(atmosConfig, "component.NormalizeComponentSections")()

	normalized := make(map[string]any, len(components))
	for k, v := range components {
		normalized[k] = v
	}

	envelopes := make(map[string]map[string]string)
	aliases, err := AliasMap(atmosConfig)
	if err != nil {
		return nil, nil, err
	}
	for alias, canonical := range aliases {
		if _, ok := normalized[alias]; !ok {
			continue
		}
		if err := mergeAliasComponentSection(normalized, envelopes, alias, canonical); err != nil {
			return nil, nil, err
		}
	}

	return normalized, envelopes, nil
}

func componentAliasSources(atmosConfig *schema.AtmosConfiguration) []componentAliasSource {
	sources := []componentAliasSource{
		{canonical: componentTypeTerraform, aliases: atmosConfig.Components.Terraform.Aliases},
		{canonical: componentTypeHelmfile, aliases: atmosConfig.Components.Helmfile.Aliases},
		{canonical: componentTypePacker, aliases: atmosConfig.Components.Packer.Aliases},
		{canonical: componentTypeAnsible, aliases: atmosConfig.Components.Ansible.Aliases},
	}
	for componentType, raw := range atmosConfig.Components.Plugins {
		sources = append(sources, componentAliasSource{
			canonical: componentType,
			aliases:   extractPluginAliases(raw),
		})
	}
	return sources
}

func registerComponentAliases(
	aliases map[string]string,
	canonicalTypes map[string]struct{},
	source componentAliasSource,
) error {
	canonical := normalizeComponentType(source.canonical)
	seenForCanonical := make(map[string]struct{}, len(source.aliases))
	for _, value := range source.aliases {
		alias := normalizeComponentType(value)
		if err := validateComponentAlias(alias, canonical, seenForCanonical, canonicalTypes, aliases); err != nil {
			return err
		}
		seenForCanonical[alias] = struct{}{}
		aliases[alias] = canonical
	}
	return nil
}

func validateComponentAlias(
	alias string,
	canonical string,
	seenForCanonical map[string]struct{},
	canonicalTypes map[string]struct{},
	aliases map[string]string,
) error {
	if alias == "" {
		return fmt.Errorf("%w: alias for %q cannot be empty", ErrInvalidComponentAlias, canonical)
	}
	if alias == canonical {
		return fmt.Errorf("%w: alias %q cannot alias itself", ErrInvalidComponentAlias, alias)
	}
	if _, exists := seenForCanonical[alias]; exists {
		return fmt.Errorf("%w: alias %q is declared more than once for %q", ErrInvalidComponentAlias, alias, canonical)
	}
	if _, exists := canonicalTypes[alias]; exists {
		return fmt.Errorf("%w: alias %q conflicts with registered component type", ErrInvalidComponentAlias, alias)
	}
	if existing, exists := aliases[alias]; exists && existing != canonical {
		return fmt.Errorf("%w: alias %q maps to both %q and %q", ErrInvalidComponentAlias, alias, existing, canonical)
	}
	return nil
}

func mergeAliasComponentSection(
	normalized map[string]any,
	envelopes map[string]map[string]string,
	alias string,
	canonical string,
) error {
	aliasSection, err := componentSectionMap(normalized, alias)
	if err != nil {
		return err
	}
	canonicalSection := map[string]any{}
	if _, ok := normalized[canonical]; ok {
		canonicalSection, err = componentSectionMap(normalized, canonical)
		if err != nil {
			return err
		}
	}

	for componentName, componentConfig := range aliasSection {
		if _, exists := canonicalSection[componentName]; exists {
			return fmt.Errorf("%w: component %q is defined under both components.%s and components.%s",
				ErrInvalidComponentSection, componentName, canonical, alias)
		}
		canonicalSection[componentName] = componentConfig
		if envelopes[canonical] == nil {
			envelopes[canonical] = make(map[string]string)
		}
		envelopes[canonical][componentName] = alias
	}

	normalized[canonical] = canonicalSection
	delete(normalized, alias)
	return nil
}

func componentSectionMap(components map[string]any, componentType string) (map[string]any, error) {
	section, ok := components[componentType].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: components.%s must be a map", ErrInvalidComponentSection, componentType)
	}
	return section, nil
}

func normalizeComponentType(componentType string) string {
	return strings.ToLower(strings.TrimSpace(componentType))
}

func knownComponentTypes(atmosConfig *schema.AtmosConfiguration) map[string]struct{} {
	types := make(map[string]struct{}, len(builtInComponentTypes)+len(atmosConfig.Components.Plugins)+len(ListTypes()))
	for t := range builtInComponentTypes {
		types[t] = struct{}{}
	}
	for t := range atmosConfig.Components.Plugins {
		types[normalizeComponentType(t)] = struct{}{}
	}
	for _, t := range ListTypes() {
		types[normalizeComponentType(t)] = struct{}{}
	}
	return types
}

func extractPluginAliases(raw any) []string {
	config, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	value, ok := config[componentAliasesKey]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		aliases := make([]string, 0, len(typed))
		for _, item := range typed {
			if alias, ok := item.(string); ok {
				aliases = append(aliases, alias)
			}
		}
		return aliases
	default:
		return nil
	}
}
