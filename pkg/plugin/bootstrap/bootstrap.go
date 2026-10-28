package bootstrap

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/plugin/secrets"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const sessionDirectoryMode = 0o700

// ProjectDirectory locates project-local plugin state using resolved config provenance.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func ProjectDirectory(config *schema.AtmosConfiguration) string {
	if config.BasePathConfigDir != "" {
		return config.BasePathConfigDir
	}
	if config.BasePath != "" {
		return config.BasePath
	}
	return "."
}

// Prepare restores the project packages the configuration needs (see neededPackages) and
// registers their exports before auth initialization. Native binaries launch lazily when their services run.
func Prepare(ctx context.Context, atmos *schema.AtmosConfiguration, options ...Option) (func(), error) {
	defer perf.Track(atmos, "pluginbootstrap.Prepare")()

	temporary, err := PrepareAvailable(ctx, atmos, options...)
	if err != nil {
		return nil, err
	}
	defer temporary()
	state, err := selectedForUse(ctx, atmos)
	if err != nil {
		return nil, err
	}
	temporary()
	return registerSelected(ctx, atmos, state, options...)
}

func registerSelected(ctx context.Context, atmos *schema.AtmosConfiguration, state *config.Selections, options ...Option) (func(), error) {
	if len(state.Packages) == 0 {
		return func() {}, nil
	}
	announceLocal(atmos, state)
	keyring, err := credentials.OpenKeyring(&atmos.Auth)
	if err != nil {
		return nil, err
	}
	locks, err := xdg.GetXDGCacheDir(filepath.Join("plugins", "sessions"), sessionDirectoryMode)
	if err != nil {
		return nil, err
	}
	services := adapter.Services{Secrets: secrets.Factory(atmos), Keyring: keyring, CredentialStore: credentials.NewCredentialStoreWithConfig(&atmos.Auth), LockDirectory: locks}
	for _, option := range options {
		option(&services)
	}
	constructors := newConstructors(&services)
	registry, err := descriptors(constructors)
	if err != nil {
		return nil, err
	}
	if err := activatePackages(ctx, state, registry, atmos.Plugins.Packages); err != nil {
		return nil, err
	}

	return authregistry.RegisterBatch(constructors.Batch)
}

func validateExecutable(directory, executable string) error {
	if !manifest.LocalPath(executable) {
		return errUtils.ErrPluginInvalidConfig
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	defer root.Close()
	file, err := root.Open(executable)
	if err != nil {
		return errors.Join(errUtils.ErrPluginNotInstalled, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	if !info.Mode().IsRegular() {
		return errUtils.ErrPluginInvalidConfig
	}
	return nil
}

func activatePackages(ctx context.Context, state *config.Selections, registry *extension.Registry, declarations map[string]schema.PluginPackage) error {
	for _, alias := range slices.Sorted(maps.Keys(state.Packages)) {
		entry := state.Packages[alias]
		timeout, err := config.PackageTimeout(declarations[alias])
		if err != nil {
			return err
		}
		if err := activatePackageWithTimeout(ctx, alias, &entry, registry, timeout); err != nil {
			return err
		}
	}
	return nil
}

func activatePackageWithTimeout(ctx context.Context, alias string, entry *config.Selection, registry *extension.Registry, timeout time.Duration) error {
	definition, err := loadLocalManifest(alias, entry)
	if err != nil {
		return err
	}
	exports, err := registry.Validate(entry.Path, definition.Spec.Capabilities)
	if err != nil {
		return repairLocal(alias, entry, err)
	}
	executable := selectedExecutable(entry, definition)
	if err := validateExecutable(entry.Path, executable); err != nil {
		return repairLocal(alias, entry, err)
	}
	expected, err := DescribePackage(entry.Path, definition, exports)
	if err != nil {
		return repairLocal(alias, entry, err)
	}
	for _, binding := range exports {
		descriptor, exists := registry.Get(binding.Capability)
		if !exists || descriptor.Activate == nil {
			return errUtils.ErrPluginUnsupportedExtension
		}
		if err := descriptor.Activate(ctx, &extension.Activation{Binding: binding, Package: definition.Metadata.Name, Executable: filepath.Join(binding.Directory, executable), Description: expected, Timeout: timeout}); err != nil {
			return err
		}
	}
	return nil
}
