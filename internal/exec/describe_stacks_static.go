package exec

import (
	"slices"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

// findDescribeStacksMap keeps static inspection isolated from manifest maps cached by evaluated callers.
//
//nolint:revive // Return shape intentionally mirrors FindStacksMap.
func findDescribeStacksMap(config *schema.AtmosConfiguration, stack string, ignoreMissing, staticOnly bool) (map[string]any, map[string]map[string]any, AllStacksDeferredContexts, error) {
	if !staticOnly {
		return FindStacksMap(config, ignoreMissing)
	}
	files, err := cfg.SelectStaticStackFiles(config, stack, config.StackConfigFilesAbsolutePaths, GetFileContentWithoutCache)
	if err != nil {
		return nil, nil, nil, err
	}
	_, stacks, raw, deferred, err := processYAMLConfigFilesWithOptions(config, config.StacksBaseAbsolutePath,
		config.TerraformDirAbsolutePath, config.HelmfileDirAbsolutePath, config.PackerDirAbsolutePath,
		config.AnsibleDirAbsolutePath, files, false, true, ignoreMissing, &StackLoadOptions{SkipTemplates: true, StaticOnly: true})
	return stacks, raw, deferred, err
}

func (p *describeStacksProcessor) resolveStackName(file, manifest string, info schema.ConfigAndStacksInfo, vars map[string]any) (string, schema.Context, error) { //nolint:gocritic // Mirrors the existing read-only name resolver.
	if !p.staticOnly {
		return ResolveStackName(p.atmosConfig, file, manifest, info, vars)
	}
	if manifest != "" {
		name, err := cfg.StaticStackManifestName(p.atmosConfig, file, manifest)
		return name, schema.Context{}, err
	}
	if p.atmosConfig.Stacks.NameTemplate == "" {
		if err := cfg.ValidateStaticStackNamePattern(p.atmosConfig, file, vars); err != nil {
			return "", schema.Context{}, err
		}
		return ResolveStackName(p.atmosConfig, file, "", info, literalsource.RestoreMap(vars))
	}
	name, err := cfg.RenderStaticStackName(p.atmosConfig.Stacks.NameTemplate, file, info.ComponentSection)
	return name, schema.Context{}, err
}

// hasRequestedComponents avoids inventing a global name when component overrides determine real stack names.
func (p *describeStacksProcessor) hasRequestedComponents(stack map[string]any) bool {
	components, _ := stack["components"].(map[string]any)
	for kind, section := range components {
		if len(p.componentTypes) > 0 && !slices.Contains(p.componentTypes, kind) {
			continue
		}
		if values, ok := section.(map[string]any); ok && len(values) > 0 {
			return true
		}
	}
	return false
}

func (p *describeStacksProcessor) includeStaticEmptyStack(file string, stack map[string]any) error {
	vars, _ := stack["vars"].(map[string]any)
	name, _, err := p.resolveStackName(file, "", schema.ConfigAndStacksInfo{ComponentSection: stack}, vars)
	if err != nil {
		return err
	}
	if !shouldFilterByStack(p.filterByStack, file, name) {
		if _, exists := p.finalStacksMap[name]; !exists {
			p.finalStacksMap[name] = map[string]any{"components": map[string]any{}}
		}
	}
	return nil
}
