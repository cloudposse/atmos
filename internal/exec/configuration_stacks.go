package exec

import (
	"context"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Discovered definitions are literal provider inventory data, so static inspection
// (for example scoped `atmos auth validate`) includes them as well.
func init() {
	mergeStaticStacks = mergeConfigurationStacks
}

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

// checkConfigurationBackendOverrides passes the component's own backend layers to the
// configuration package before normalization discards their origin. Global backends remain
// allowed; the provider replaces them during preparation.
func checkConfigurationBackendOverrides(opts *ComponentProcessorOptions, result *ComponentProcessorResult, settings map[string]any) error {
	defer perf.Track(nil, "exec.checkConfigurationBackendOverrides")()

	return configuration.CheckBackendOverrides(
		opts.Component, opts.StackName, settings,
		configuration.BackendLayer{Type: result.BaseComponentBackendType, Settings: result.BaseComponentBackendSection},
		configuration.BackendLayer{Type: result.ComponentBackendType, Settings: result.ComponentBackendSection},
	)
}

// validateDiscoveredManifests adapts the configuration package's manifest validation
// to the stack processor's compiled-schema cache.
func validateDiscoveredManifests(config *schema.AtmosConfiguration, schemaPath string) ([]string, error) {
	if config.ComponentConfigurations == nil {
		return nil, nil
	}
	discovered, err := config.ComponentConfigurations.Discover(context.Background())
	if err != nil {
		return nil, err
	}
	return configuration.ValidateDiscoveredManifests(discovered, func() (*jsonschema.Schema, error) {
		return compiledManifestSchema(schemaPath)
	})
}

// compiledManifestSchema shares the stack processor's compiled-schema cache.
func compiledManifestSchema(path string) (*jsonschema.Schema, error) {
	if compiled, found := getCachedCompiledSchema(path); found {
		return compiled, nil
	}
	compiled, err := configuration.CompileManifestSchema(path)
	if err != nil {
		return nil, err
	}
	cacheCompiledSchema(path, compiled)
	return compiled, nil
}
