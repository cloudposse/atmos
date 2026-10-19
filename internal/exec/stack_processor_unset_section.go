package exec

import (
	"slices"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// unsettableComponentSections lists the component sections that `!unset` can remove as a whole.
// Each one merges across the global, base component, component and overrides layers. The
// `!unset` tag is decoded to the plain string "!unset" and is only resolved after the final
// merge, so these sections must be recognized before their map type assertions run.
var unsettableComponentSections = []string{
	cfg.VarsSectionName,
	cfg.SettingsSectionName,
	cfg.EnvSectionName,
	cfg.AuthSectionName,
	cfg.SecretsSectionName,
	cfg.ProvidersSectionName,
	cfg.RequiredProvidersSectionName,
	cfg.HooksSectionName,
	cfg.TestSectionName,
	cfg.MocksSectionName,
	cfg.GenerateSectionName,
	cfg.FlagsSectionName,
	cfg.DependenciesSectionName,
	cfg.LocalsSectionName,
	cfg.RetrySectionName,
	cfg.ProvisionSectionName,
}

// isUnsetSection reports whether a section value is the decoded `!unset` YAML tag.
func isUnsetSection(value any) bool {
	s, ok := value.(string)
	return ok && strings.TrimSpace(s) == u.AtmosYamlFuncUnset
}

// splitUnsetSections returns the section map without the unsettable keys set to `!unset`,
// plus the names of those keys. The input map is never mutated: it is returned unchanged when
// nothing is unset, and a shallow copy is returned otherwise.
func splitUnsetSections(section map[string]any) (map[string]any, []string) {
	var unset []string
	for _, key := range unsettableComponentSections {
		if isUnsetSection(section[key]) {
			unset = append(unset, key)
		}
	}
	if len(unset) == 0 {
		return section, nil
	}

	stripped := make(map[string]any, len(section))
	for k, v := range section {
		stripped[k] = v
	}
	for _, key := range unset {
		delete(stripped, key)
	}
	return stripped, unset
}

// clearUnsetBaseComponentSections drops the values accumulated from lower levels of the
// inheritance chain for each section a base component sets to `!unset`, and records the
// sections so the stack-level (global) layer is dropped in the final merge as well.
func clearUnsetBaseComponentSections(c *schema.BaseComponentConfig, unset []string) {
	for _, section := range unset {
		if field := baseComponentSectionField(c, section); field != nil {
			*field = nil
		}
		if !slices.Contains(c.BaseComponentUnsetSections, section) {
			c.BaseComponentUnsetSections = append(c.BaseComponentUnsetSections, section)
		}
	}
}

// baseComponentSectionField returns the accumulated base component field for an unsettable section.
//
//nolint:cyclop,revive // Flat section-to-field lookup table.
func baseComponentSectionField(c *schema.BaseComponentConfig, section string) *schema.AtmosSectionMapType {
	switch section {
	case cfg.VarsSectionName:
		return &c.BaseComponentVars
	case cfg.SettingsSectionName:
		return &c.BaseComponentSettings
	case cfg.EnvSectionName:
		return &c.BaseComponentEnv
	case cfg.AuthSectionName:
		return &c.BaseComponentAuth
	case cfg.SecretsSectionName:
		return &c.BaseComponentSecrets
	case cfg.ProvidersSectionName:
		return &c.BaseComponentProviders
	case cfg.RequiredProvidersSectionName:
		return &c.BaseComponentRequiredProviders
	case cfg.HooksSectionName:
		return &c.BaseComponentHooks
	case cfg.TestSectionName:
		return &c.BaseComponentTest
	case cfg.MocksSectionName:
		return &c.BaseComponentMocks
	case cfg.GenerateSectionName:
		return &c.BaseComponentGenerate
	case cfg.FlagsSectionName:
		return &c.BaseComponentFlags
	case cfg.DependenciesSectionName:
		return &c.BaseComponentDependencies
	case cfg.LocalsSectionName:
		return &c.BaseComponentLocals
	case cfg.RetrySectionName:
		return &c.BaseComponentRetry
	case cfg.ProvisionSectionName:
		return &c.BaseComponentProvisionSection
	}
	return nil
}

// dropUnsetSectionLayers applies `!unset` on whole sections before the final merge. A section
// unset at one layer drops every lower layer: overrides drop the component, base component
// and global layers; the component drops the base component and global layers; a base
// component drops the global layer. Overrides still apply on top of a component-level unset.
// The inputs are never mutated; shallow copies are returned when a layer is dropped.
func dropUnsetSectionLayers(opts *ComponentProcessorOptions, result *ComponentProcessorResult) (*ComponentProcessorOptions, *ComponentProcessorResult) {
	if len(result.ComponentUnsetSections) == 0 && len(result.ComponentOverridesUnsetSections) == 0 && len(result.BaseComponentUnsetSections) == 0 {
		return opts, result
	}

	o := *opts
	r := *result
	for _, section := range unsettableComponentSections {
		global, base, component := sectionLayerFields(&o, &r, section)
		var drop []*map[string]any
		switch {
		case slices.Contains(r.ComponentOverridesUnsetSections, section):
			drop = []*map[string]any{global, base, component}
		case slices.Contains(r.ComponentUnsetSections, section):
			drop = []*map[string]any{global, base}
		case slices.Contains(r.BaseComponentUnsetSections, section):
			drop = []*map[string]any{global}
		}
		for _, field := range drop {
			if field != nil {
				*field = nil
			}
		}
	}
	return &o, &r
}

// sectionLayerFields returns the global, base component and component layers merged for an
// unsettable section. A nil pointer means the section has no such layer.
//
//nolint:cyclop,revive // Flat section-to-field lookup table.
func sectionLayerFields(o *ComponentProcessorOptions, r *ComponentProcessorResult, section string) (global, base, component *map[string]any) {
	switch section {
	case cfg.VarsSectionName:
		return &o.GlobalVars, &r.BaseComponentVars, &r.ComponentVars
	case cfg.SettingsSectionName:
		return &o.GlobalSettings, &r.BaseComponentSettings, &r.ComponentSettings
	case cfg.EnvSectionName:
		return &o.GlobalEnv, &r.BaseComponentEnv, &r.ComponentEnv
	case cfg.AuthSectionName:
		return &o.GlobalAuth, &r.BaseComponentAuth, &r.ComponentAuth
	case cfg.SecretsSectionName:
		return &o.GlobalSecrets, &r.BaseComponentSecrets, &r.ComponentSecrets
	case cfg.ProvidersSectionName:
		return &o.TerraformProviders, &r.BaseComponentProviders, &r.ComponentProviders
	case cfg.RequiredProvidersSectionName:
		return &o.TerraformRequiredProviders, &r.BaseComponentRequiredProviders, &r.ComponentRequiredProviders
	case cfg.HooksSectionName:
		return &o.GlobalAndTerraformHooks, &r.BaseComponentHooks, &r.ComponentHooks
	case cfg.TestSectionName:
		return nil, &r.BaseComponentTest, &r.ComponentTest
	case cfg.MocksSectionName:
		return nil, &r.BaseComponentMocks, &r.ComponentMocks
	case cfg.GenerateSectionName:
		return &o.GlobalAndTerraformGenerate, &r.BaseComponentGenerate, &r.ComponentGenerate
	case cfg.FlagsSectionName:
		return &o.GlobalAndTerraformFlags, &r.BaseComponentFlags, &r.ComponentFlags
	case cfg.DependenciesSectionName:
		return &o.GlobalDependencies, &r.BaseComponentDependencies, &r.ComponentDependencies
	case cfg.LocalsSectionName:
		return nil, &r.BaseComponentLocals, &r.ComponentLocals
	case cfg.RetrySectionName:
		return &o.GlobalComponentRetry, &r.BaseComponentRetry, &r.ComponentRetry
	case cfg.ProvisionSectionName:
		return &o.GlobalProvisionSection, &r.BaseComponentProvisionSection, &r.ComponentProvision
	}
	return nil, nil, nil
}
