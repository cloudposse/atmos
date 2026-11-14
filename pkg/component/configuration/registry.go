// Package configuration composes alternate configuration formats with built-in component engines.
package configuration

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Factory constructs an invocation-local session. Registration stores no invocation state.
type Factory func(context.Context, *schema.AtmosConfiguration, string) (schema.ComponentConfigurationRuntime, error)

var factories = struct {
	sync.RWMutex
	values map[string]Factory
}{values: make(map[string]Factory)}

// Register adds a configuration service factory during host composition.
func Register(name string, factory Factory) error {
	defer perf.Track(nil, "configuration.Register")()

	if name == "" || factory == nil {
		return errUtils.ErrPluginInvalidConfig
	}
	factories.Lock()
	defer factories.Unlock()
	if _, exists := factories.values[name]; exists {
		return errUtils.ErrDuplicatePluginExtension
	}
	factories.values[name] = factory
	return nil
}

// Attach creates private state only when a configuration provider is enabled.
func Attach(ctx context.Context, config *schema.AtmosConfiguration, operation string) error {
	defer perf.Track(config, "configuration.Attach")()

	if !config.HasComponentConfiguration() {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	factories.RLock()
	factory := factories.values["plugin"]
	factories.RUnlock()
	if factory == nil {
		return errUtils.ErrPluginUnsupportedExtension
	}
	session, err := factory(ctx, config, operation)
	if err != nil {
		return err
	}
	config.ComponentConfigurations = session
	return nil
}

// MatchDependencies returns the dependencies to compare against a change set.
// Provider watches are rooted at BasePathAbsolute, and base-path resolution
// can spell that root differently from the repository root the changed files
// are joined to: through a symlinked temp dir on macOS (/var vs /private/var)
// or as an 8.3 short name on Windows (RUNNER~1 vs runneradmin). Two spellings
// of one directory never match, so provider watches are canonicalized with
// resolve, which the caller also applies to the change set. YAML dependencies
// keep exact matching and are returned unchanged.
func MatchDependencies(deps []schema.ComponentDependency, provider bool, resolve func(string) string) []schema.ComponentDependency {
	defer perf.Track(nil, "configuration.MatchDependencies")()

	if !provider || resolve == nil {
		return deps
	}
	resolved := slices.Clone(deps)
	for i := range resolved {
		resolved[i].Path = resolve(resolved[i].Path)
	}
	return resolved
}

// ConfiguredDependencyPath reports a matched dependency in its configured spelling.
// The matched slice must come from MatchDependencies for the same deps.
//
//nolint:lintroller // Pure lookup or validation; exempt from performance tracking.
func ConfiguredDependencyPath(deps, matched []schema.ComponentDependency, path string) string {
	for i := range matched {
		if matched[i].Path == path && i < len(deps) {
			return deps[i].Path
		}
	}
	return path
}

// ValidateDiscoveredManifests checks provider definitions against the same manifest
// schema as YAML stack manifests, so an invalid discovered field fails validation.
// The schema is compiled lazily through compile, which lets the caller share its
// compiled-schema cache; nothing is compiled when no stack was discovered.
func ValidateDiscoveredManifests(discovered map[string]map[string]any, compile func() (*jsonschema.Schema, error)) ([]string, error) {
	defer perf.Track(nil, "configuration.ValidateDiscoveredManifests")()

	if len(discovered) == 0 {
		return nil, nil
	}
	compiled, err := compile()
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

// CompileManifestSchema compiles the Atmos manifest JSON schema at path.
func CompileManifestSchema(path string) (*jsonschema.Schema, error) {
	defer perf.Track(nil, "configuration.CompileManifestSchema")()

	reader, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: manifest schema %s: %w", errUtils.ErrOpenFile, path, err)
	}
	defer reader.Close()
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource(path, reader); err != nil {
		return nil, fmt.Errorf("%w: register manifest schema %s: %w", errUtils.ErrStackManifestSchemaValidation, path, err)
	}
	compiled, err := compiler.Compile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: compile manifest schema %s: %w", errUtils.ErrStackManifestSchemaValidation, path, err)
	}
	return compiled, nil
}
