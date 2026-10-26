// Package scoped resolves secret provider configuration outside a component context.
package scoped

import (
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosYaml "github.com/cloudposse/atmos/pkg/yaml"
)

// StackProviderSections resolves providers in matching manifests without requiring components.
// Imports and stack identity use the same loader and naming logic as component description.
func StackProviderSections(config *schema.AtmosConfiguration, filter string, skip []string) ([]map[string]any, error) {
	defer perf.Track(config, "scoped.StackProviderSections")()

	var sections []map[string]any
	for _, path := range config.StackConfigFilesAbsolutePaths {
		result, err := stack.ProcessYAMLConfigFile(config, config.StacksBaseAbsolutePath, path,
			map[string]map[string]any{}, nil, false, false, config.Templates.Settings.IgnoreMissingTemplateValues,
			false, nil, nil, nil, nil, "")
		if err != nil {
			return nil, err
		}
		section := result.DeepMergedConfig
		if len(secrets.ExtractProviders(section)) == 0 {
			continue
		}
		file, err := filepath.Rel(config.StacksBaseAbsolutePath, path)
		if err != nil {
			return nil, err
		}
		file = filepath.ToSlash(strings.TrimSuffix(file, filepath.Ext(file)))
		explicitName, _ := section[cfg.NameSectionName].(string)
		vars, _ := section[cfg.VarsSectionName].(map[string]any)
		info := schema.ConfigAndStacksInfo{ComponentSection: section, ComponentVarsSection: vars}
		name, _, err := e.ResolveStackName(config, file, explicitName, info, vars)
		if err != nil {
			return nil, err
		}
		if filter != "" && filter != file && filter != name {
			continue
		}
		resolved, err := resolveProviders(config, name, section, skip)
		if err != nil {
			return nil, err
		}
		sections = append(sections, resolved)
	}
	return sections, nil
}

// resolveProviders evaluates providers and their local dependencies without reading secret values.
func resolveProviders(config *schema.AtmosConfiguration, name string, section map[string]any, skip []string) (map[string]any, error) {
	paths := deferred.ExpandEvaluationPaths(section, [][]string{{cfg.SecretsSectionName, "providers"}}, config.Templates.Settings.Delimiters...)
	input, _ := deferred.SplitEvaluationFields(section, paths)
	source, err := atmosYaml.ConvertToYAMLPreservingDelimiters(input, config.Templates.Settings.Delimiters)
	if err != nil {
		return nil, err
	}
	var settings schema.Settings
	if err = mapstructure.Decode(section[cfg.SettingsSectionName], &settings); err != nil {
		return nil, err
	}
	vars, _ := section[cfg.VarsSectionName].(map[string]any)
	info := schema.ConfigAndStacksInfo{
		ComponentSection: section, ComponentVarsSection: vars,
		Context: cfg.GetContextFromVars(vars), Stack: name, EvaluationPaths: paths,
	}
	rendered, err := e.ProcessTmplWithDatasources(config, &info, settings, "secret-stack-providers", source, section, true)
	if err != nil {
		return nil, err
	}
	converted, err := u.UnmarshalYAML[map[string]any](rendered)
	if err != nil {
		return nil, err
	}
	return e.ProcessCustomYamlTags(config, converted, name, skip, &info)
}
