package manage

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	"github.com/cloudposse/atmos/pkg/schema"
	yamledit "github.com/cloudposse/atmos/pkg/yaml"
)

// EditRegistry changes catalog configuration independently of registry credentials.
func (m *Manager) EditRegistry(ctx context.Context, alias string, registry *schema.PluginRegistry) error {
	defer perf.Track(m.Config, "pluginmanage.Manager.EditRegistry")()

	if !aliasPattern.MatchString(alias) {
		return errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanationf("%q is not a valid registry alias.", alias).
			WithHint("Use lowercase letters, digits, hyphens, and underscores, starting with a letter, for example `corporate`.").Err()
	}
	if err := m.validateRegistry(ctx, alias, registry); err != nil {
		return err
	}
	return config.Mutate(ctx, m.Project, func() ([]config.Change, error) {
		before, err := readOptional(filepath.Join(m.Project, m.ConfigFile))
		if err != nil {
			return nil, err
		}
		if _, found := config.EffectiveRegistries(&m.Config.Plugins)[alias]; registry == nil && !found {
			return nil, RegistryNotFound(m.Config, alias)
		}
		after, err := registryChange(before, alias, registry)
		return []config.Change{{Name: m.ConfigFile, Before: before, After: after}}, err
	})
}

func (m *Manager) validateRegistry(ctx context.Context, alias string, registry *schema.PluginRegistry) error {
	if registry == nil {
		return nil
	}
	host, err := config.RegistryHost(registry)
	if err != nil {
		return err
	}
	sources := config.RegistrySources(registry)
	if len(sources) == 0 {
		if err := pluginpolicy.Login(ctx, host); err != nil {
			return err
		}
	}
	for _, source := range sources {
		if err := pluginpolicy.Source(ctx, "registry.configure", source); err != nil {
			return err
		}
	}
	proposed := *m.Config
	proposed.Plugins.Registries = maps.Clone(m.Config.Plugins.Registries)
	if proposed.Plugins.Registries == nil {
		proposed.Plugins.Registries = make(map[string]schema.PluginRegistry)
	}
	proposed.Plugins.Registries[alias] = *registry
	if _, err := registryauth.PluginBindings(&proposed); err != nil {
		return err
	}
	return nil
}

// RegistryNotFound reports an unknown registry alias and lists the configured ones.
func RegistryNotFound(atmos *schema.AtmosConfiguration, alias string) error {
	defer perf.Track(nil, "manage.RegistryNotFound")()

	builder := errUtils.Build(errUtils.ErrPluginRegistryNotFound).
		WithExplanationf("No plugin registry named %q is configured.", alias)
	var names []string
	if atmos != nil {
		names = slices.Sorted(maps.Keys(config.EffectiveRegistries(&atmos.Plugins)))
	}
	if len(names) == 0 {
		return builder.WithHint("No registries are configured yet. Add one with `atmos plugin registry add <name> <catalog>`.").Err()
	}
	return builder.WithHintf("Configured registries: %s.", strings.Join(names, ", ")).Err()
}

// ResolveRegistryHost maps a configured registry alias or an explicit registry host to the
// host that credentials are scoped to. A bare word that is neither a configured alias nor
// shaped like a host (no dot, port, or localhost) is treated as a mistyped alias.
func ResolveRegistryHost(atmos *schema.AtmosConfiguration, value string) (string, error) {
	defer perf.Track(atmos, "pluginmanage.ResolveRegistryHost")()

	if atmos == nil {
		return "", errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("Atmos configuration is not loaded.").
			WithHint("Run this command from a project with an `atmos.yaml`.").Err()
	}
	if configured, found := config.EffectiveRegistries(&atmos.Plugins)[value]; found {
		return config.RegistryHost(&configured)
	}
	if !strings.ContainsAny(value, ".:") && value != "localhost" {
		return "", RegistryNotFound(atmos, value)
	}
	registry, err := name.NewRegistry(value, name.StrictValidation)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrPluginInvalidConfig).WithCause(err).
			WithExplanationf("%q is neither a configured registry alias nor a valid registry host.", value).
			WithHint("Pass a registry host such as `ghcr.io` or `localhost:5000`, or a configured alias.").Err()
	}
	return registry.Name(), nil
}

// ResolveCatalogReference maps a configured registry alias or an explicit oci:// reference to a
// catalog reference. Anything else is rejected with the aliases that would have worked.
func ResolveCatalogReference(atmos *schema.AtmosConfiguration, value string) (string, error) {
	defer perf.Track(atmos, "pluginmanage.ResolveCatalogReference")()

	if atmos == nil {
		return "", errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("Atmos configuration is not loaded.").
			WithHint("Run this command from a project with an `atmos.yaml`.").Err()
	}
	if configured, found := config.EffectiveRegistries(&atmos.Plugins)[value]; found {
		if configured.Catalog == "" {
			return "", errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanationf("Discovery source %q does not configure a catalog.", value).
				WithHint("Use an explicit `oci://` catalog reference for catalog maintenance, or set `plugins.registries." + value + ".catalog`.").Err()
		}
		return configured.Catalog, nil
	}
	if strings.HasPrefix(value, "oci://") {
		return value, nil
	}
	builder := errUtils.Build(errUtils.ErrPluginInvalidConfig).
		WithExplanationf("%q is neither a configured registry alias nor an OCI reference.", value)
	var aliases []string
	for alias, registry := range config.EffectiveRegistries(&atmos.Plugins) {
		if registry.Catalog != "" {
			aliases = append(aliases, alias)
		}
	}
	slices.Sort(aliases)
	if len(aliases) == 0 {
		return "", builder.WithHint("Prefix the reference with `oci://`, or configure a catalog with `atmos plugin registry add <name> <catalog>`.").Err()
	}
	return "", builder.WithHintf("Either prefix with `oci://` or use a configured alias: %s.", strings.Join(aliases, ", ")).Err()
}

// registryKeys lists every key of a registry entry, so a replacement can drop the stale ones.
var registryKeys = []string{"host", "catalog", "identity", "auth", "repositories"}

// registryChange edits only the registry entry's keys, so comments, blank lines, and block style
// elsewhere in the user's configuration survive. A nil registry deletes the entry.
func registryChange(before []byte, alias string, registry *schema.PluginRegistry) ([]byte, error) {
	path := []string{"plugins", "registries", alias}
	if registry == nil {
		after, err := config.DeleteKey(before, path)
		if err != nil || alias != config.DefaultRegistryAlias {
			return after, err
		}
		return yamledit.SetRaw(after, "plugins.discovery.default_registry", "false")
	}
	fields := []config.Field{}
	set := map[string]bool{}
	add := func(key, value string) {
		if value != "" {
			fields = append(fields, config.Field{Key: key, Value: value})
			set[key] = true
		}
	}
	add("host", registry.Host)
	add("catalog", registry.Catalog)
	add("identity", registry.Identity)
	add("auth", registry.Auth)
	if len(registry.Repositories) > 0 {
		fields = append(fields, config.Field{Key: "repositories", Items: registry.Repositories})
		set["repositories"] = true
	}
	after := before
	var err error
	for _, key := range registryKeys {
		if set[key] {
			continue
		}
		if after, err = config.DeleteKey(after, append(slices.Clone(path), key)); err != nil {
			return nil, err
		}
	}
	return config.SetFields(after, path, fields)
}
