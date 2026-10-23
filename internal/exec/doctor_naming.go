package exec

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/helmfile"
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StaticNamingResults evaluates naming fields through production consumers after static discovery.
// The caller must capture a complete source closure free of executable configuration. Naming
// expressions themselves are checked again here before invoking any template evaluator.
func StaticNamingResults(ctx context.Context, config *schema.AtmosConfiguration, inputs StaticStackInputs) (map[naming.Identity]naming.Result, error) {
	defer perf.Track(config, "exec.StaticNamingResults")()
	if !defaultNamingDelimiters(config.Templates.Settings.Delimiters) {
		return nil, fmt.Errorf("%w: custom template delimiters require manual naming review", errUtils.ErrInvalidArguments)
	}
	if config.Templates.Settings.IgnoreMissingTemplateValues {
		return nil, fmt.Errorf("%w: ignored missing template inputs cannot establish naming equivalence", errUtils.ErrInvalidArguments)
	}
	for _, value := range []string{config.Stacks.NameTemplate, config.Components.Helmfile.ClusterNameTemplate} {
		if !naming.SafeTemplate(value) {
			return nil, fmt.Errorf("%w: unsafe naming template", errUtils.ErrInvalidArguments)
		}
	}
	stacks, err := processStaticStacks(ctx, config, inputs)
	if err != nil {
		return nil, err
	}
	results := map[naming.Identity]naming.Result{}
	for manifest, raw := range stacks {
		if err := addStaticManifestNames(ctx, config, manifest, raw, results); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func defaultNamingDelimiters(delimiters []string) bool {
	return len(delimiters) == 0 || slices.Equal(delimiters, []string{"{{", "}}"})
}

func addStaticManifestNames(ctx context.Context, config *schema.AtmosConfiguration, manifest string, raw any, results map[naming.Identity]naming.Result) error {
	stack, _ := raw.(map[string]any)
	kinds, _ := stack["components"].(map[string]any)
	explicit := getStackManifestName(raw)
	for kind, rawComponents := range kinds {
		components, _ := rawComponents.(map[string]any)
		for component, rawSection := range components {
			if err := ctx.Err(); err != nil {
				return err
			}
			section, _ := rawSection.(map[string]any)
			metadata, _ := section["metadata"].(map[string]any)
			if IsComponentAbstract(metadata) {
				continue
			}
			identity := naming.Identity{Manifest: manifest, ComponentType: kind, Component: component}
			result, err := staticComponentNames(config, identity, explicit, section)
			if err != nil {
				return err
			}
			results[identity] = result
		}
	}
	return nil
}

func staticComponentNames(config *schema.AtmosConfiguration, identity naming.Identity, explicit string, section map[string]any) (naming.Result, error) {
	manifest, kind, component := identity.Manifest, identity.ComponentType, identity.Component
	result := naming.Result{}
	data := maps.Clone(section)
	vars, _ := data["vars"].(map[string]any)
	metadata, _ := data["metadata"].(map[string]any)
	metadata = maps.Clone(metadata)
	data["metadata"] = metadata
	data["atmos_component"] = component
	metadata, base, _, _, _ := ProcessComponentMetadata(component, data)
	if base == "" {
		data["component"] = component
	} else {
		data["component"] = base
	}
	info := schema.ConfigAndStacksInfo{Stack: manifest, ComponentFromArg: component, ComponentSection: data, ComponentVarsSection: vars, ComponentMetadataSection: metadata, StackManifestName: explicit, BaseComponentPath: base}
	var err error
	result.Stack, _, err = resolveStackName(config, manifest, explicit, info, vars)
	if err != nil {
		return result, err
	}
	if err := processStackContextPrefix(config, &info, manifest, explicit); err != nil {
		return result, err
	}
	data["atmos_stack"] = result.Stack
	data["stack"] = result.Stack
	if kind == "terraform" {
		result.Workspace, err = staticWorkspaceName(config, &info)
		if err != nil {
			return result, err
		}
	}
	return staticClusterNames(config, kind, &info, result)
}

func staticWorkspaceName(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	if value, _ := info.ComponentMetadataSection["terraform_workspace_template"].(string); !naming.SafeTemplate(value) {
		return "", fmt.Errorf("%w: unsafe workspace template", errUtils.ErrInvalidArguments)
	}
	workspace, err := BuildTerraformWorkspace(config, *info)
	if err != nil || !config.Templates.Settings.Enabled || !strings.Contains(workspace, "{{") {
		return workspace, err
	}
	// The component render happens after token expansion, including the derived workspace.
	// Preserve that order: a token introduced by a variable is not expanded a second time.
	if !naming.SafeTemplate(workspace) {
		return "", fmt.Errorf("%w: unsafe workspace pattern", errUtils.ErrInvalidArguments)
	}
	return ProcessTmpl(config, "terraform-workspace-result", workspace, info.ComponentSection, false)
}

func staticClusterNames(config *schema.AtmosConfiguration, kind string, info *schema.ConfigAndStacksInfo, result naming.Result) (naming.Result, error) {
	component, data, vars := info.ComponentFromArg, info.ComponentSection, info.ComponentVarsSection
	cluster := config.Components.Helmfile
	if cluster.ClusterName == "" && cluster.ClusterNameTemplate == "" && cluster.ClusterNamePattern == "" {
		return result, nil
	}
	input := helmfile.ClusterNameInput{ConfigValue: cluster.ClusterName, Template: cluster.ClusterNameTemplate, Pattern: cluster.ClusterNamePattern}
	// Helmfile's runner and aws eks update-kubeconfig populate different legacy contexts.
	// Compare both consumers rather than assuming {component} has one universal meaning.
	legacy := cfg.GetContextFromVars(vars)
	if kind == "helmfile" {
		resolved, err := helmfile.ResolveClusterName(input, &legacy, config, data, ProcessTmpl)
		if err != nil {
			return result, err
		}
		result.Cluster = resolved.ClusterName
	}
	if kind == "helmfile" || kind == "terraform" {
		legacy.Component = strings.ReplaceAll(component, "/", "-")
		resolved, err := helmfile.ResolveClusterName(input, &legacy, config, data, ProcessTmpl)
		if err != nil {
			return result, err
		}
		result.KubeconfigCluster = resolved.ClusterName
	}
	return result, nil
}
