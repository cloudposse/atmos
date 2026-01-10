package toolchain

import (
	"path/filepath"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain/installer"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// Tool is a type alias for registry.Tool for backward compatibility.
// New code should import and use toolchain/registry.Tool directly.
type Tool = registry.Tool

// File is a type alias for registry.File for backward compatibility.
type File = registry.File

// Override is a type alias for registry.Override for backward compatibility.
type Override = registry.Override

// Installer is a type alias for installer.Installer for backward compatibility.
// New code should import and use toolchain/installer.Installer directly.
type Installer = installer.Installer

// ToolResolver is a type alias for installer.ToolResolver for backward compatibility.
type ToolResolver = installer.ToolResolver

// DefaultToolResolver is a type alias for installer.DefaultToolResolver for backward compatibility.
type DefaultToolResolver = installer.DefaultToolResolver

// RegistryFactory is a type alias for installer.RegistryFactory for backward compatibility.
type RegistryFactory = installer.RegistryFactory

// Option is a type alias for installer.Option for backward compatibility.
type Option = installer.Option

// realRegistryFactory creates real Aqua registries.
// This is used to inject a working registry factory into the installer.
type realRegistryFactory struct{}

func (r *realRegistryFactory) NewAquaRegistry() registry.ToolRegistry {
	defer perf.Track(nil, "toolchain.realRegistryFactory.NewAquaRegistry")()

	return NewAquaRegistry()
}

// NewInstaller creates a new Installer with the given options.
// This is a wrapper around installer.New() for backward compatibility.
// It automatically injects the real registry factory and loads registries
// from atmos.yaml configuration.
func NewInstaller(opts ...Option) *Installer {
	defer perf.Track(nil, "toolchain.NewInstaller")()

	// Set binDir from config (GetInstallPath() + "/bin").
	binDir := filepath.Join(GetInstallPath(), "bin")

	// Build base options: binDir and registry factory first (so user opts can override).
	baseOpts := []Option{
		WithBinDir(binDir),
		WithRegistryFactory(&realRegistryFactory{}),
	}

	// Try to load configured registries from atmos.yaml.
	if config := GetAtmosConfig(); config != nil {
		log.Debug("Loading toolchain registries from atmos.yaml",
			"registryCount", len(config.Toolchain.Registries))
		if reg, err := NewRegistry(config); err != nil {
			log.Warn("Failed to load configured registry from atmos.yaml", "error", err)
		} else if reg != nil {
			log.Debug("Successfully loaded configured registry from atmos.yaml")
			baseOpts = append(baseOpts, WithConfiguredRegistry(reg))
		}
	} else {
		log.Debug("No atmos config available for toolchain registry loading")
	}

	// User options come last (highest priority).
	baseOpts = append(baseOpts, opts...)
	return installer.New(baseOpts...)
}

// NewInstallerWithResolver creates an installer with a custom resolver.
// Deprecated: Use NewInstaller() with WithResolver() option instead.
func NewInstallerWithResolver(resolver ToolResolver, binDir string) *Installer {
	defer perf.Track(nil, "toolchain.NewInstallerWithResolver")()

	return installer.NewInstallerWithResolver(resolver, binDir)
}

// WithBinDir sets the binary installation directory.
func WithBinDir(binDir string) Option {
	defer perf.Track(nil, "toolchain.WithBinDir")()

	return installer.WithBinDir(binDir)
}

// WithCacheDir sets the cache directory.
func WithCacheDir(cacheDir string) Option {
	defer perf.Track(nil, "toolchain.WithCacheDir")()

	return installer.WithCacheDir(cacheDir)
}

// WithResolver sets the tool resolver.
func WithResolver(resolver ToolResolver) Option {
	defer perf.Track(nil, "toolchain.WithResolver")()

	return installer.WithResolver(resolver)
}

// WithConfiguredRegistry sets a pre-configured registry.
func WithConfiguredRegistry(reg registry.ToolRegistry) Option {
	defer perf.Track(nil, "toolchain.WithConfiguredRegistry")()

	return installer.WithConfiguredRegistry(reg)
}

// WithRegistryFactory sets the factory for creating registry instances.
func WithRegistryFactory(factory RegistryFactory) Option {
	defer perf.Track(nil, "toolchain.WithRegistryFactory")()

	return installer.WithRegistryFactory(factory)
}

// BuiltinAliases are always available and can be overridden in atmos.yaml.
var BuiltinAliases = installer.BuiltinAliases

// NewInstallerWithBinDir creates an installer with a specific bin directory.
// This is a convenience wrapper for testing.
func NewInstallerWithBinDir(binDir string) *Installer {
	defer perf.Track(nil, "toolchain.NewInstallerWithBinDir")()

	return NewInstaller(WithBinDir(binDir))
}
