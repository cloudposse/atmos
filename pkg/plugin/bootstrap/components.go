package bootstrap

import (
	"context"
	"fmt"
	"maps"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	authadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	componentadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentReference requests one independently negotiated configuration capability.
type ComponentReference struct{ Provider, Capability string }

// LoadComponents restores only referenced configuration providers and composes a
// private registry. Package installation alone never calls discovery or starts a
// native process. The caller supplies references from explicitly enabled config.
func LoadComponents(ctx context.Context, atmos *schema.AtmosConfiguration, references []ComponentReference, options ...Option) (*componentadapter.Registry, error) {
	defer perf.Track(atmos, "pluginbootstrap.LoadComponents")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(references) == 0 {
		return componentadapter.NewRegistry(nil)
	}
	if atmos == nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	if err := validComponentReferences(references); err != nil {
		return nil, err
	}
	selected, err := componentPackages(ctx, atmos, references)
	if err != nil {
		return nil, err
	}
	if len(selected.Plugins.Packages) == 0 {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	temporary, err := PrepareAvailable(ctx, atmos, options...)
	if err != nil {
		return nil, err
	}
	defer temporary()
	registry, err := activateComponentPackages(ctx, selected)
	if err != nil {
		return nil, err
	}
	if err := requireComponentReferences(registry, references); err != nil {
		return nil, err
	}
	return registry, nil
}

func requireComponentReferences(registry *componentadapter.Registry, references []ComponentReference) error {
	for _, reference := range references {
		provider, exists := registry.Get(reference.Provider)
		if !exists || !provider.Supports(reference.Capability) {
			return fmt.Errorf("%w: %s (%s)", errUtils.ErrPluginUnsupportedExtension, reference.Provider, reference.Capability)
		}
	}
	return nil
}

func activateComponentPackages(ctx context.Context, atmos *schema.AtmosConfiguration) (*componentadapter.Registry, error) {
	state, err := selectedPackages(ctx, atmos)
	if err != nil {
		return nil, err
	}
	announceLocal(atmos, state)
	constructors := newConstructors(&authadapter.Services{})
	descriptors, err := descriptors(constructors)
	if err != nil {
		return nil, err
	}
	if err := activatePackages(ctx, state, descriptors, atmos.Plugins.Packages); err != nil {
		return nil, err
	}
	registry, err := componentadapter.NewRegistry(constructors.Components)
	if err != nil {
		return nil, err
	}
	return registry, nil
}

func validComponentReferences(references []ComponentReference) error {
	for _, reference := range references {
		if reference.Provider == "" || !extension.IsComponent(reference.Capability) {
			return errUtils.ErrPluginInvalidConfig
		}
	}
	return nil
}

func componentPackages(ctx context.Context, atmos *schema.AtmosConfiguration, references []ComponentReference) (*schema.AtmosConfiguration, error) {
	state, err := readSelections(ctx, atmos)
	if err != nil {
		return nil, err
	}
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	if err != nil {
		return nil, err
	}
	selected := *atmos
	selected.Plugins.Packages = make(map[string]schema.PluginPackage)
	owners := make(map[ComponentReference]string)
	var unlocked []string
	for _, alias := range slices.Sorted(maps.Keys(atmos.Plugins.Packages)) {
		declaration := atmos.Plugins.Packages[alias]
		if !enabled(&declaration) {
			continue
		}
		exports, known, err := componentPackageExports(alias, state, file)
		if err != nil {
			return nil, err
		}
		if !known {
			unlocked = append(unlocked, alias)
			continue
		}
		matched, err := selectComponentOwner(alias, references, exports, owners)
		if err != nil {
			return nil, err
		}
		if matched {
			selected.Plugins.Packages[alias] = declaration
		}
	}
	selectUnresolvedComponents(atmos, &selected, references, owners, unlocked)
	return &selected, nil
}

func selectUnresolvedComponents(atmos, selected *schema.AtmosConfiguration, references []ComponentReference, owners map[ComponentReference]string, unlocked []string) {
	for _, reference := range references {
		if owners[reference] != "" {
			continue
		}
		for _, alias := range unlocked {
			selected.Plugins.Packages[alias] = atmos.Plugins.Packages[alias]
		}
		return
	}
}

func selectComponentOwner(alias string, references []ComponentReference, exports map[ComponentReference]bool, owners map[ComponentReference]string) (bool, error) {
	matched := false
	for _, reference := range references {
		if !exports[reference] {
			continue
		}
		if owner := owners[reference]; owner != "" && owner != alias {
			return false, errUtils.ErrDuplicatePluginExtension
		}
		owners[reference] = alias
		matched = true
	}
	return matched, nil
}

func componentPackageExports(alias string, state *config.Selections, file *pluginlock.File) (map[ComponentReference]bool, bool, error) {
	result := make(map[ComponentReference]bool)
	if local, exists := state.Packages[alias]; exists {
		definition, err := manifest.Load(local.Path)
		if err != nil {
			return nil, true, err
		}
		registry, err := Descriptors()
		if err != nil {
			return nil, true, err
		}
		bindings, err := registry.Discover(definition.Spec.Capabilities)
		if err != nil {
			return nil, true, err
		}
		for _, binding := range bindings {
			result[ComponentReference{Provider: binding.Name, Capability: binding.Capability}] = true
		}
		return result, true, nil
	}
	entry, exists := file.Spec.Packages[alias]
	for _, export := range entry.Exports {
		result[ComponentReference{Provider: export.Name, Capability: export.Capability}] = true
	}
	return result, exists, nil
}
