package exec

import (
	"context"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
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
