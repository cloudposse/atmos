package exec

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

// StaticStackInputs names the top-level stack manifests of a static validation.
type StaticStackInputs struct {
	// Files are the manifests, as slash paths relative to the stacks base path.
	Files []string
	// Reader supplies the content of each manifest and of everything it imports. A caller that holds
	// edited content reads through an overlay; nil reads the file system.
	Reader StackSourceReader
}

// ValidateStaticStacks reuses stack inheritance and duplicate-instance validation for complete static
// inputs. Each manifest is loaded through the production stack loader, so its imports are merged exactly
// as at runtime, but templates and YAML functions are never evaluated and nothing is provisioned.
// Callers must reject inputs, anywhere in the import closure, that need evaluation.
func ValidateStaticStacks(ctx context.Context, config *schema.AtmosConfiguration, inputs StaticStackInputs) error {
	defer perf.Track(config, "exec.ValidateStaticStacks")()
	stacks, err := processStaticStacks(ctx, config, inputs)
	if err != nil {
		return err
	}
	var messages []string
	for _, kind := range []string{cfg.TerraformComponentType, "helmfile"} {
		instances, err := createComponentStackMap(config, stacks, kind)
		if err != nil {
			return err
		}
		failures, err := checkComponentStackMap(stacks, kind, instances)
		if err != nil {
			return err
		}
		messages = append(messages, failures...)
	}
	if len(messages) > 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrValidationFailed, strings.Join(dedupeStrings(messages), "\n\n"))
	}
	return nil
}

// processStaticStacks loads each manifest with its imports and resolves inheritance without evaluating configuration.
func processStaticStacks(ctx context.Context, config *schema.AtmosConfiguration, inputs StaticStackInputs) (map[string]any, error) {
	// The base component cache is keyed by stack and component, not by content, and assumes one
	// configuration per process. Static validation loads the same stack with different content,
	// such as a repair candidate next to the original, so it must not see a previous load.
	ClearBaseComponentConfigCache()
	stacks := map[string]any{}
	paths := slices.Clone(inputs.Files)
	slices.Sort(paths)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file := filepath.Join(config.StacksBaseAbsolutePath, filepath.FromSlash(path))
		loaded, err := ProcessYAMLConfigFileWithOptions(config, config.StacksBaseAbsolutePath, file, StackLoadOptions{Reader: inputs.Reader, SkipTemplates: true})
		if err != nil {
			return nil, err
		}
		stack, _, err := ProcessStackConfig(config, config.StacksBaseAbsolutePath, config.TerraformDirAbsolutePath, config.HelmfileDirAbsolutePath, config.PackerDirAbsolutePath, config.AnsibleDirAbsolutePath, file, loaded.DeepMergedConfig, false, true, "", map[string]map[string][]string{}, loaded.ImportsConfig, true)
		if err != nil {
			return nil, err
		}
		stacks[strings.TrimSuffix(path, filepath.Ext(path))] = stack
	}
	return stacks, nil
}

// logicalTerraformStacks groups the Terraform components of the processed manifests under the logical
// stack names validate and describe stacks use, so a stack split across manifests is seen as one.
func logicalTerraformStacks(ctx context.Context, config *schema.AtmosConfiguration, stacks map[string]any) (map[string]any, error) {
	instances, err := createComponentStackMap(config, stacks, cfg.TerraformComponentType)
	if err != nil {
		return nil, err
	}
	logical := map[string]any{}
	for component, names := range instances {
		for name, manifests := range names {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			slices.Sort(manifests)
			value, err := getValidationComponentConfig(stacks, cfg.TerraformComponentType, component, manifests[0])
			if err != nil {
				return nil, err
			}
			if logical[name] == nil {
				logical[name] = map[string]any{"components": map[string]any{cfg.TerraformComponentType: map[string]any{}}}
			}
			logical[name].(map[string]any)["components"].(map[string]any)[cfg.TerraformComponentType].(map[string]any)[component] = value
		}
	}
	return logical, nil
}

// ValidateStaticDependencies checks required targets and cycles using the production
// Terraform graph builder. Inputs must already be complete and free of executable values.
// Every graph error is reported, joined, with its location in the graph.
func ValidateStaticDependencies(ctx context.Context, config *schema.AtmosConfiguration, inputs StaticStackInputs) error {
	defer perf.Track(config, "exec.ValidateStaticDependencies")()
	logical, err := staticLogicalStacks(ctx, config, inputs)
	if err != nil {
		return err
	}
	leftDelim, _ := tags.TemplateDelims(config.Templates.Settings.Delimiters)
	return adapters.ValidateTerraformGraph(logical, leftDelim)
}

// StaticDependencyTargets returns the components each enabled Terraform component depends on, as the
// scheduler selects them, keyed by component and logical stack name.
func StaticDependencyTargets(ctx context.Context, config *schema.AtmosConfiguration, inputs StaticStackInputs) (map[dependency.ComponentRef][]dependency.ComponentRef, error) {
	defer perf.Track(config, "exec.StaticDependencyTargets")()
	logical, err := staticLogicalStacks(ctx, config, inputs)
	if err != nil {
		return nil, err
	}
	leftDelim, _ := tags.TemplateDelims(config.Templates.Settings.Delimiters)
	return adapters.TerraformDependencyTargets(logical, leftDelim)
}

// staticLogicalStacks loads the manifests and resolves the same logical stack identities as validate/describe stacks.
func staticLogicalStacks(ctx context.Context, config *schema.AtmosConfiguration, inputs StaticStackInputs) (map[string]any, error) {
	stacks, err := processStaticStacks(ctx, config, inputs)
	if err != nil {
		return nil, err
	}
	return logicalTerraformStacks(ctx, config, stacks)
}
