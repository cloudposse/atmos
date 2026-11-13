package exec

import (
	"context"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mergeConfigurationStacks feeds provider definitions through the ordinary stack compiler.
// The invocation-owned provider retains the caller's cancellation scope.
func mergeConfigurationStacks(config *schema.AtmosConfiguration, stacks map[string]any, raw map[string]map[string]any, contexts AllStacksDeferredContexts) error {
	if config.ComponentConfigurations == nil {
		return nil
	}
	discovered, err := config.ComponentConfigurations.Discover(context.Background())
	if err != nil {
		return err
	}
	for name, definition := range discovered {
		for file, value := range stacks {
			existing, _ := value.(map[string]any)
			if file == name || existing["name"] == name {
				return errUtils.Build(errUtils.ErrDuplicatePluginExtension).
					WithExplanationf("A discovered stack is named `%s`, which is already a stack defined in the stack manifests.", name).
					WithContext("stack", name).
					WithHintf("Rename the stack manifest `%s`, or move the discovered units to a different directory so their derived stack name differs.", name).
					Err()
			}
		}
		processed, deferred, err := ProcessStackConfig(config, config.StacksBaseAbsolutePath, config.TerraformDirAbsolutePath, config.HelmfileDirAbsolutePath, config.PackerDirAbsolutePath, config.AnsibleDirAbsolutePath, filepath.Join(config.StacksBaseAbsolutePath, name+".yaml"), definition, false, true, "", map[string]map[string][]string{}, map[string]map[string]any{}, false)
		if err != nil {
			return err
		}
		stacks[name], raw[name], contexts[name] = processed, definition, deferred
	}
	return nil
}

// checkConfigurationBackendOverrides rejects component options before normalization discards
// their origin. Global backends remain allowed; the provider replaces them during preparation.
func checkConfigurationBackendOverrides(opts *ComponentProcessorOptions, result *ComponentProcessorResult, settings map[string]any) error {
	defer perf.Track(nil, "exec.checkConfigurationBackendOverrides")()
	if _, owned := settings["configuration"]; !owned {
		return nil
	}
	for _, layer := range []struct {
		kind    string
		backend map[string]any
	}{
		{result.BaseComponentBackendType, result.BaseComponentBackendSection},
		{result.ComponentBackendType, result.ComponentBackendSection},
	} {
		setting := ""
		if layer.kind != "" && layer.kind != "local" {
			setting = "backend_type"
		}
		if local, ok := layer.backend["local"].(map[string]any); ok && len(local) > 0 {
			setting = "backend.local"
		}
		if setting != "" {
			return errUtils.Build(fmt.Errorf("%w: the state backend of component %q in stack %q is owned by the configuration provider, so `%s` cannot be set", errUtils.ErrConfigurationOverrideUnsupported, opts.Component, opts.StackName, setting)).
				WithContext("component", opts.Component).
				WithContext("stack", opts.StackName).
				WithHint("Define the backend in the unit's remote_state block; component backend overrides cannot change the unit's state identity.").Err()
		}
	}
	return nil
}
