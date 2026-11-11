package exec

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
				return fmt.Errorf("%w: stack %s", errUtils.ErrDuplicatePluginExtension, name)
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

// validateDiscoveredManifests checks provider definitions against the same manifest schema
// as YAML stack manifests, so an invalid discovered field fails `atmos validate stacks`.
func validateDiscoveredManifests(config *schema.AtmosConfiguration, schemaPath string) ([]string, error) {
	if config.ComponentConfigurations == nil {
		return nil, nil
	}
	discovered, err := config.ComponentConfigurations.Discover(context.Background())
	if err != nil {
		return nil, err
	}
	compiled, err := compiledManifestSchema(schemaPath)
	if err != nil {
		return nil, err
	}
	var messages []string
	for _, name := range slices.Sorted(maps.Keys(discovered)) {
		data, err := u.ConvertToJSONFast(discovered[name])
		if err != nil {
			return nil, err
		}
		instance, err := u.ConvertFromJSON(data)
		if err != nil {
			return nil, err
		}
		if err := compiled.Validate(instance); err != nil {
			messages = append(messages, fmt.Sprintf("Atmos manifest JSON Schema validation error in the discovered stack '%s':\n%v", name, err))
		}
	}
	return messages, nil
}

// compiledManifestSchema shares the stack processor's compiled-schema cache.
func compiledManifestSchema(path string) (*jsonschema.Schema, error) {
	if compiled, found := getCachedCompiledSchema(path); found {
		return compiled, nil
	}
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource(path, reader); err != nil {
		return nil, err
	}
	compiled, err := compiler.Compile(path)
	if err != nil {
		return nil, err
	}
	cacheCompiledSchema(path, compiled)
	return compiled, nil
}
