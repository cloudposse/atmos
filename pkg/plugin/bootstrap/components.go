package bootstrap

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/component/provideroptions"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	authadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	componentadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	configurationadapter "github.com/cloudposse/atmos/pkg/plugin/adapter/configuration"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/install"
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
		return nil, missingProviderError(references)
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

// ComponentOptions reads the static option declarations of the enabled configuration providers from
// their package manifests. It never installs, restores, verifies, or starts a package, so command
// registration and help stay cheap. A package that is not installed yet contributes nothing; the
// command that uses the provider reports that instead.
func ComponentOptions(ctx context.Context, atmos *schema.AtmosConfiguration) ([]provideroptions.Provider, error) {
	defer perf.Track(atmos, "pluginbootstrap.ComponentOptions")()

	wanted := enabledSections(atmos)
	if len(wanted) == 0 {
		return nil, nil
	}
	bindings, err := declaredComponentBindings(ctx, atmos, wanted)
	if err != nil {
		return nil, err
	}
	var providers []provideroptions.Provider
	for _, binding := range bindings {
		declared, err := bindingProviders(atmos, &binding, wanted[binding.Name])
		if err != nil {
			return nil, err
		}
		providers = append(providers, declared...)
	}
	return providers, nil
}

// enabledSections lists the enabled configuration sections by the provider they use.
func enabledSections(atmos *schema.AtmosConfiguration) map[string][]string {
	wanted := make(map[string][]string)
	for section, settings := range atmos.Components.Terraform.ConfigurationProviders {
		if !settings.Enabled {
			continue
		}
		name := section
		if settings.Provider != "" {
			name = settings.Provider
		}
		wanted[name] = append(wanted[name], section)
	}
	return wanted
}

// bindingProviders turns one discovery export into a provider for each section that runs a mode it supports.
func bindingProviders(atmos *schema.AtmosConfiguration, binding *extension.Binding, sections []string) ([]provideroptions.Provider, error) {
	options, err := provideroptions.Parse(binding.Name, binding.Metadata["options"])
	if err != nil {
		return nil, err
	}
	modes, err := extension.ComponentModes(binding.Metadata)
	if err != nil {
		return nil, err
	}
	var providers []provideroptions.Provider
	for _, section := range slices.Sorted(slices.Values(sections)) {
		mode := atmos.Components.Terraform.ConfigurationProviders[section].Mode
		if mode == "" {
			mode = provideroptions.ModeNative
		}
		if slices.Contains(modes, mode) {
			providers = append(providers, provideroptions.Provider{Section: section, Name: binding.Name, Mode: mode, Options: options})
		}
	}
	return providers, nil
}

// RegisterComponentFlags adds the options of the enabled configuration providers to the subcommands
// of root they apply to. Declaration and collision problems are returned; a package that cannot be
// read is skipped here because the command that uses the provider reports it when it activates.
func RegisterComponentFlags(ctx context.Context, atmos *schema.AtmosConfiguration, root *cobra.Command) error {
	defer perf.Track(atmos, "pluginbootstrap.RegisterComponentFlags")()

	providers, err := ComponentOptions(ctx, atmos)
	if err != nil {
		if errors.Is(err, errUtils.ErrProviderOption) {
			return err
		}
		log.Debug("Skipped provider flag registration", "error", err)
		return nil
	}
	return provideroptions.Register(root, providers)
}

// declaredComponentBindings lists the discovery exports of the wanted providers from installed manifests.
func declaredComponentBindings(ctx context.Context, atmos *schema.AtmosConfiguration, wanted map[string][]string) ([]extension.Binding, error) {
	state, err := readSelections(ctx, atmos)
	if err != nil {
		return nil, err
	}
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	if err != nil {
		return nil, err
	}
	cache, err := install.NewCache(atmos.Plugins.CacheDir)
	if err != nil {
		return nil, err
	}
	registry, err := Descriptors()
	if err != nil {
		return nil, err
	}
	lookup := componentBindingLookup{atmos: atmos, state: state, file: file, cache: cache, registry: registry, wanted: wanted}
	var result []extension.Binding
	for _, alias := range slices.Sorted(maps.Keys(atmos.Plugins.Packages)) {
		bindings, err := lookup.packageBindings(alias)
		if err != nil {
			return nil, err
		}
		result = append(result, bindings...)
	}
	return result, nil
}

type componentBindingLookup struct {
	atmos    *schema.AtmosConfiguration
	state    *config.Selections
	file     *pluginlock.File
	cache    *install.Cache
	registry *extension.Registry
	wanted   map[string][]string
}

func (lookup componentBindingLookup) packageBindings(alias string) ([]extension.Binding, error) {
	declaration := lookup.atmos.Plugins.Packages[alias]
	if !enabled(&declaration) || !packageExportsWanted(alias, lookup.state, lookup.file, lookup.wanted) {
		return nil, nil
	}
	directory := packageDirectory(alias, lookup.state, lookup.file, lookup.cache)
	if directory == "" {
		return nil, nil
	}
	bindings, err := discoveryBindings(lookup.registry, directory, lookup.wanted)
	if errors.Is(err, errUtils.ErrProviderOption) {
		return nil, err
	}
	if err != nil {
		// One unreadable package must not take every other provider's flags with it.
		log.Debug("Skipped plugin package while reading provider options", "package", alias, "error", err)
		return nil, nil
	}
	return bindings, nil
}

func packageExportsWanted(alias string, state *config.Selections, file *pluginlock.File, wanted map[string][]string) bool {
	if _, local := state.Packages[alias]; local {
		return true // Local packages have no lock exports; inspect their manifest.
	}
	entry, locked := file.Spec.Packages[alias]
	if !locked {
		return false
	}
	for _, export := range entry.Exports {
		if export.Capability == extension.ComponentDiscovery && wanted[export.Name] != nil {
			return true
		}
	}
	return false
}

// discoveryBindings reads a package manifest and keeps the discovery exports of the wanted providers.
func discoveryBindings(registry *extension.Registry, directory string, wanted map[string][]string) ([]extension.Binding, error) {
	definition, err := manifest.Load(directory)
	if err != nil {
		return nil, err
	}
	bindings, err := registry.Discover(definition.Spec.Capabilities)
	if err != nil {
		return nil, err
	}
	var result []extension.Binding
	for _, binding := range bindings {
		if binding.Capability == extension.ComponentDiscovery && wanted[binding.Name] != nil {
			result = append(result, binding)
		}
	}
	return result, nil
}

// packageDirectory returns where an installed package lives, or empty when it is not installed.
func packageDirectory(alias string, state *config.Selections, file *pluginlock.File, cache *install.Cache) string {
	if local, exists := state.Packages[alias]; exists {
		return local.Path
	}
	entry, locked := file.Spec.Packages[alias]
	if !locked {
		return ""
	}
	directory, err := cache.DiscoveryDirectory(&entry, install.CurrentPlatform())
	if err != nil {
		return ""
	}
	return directory
}

func requireComponentReferences(registry *componentadapter.Registry, references []ComponentReference) error {
	for _, reference := range references {
		provider, exists := registry.Get(reference.Provider)
		if !exists || !provider.Supports(reference.Capability) {
			return missingProviderError([]ComponentReference{reference})
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
			return false, errUtils.Build(errUtils.ErrDuplicatePluginExtension).
				WithExplanationf("Plugin packages `%s` and `%s` both provide `%s` (%s).", owner, alias, reference.Provider, reference.Capability).
				WithContext("provider", reference.Provider).
				WithHint("Keep one of them under `plugins.packages`, or disable the other with `enabled: false`.").
				Err()
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

// missingProviderError explains that an enabled configuration provider has no plugin behind it and how
// to supply one.
func missingProviderError(references []ComponentReference) error {
	names := make([]string, 0, len(references))
	for _, reference := range references {
		if !slices.Contains(names, reference.Provider) {
			names = append(names, reference.Provider)
		}
	}
	slices.Sort(names)
	provider := strings.Join(names, ", ")
	return errUtils.Build(errUtils.ErrPluginUnsupportedExtension).
		WithExplanationf("`components.terraform.%s.enabled` is true, but no installed plugin package provides the `%s` configuration provider.", names[0], provider).
		WithContext("provider", provider).
		WithHint("Declare the package under `plugins.packages` with `source` and `version`, or `path` for a local build, then check it with `atmos plugin list`.").
		WithHint("If the project ships a profile that supplies a local build, such as the Terragrunt example's `dev` profile, activate it with `--profile` or `ATMOS_PROFILE`.").
		WithHintf("To stop using the provider, set `components.terraform.%s.enabled` to false.", names[0]).
		Err()
}

func init() {
	// Register the constructor, never an invocation's provider or discovered stack map.
	if err := configuration.Register("plugin", newConfigurationSession); err != nil {
		panic(err)
	}
}

func newConfigurationSession(ctx context.Context, config *schema.AtmosConfiguration, operation string) (schema.ComponentConfigurationRuntime, error) {
	snapshot := *config
	return configurationadapter.New(ctx, config, operation, func(call context.Context, names []string) (map[string]configurationadapter.Provider, error) {
		references := make([]ComponentReference, 0, len(names))
		for _, name := range names {
			references = append(references, ComponentReference{Provider: name, Capability: extension.ComponentDiscovery})
		}
		registry, err := LoadComponents(call, &snapshot, references)
		if err != nil {
			return nil, err
		}
		providers := make(map[string]configurationadapter.Provider)
		for _, name := range registry.Names() {
			providers[name], _ = registry.Get(name)
		}
		return providers, nil
	})
}
