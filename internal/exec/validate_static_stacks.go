package exec

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

// ValidateStaticStackMaps reuses stack inheritance and duplicate-instance validation
// for already decoded, complete static inputs. It never loads YAML, runs templates,
// evaluates functions, or provisions components. Callers must reject unresolved inputs.
func ValidateStaticStackMaps(ctx context.Context, config *schema.AtmosConfiguration, inputs map[string]map[string]any) error {
	defer perf.Track(config, "exec.ValidateStaticStackMaps")()
	stacks, err := processStaticStackMaps(ctx, config, inputs)
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

// processStaticStackMaps resolves inheritance without executing configuration.
func processStaticStackMaps(ctx context.Context, config *schema.AtmosConfiguration, inputs map[string]map[string]any) (map[string]any, error) {
	stacks := map[string]any{}
	paths := make([]string, 0, len(inputs))
	for path := range inputs {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		input := inputs[path]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stack, _, err := ProcessStackConfig(config, config.StacksBaseAbsolutePath, config.TerraformDirAbsolutePath, config.HelmfileDirAbsolutePath, config.PackerDirAbsolutePath, config.AnsibleDirAbsolutePath, filepath.Join(config.StacksBaseAbsolutePath, path), input, false, true, "", map[string]map[string][]string{}, map[string]map[string]any{}, true)
		if err != nil {
			return nil, err
		}
		stacks[strings.TrimSuffix(path, filepath.Ext(path))] = stack
	}
	return stacks, nil
}

// ValidateStaticDependencies checks required targets and cycles using the production
// Terraform graph builder. Inputs must already be complete and free of executable values.
func ValidateStaticDependencies(ctx context.Context, config *schema.AtmosConfiguration, inputs map[string]map[string]any) error {
	defer perf.Track(config, "exec.ValidateStaticDependencies")()
	stacks, err := processStaticStackMaps(ctx, config, inputs)
	if err != nil {
		return err
	}
	// Resolve the same logical stack identities as validate/describe stacks.
	instances, err := createComponentStackMap(config, stacks, cfg.TerraformComponentType)
	if err != nil {
		return err
	}
	logical := map[string]any{}
	for component, names := range instances {
		for name, manifests := range names {
			if err := ctx.Err(); err != nil {
				return err
			}
			slices.Sort(manifests)
			value, err := getValidationComponentConfig(stacks, cfg.TerraformComponentType, component, manifests[0])
			if err != nil {
				return err
			}
			if logical[name] == nil {
				logical[name] = map[string]any{"components": map[string]any{cfg.TerraformComponentType: map[string]any{}}}
			}
			logical[name].(map[string]any)["components"].(map[string]any)[cfg.TerraformComponentType].(map[string]any)[component] = value
		}
	}
	leftDelim, _ := tags.TemplateDelims(config.Templates.Settings.Delimiters)
	return adapters.ValidateTerraformGraph(logical, leftDelim)
}
