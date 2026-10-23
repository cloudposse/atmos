// Package manifest defines the statically readable plugin package contract.
package manifest

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
	"github.com/cloudposse/atmos/pkg/version"
)

const Kind = "AtmosPluginConfig"

const maxManifestBytes = 8 << 20

var packageName = regexp.MustCompile(`^[a-z][a-z0-9-]*/[a-z][a-z0-9-]*$`)

// Config is a versioned Atmos manifest; loading it never executes plugin code.
type Config = manifest.Manifest[Spec]

// Spec describes package runtime, platforms, compatibility, and extension metadata.
type Spec struct {
	Runtime       Runtime             `yaml:"runtime" json:"runtime"`
	Platforms     map[string]Platform `yaml:"platforms,omitempty" json:"platforms,omitempty"`
	Compatibility Compatibility       `yaml:"compatibility" json:"compatibility"`
	Capabilities  map[string]any      `yaml:"capabilities" json:"capabilities"`
	ConfigSchema  string              `yaml:"config_schema,omitempty" json:"config_schema,omitempty"`
}

// Runtime selects the native protocol and default development executable.
type Runtime struct {
	Type       string `yaml:"type" json:"type"`
	Protocol   uint32 `yaml:"protocol" json:"protocol"`
	Executable string `yaml:"executable" json:"executable"`
}

// Platform binds a precompiled executable to a digest.
type Platform struct {
	Executable string `yaml:"executable" json:"executable"`
	SHA256     string `yaml:"sha256,omitempty" json:"sha256,omitempty"`
}

// Compatibility declares ranges consumed without starting the executable.
type Compatibility struct {
	Atmos     string            `yaml:"atmos,omitempty" json:"atmos,omitempty"`
	Protocol  string            `yaml:"protocol" json:"protocol"`
	Contracts map[string]string `yaml:"contracts,omitempty" json:"contracts,omitempty"`
}

func init() { manifest.MustRegister(Kind, manifest.DefaultAPIVersion, &Spec{}) }

// Load reads a package manifest through a confined filesystem root.
func Load(directory string) (*Config, error) {
	defer perf.Track(nil, "pluginmanifest.Load")()

	raw, err := ReadFile(directory, "plugin.yaml")
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// Parse validates package metadata without opening files or executing code.
func Parse(raw []byte) (*Config, error) {
	defer perf.Track(nil, "manifest.Parse")()

	result, err := manifest.Load[Spec](Kind, raw)
	if err != nil {
		return nil, err
	}
	if !packageName.MatchString(result.Metadata.Name) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	if _, err := semver.StrictNewVersion(result.Metadata.Version); err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	if result.Spec.Runtime.Type != "native" || result.Spec.Runtime.Protocol != 1 || !LocalPath(result.Spec.Runtime.Executable) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	if err := checkCompatibility(result.Spec.Compatibility); err != nil {
		return nil, err
	}
	return result, nil
}

// Executable selects the executable a local build runs on a platform such as darwin/arm64: the
// platform entry when the manifest declares a usable one, otherwise the runtime executable.
func Executable(definition *Config, platform string) string {
	defer perf.Track(nil, "pluginmanifest.Executable")()

	if selected, err := pluginplatform.Select(definition.Spec.Platforms, platform); err == nil {
		if entry := definition.Spec.Platforms[selected]; LocalPath(entry.Executable) {
			return entry.Executable
		}
	}
	return definition.Spec.Runtime.Executable
}

// LocalPath rejects absolute and escaping paths before package access.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func LocalPath(path string) bool { return path != "" && filepath.IsLocal(path) }

// ReadFile confines schema and manifest reads to the plugin package.
func ReadFile(directory, name string) ([]byte, error) {
	defer perf.Track(nil, "pluginmanifest.ReadFile")()

	if !LocalPath(name) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	if len(raw) > maxManifestBytes {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	return raw, nil
}

func checkCompatibility(compatibility Compatibility) error {
	if err := supportsV1(compatibility.Protocol); err != nil {
		return err
	}
	for _, expression := range compatibility.Contracts {
		if err := supportsV1(expression); err != nil {
			return err
		}
	}
	if compatibility.Atmos != "" {
		constraint, err := semver.NewConstraint(compatibility.Atmos)
		if err != nil {
			return errUtils.ErrPluginInvalidConfig
		}
		current, err := semver.NewVersion(strings.TrimPrefix(version.Version, "v"))
		if err != nil || !constraint.Check(current) {
			return errUtils.ErrPluginUnsupportedExtension
		}
	}
	return nil
}

func supportsV1(expression string) error {
	constraint, err := semver.NewConstraint(expression)
	if err != nil || expression == "" || !constraint.Check(semver.MustParse("1.0.0")) {
		return errUtils.ErrPluginUnsupportedExtension
	}
	return nil
}
