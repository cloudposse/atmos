// Package lock records reproducible plugin selections independently of local overrides.
package lock

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	atmosmanifest "github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

const (
	lockFileMode          = 0o644
	sha256ReferenceLength = 71
	sha256Bytes           = 32
)

const Kind = "AtmosPluginLock"

// File is a standard versioned Atmos manifest.
type File = atmosmanifest.Manifest[Spec]

// Spec records immutable release choices; timestamps never affect frozen equality.
type Spec struct {
	LockVersion  int              `yaml:"lock_version" json:"lock_version"`
	GeneratedAt  time.Time        `yaml:"generated_at" json:"generated_at"`
	AtmosVersion string           `yaml:"atmos_version" json:"atmos_version"`
	Packages     map[string]Entry `yaml:"packages" json:"packages"`
}

// Entry contains the source constraint and the exact release/platform digests.
type Entry struct {
	Plugin       string            `yaml:"plugin" json:"plugin"`
	Source       string            `yaml:"source" json:"source"`
	Constraint   string            `yaml:"constraint" json:"constraint"`
	Version      string            `yaml:"version" json:"version"`
	IndexDigest  string            `yaml:"index_digest" json:"index_digest"`
	Platforms    map[string]string `yaml:"platforms" json:"platforms"`
	Runtime      manifest.Runtime  `yaml:"runtime" json:"runtime"`
	Exports      []Export          `yaml:"exports" json:"exports"`
	Verification Verification      `yaml:"verification" json:"verification"`
}

// Export identifies one statically validated extension.
type Export struct {
	Capability string `yaml:"capability" json:"capability"`
	Name       string `yaml:"name" json:"name"`
}

// Verification distinguishes integrity from publisher identity.
type Verification struct {
	Signature string `yaml:"signature" json:"signature"`
	Identity  string `yaml:"identity,omitempty" json:"identity,omitempty"`
	Issuer    string `yaml:"issuer,omitempty" json:"issuer,omitempty"`
}

func init() { atmosmanifest.MustRegister(Kind, atmosmanifest.DefaultAPIVersion, &Spec{}) }

// Path returns the project lock location beside its root configuration.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func Path(project string) string { return filepath.Join(project, ".atmos.plugins.lock.yaml") }

// New creates an empty release lock.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func New() *File {
	return &File{APIVersion: atmosmanifest.DefaultAPIVersion, Kind: Kind, Metadata: atmosmanifest.Metadata{Name: "plugins"}, Spec: Spec{LockVersion: 1, AtmosVersion: version.Version, GeneratedAt: time.Now().UTC(), Packages: make(map[string]Entry)}}
}

// Load reads an optional lock and validates all entries without network access.
func Load(project string) (*File, error) {
	defer perf.Track(nil, "pluginlock.Load")()

	raw, err := manifest.ReadFile(project, filepath.Base(Path(project)))
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	file, err := atmosmanifest.Load[Spec](Kind, raw)
	if err != nil {
		return nil, err
	}
	if file.Spec.LockVersion != 1 {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	for itemIndex := range file.Spec.Packages {
		entry := file.Spec.Packages[itemIndex]
		if err := validateEntry(&entry); err != nil {
			return nil, err
		}
	}
	return file, nil
}

// Save atomically writes a prepared lock. The caller owns the transaction lock.
func Save(project string, file *File) error {
	defer perf.Track(nil, "pluginlock.Save")()

	file.Spec.GeneratedAt = time.Now().UTC()
	raw, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(Path(project), raw, lockFileMode)
}

func validateEntry(entry *Entry) error {
	if err := validateEntryMetadata(entry); err != nil {
		return err
	}
	if err := validateDigest(entry.IndexDigest); err != nil {
		return errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	for _, digest := range entry.Platforms {
		if err := validateDigest(digest); err != nil {
			return errors.Join(errUtils.ErrPluginInvalidConfig, err)
		}
	}
	if err := validateExports(entry.Exports); err != nil {
		return err
	}
	constraint, err := semver.NewConstraint(entry.Constraint)
	if err != nil {
		return errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	selected, err := semver.StrictNewVersion(entry.Version)
	if err != nil || !constraint.Check(selected) {
		return errors.Join(errUtils.ErrPluginFrozenDrift, err)
	}
	return nil
}

// CheckDeclarations enforces exact dependency/lock agreement in frozen mode.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func CheckDeclarations(file *File, declarations map[string]schema.PluginPackage) error {
	if len(file.Spec.Packages) != len(declarations) {
		return errUtils.ErrPluginFrozenDrift
	}
	for alias, declaration := range declarations {
		entry, found := file.Spec.Packages[alias]
		if !found || entry.Source != declaration.Source || entry.Constraint != declaration.Version {
			return errUtils.ErrPluginFrozenDrift
		}
	}
	return nil
}

// CheckRelease prevents tag movement or layout substitution from changing a lock.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func CheckRelease(entry *Entry, release *pluginoci.Release) error {
	if entry.Plugin != release.Name || entry.Version != release.Version || entry.IndexDigest != release.Digest || len(entry.Platforms) != len(release.Platforms) {
		return errUtils.ErrPluginFrozenDrift
	}
	for platform := range release.Platforms {
		descriptor := release.Platforms[platform]
		if entry.Platforms[platform] != descriptor.Digest.String() {
			return errUtils.ErrPluginFrozenDrift
		}
	}
	return nil
}

func validateDigest(value string) error {
	if !strings.HasPrefix(value, "sha256:") || len(value) != sha256ReferenceLength {
		return errUtils.ErrPluginInvalidConfig
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	if err != nil || len(raw) != sha256Bytes || value != strings.ToLower(value) {
		return errUtils.ErrPluginInvalidConfig
	}
	return nil
}

func validateExports(entries []Export) error {
	exports := make(map[string]bool)
	for _, export := range entries {
		key := export.Capability + ":" + export.Name
		if export.Capability == "" || export.Name == "" || exports[key] {
			return errUtils.ErrPluginInvalidConfig
		}
		exports[key] = true
	}
	return nil
}

func validateEntryMetadata(entry *Entry) error {
	if entry.Plugin == "" || !strings.HasPrefix(entry.Source, "oci://") || len(entry.Platforms) == 0 || entry.Runtime.Type != "native" || entry.Runtime.Protocol != 1 || !manifest.LocalPath(entry.Runtime.Executable) || len(entry.Exports) == 0 || !validVerification(entry.Verification) {
		return errUtils.ErrPluginInvalidConfig
	}
	return nil
}

func validVerification(value Verification) bool {
	switch value.Signature {
	case "unverified":
		return value.Identity == "" && value.Issuer == ""
	case "verified":
		return value.Identity != "" && value.Issuer != ""
	default:
		return false
	}
}
