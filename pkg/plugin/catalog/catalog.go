// Package catalog manages discovery metadata independently of release installation.
package catalog

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginmanifest "github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

const Kind = "AtmosPluginCatalog"

// File is the standard Atmos manifest envelope for mutable discovery metadata.
type File = manifest.Manifest[Spec]

// Spec holds discoverable packages. Catalog inclusion does not establish trust.
type Spec struct {
	Entries []Entry `yaml:"entries" json:"entries"`
}

// Entry describes a package repository and the releases indexed for discovery.
type Entry struct {
	Name         string    `yaml:"name" json:"name"`
	Description  string    `yaml:"description,omitempty" json:"description,omitempty"`
	Publisher    string    `yaml:"publisher" json:"publisher"`
	Capabilities []string  `yaml:"capabilities" json:"capabilities"`
	Runtime      string    `yaml:"runtime" json:"runtime"`
	Source       string    `yaml:"source" json:"source"`
	Versions     []Version `yaml:"versions" json:"versions"`
}

// Version pins a discovery result to an immutable OCI index.
type Version struct {
	Version       string                       `yaml:"version" json:"version"`
	Digest        string                       `yaml:"digest" json:"digest"`
	Platforms     []string                     `yaml:"platforms" json:"platforms"`
	Compatibility pluginmanifest.Compatibility `yaml:"compatibility" json:"compatibility"`
	Yanked        *Yanked                      `yaml:"yanked,omitempty" json:"yanked,omitempty"`
}

// Yanked excludes a version from new discovery without affecting existing locks.
type Yanked struct {
	Reason string `yaml:"reason" json:"reason"`
}

func init() { manifest.MustRegister(Kind, manifest.DefaultAPIVersion, &Spec{}) }

// New creates an empty catalog suitable for an initial publication.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func New(name string) *File {
	return &File{APIVersion: manifest.DefaultAPIVersion, Kind: Kind, Metadata: manifest.Metadata{Name: name}, Spec: Spec{Entries: []Entry{}}}
}

// Parse validates catalog metadata without fetching or executing any package.
func Parse(raw []byte) (*File, error) {
	defer perf.Track(nil, "plugincatalog.Parse")()

	file, err := manifest.Load[Spec](Kind, raw)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for itemIndex := range file.Spec.Entries {
		entry := &file.Spec.Entries[itemIndex]
		if seen[entry.Name] || entry.Name == "" {
			return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanationf("Catalog entry %q is empty or listed twice.", entry.Name).
				WithHint("Give every catalog entry a unique, non-empty `name`.").Err()
		}
		seen[entry.Name] = true
		entry.Source, err = normalizeSource(entry.Source)
		if err != nil {
			return nil, err
		}
		versions := make(map[string]bool)
		for _, version := range entry.Versions {
			if versions[version.Version] {
				return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
					WithExplanationf("Catalog entry %q lists version %s twice.", entry.Name, version.Version).
					WithHint("Keep one record per version.").Err()
			}
			versions[version.Version] = true
			if _, err := semver.StrictNewVersion(version.Version); err != nil {
				return nil, err
			}
			if _, err := v1.NewHash(version.Digest); err != nil {
				return nil, err
			}
		}
	}
	return file, nil
}

// Add records a verified release's metadata without executing the plugin.
func Add(file *File, source string, release *pluginoci.Release, descriptors *extension.Registry) error {
	defer perf.Track(nil, "plugincatalog.Add")()

	source, err := normalizeSource(source)
	if err != nil {
		return err
	}
	bindings, err := descriptors.Discover(release.Definition.Spec.Capabilities)
	if err != nil {
		return err
	}
	capabilities := []string{}
	for _, binding := range bindings {
		capabilities = append(capabilities, binding.Capability)
	}
	sort.Strings(capabilities)
	platforms := slices.Sorted(maps.Keys(release.Platforms))
	entry := Entry{Name: release.Name, Description: release.Definition.Metadata.Description, Publisher: strings.SplitN(release.Name, "/", 2)[0], Capabilities: slices.Compact(capabilities), Runtime: release.Definition.Spec.Runtime.Type, Source: source}
	version := Version{Version: NormalizeVersion(release.Version), Digest: release.Digest, Platforms: platforms, Compatibility: release.Definition.Spec.Compatibility}
	for index := range file.Spec.Entries {
		previous := &file.Spec.Entries[index]
		if previous.Name != release.Name {
			continue
		}
		if previous.Source != source {
			return errUtils.Build(errUtils.ErrPluginCatalogConflict).
				WithExplanationf("The catalog already lists %s from %s, not %s.", release.Name, previous.Source, source).
				WithHint("Publish the release to the repository the catalog already lists, or unlist it first with `atmos plugin catalog remove`.").Err()
		}
		for _, existing := range previous.Versions {
			if existing.Version == version.Version {
				if existing.Digest != version.Digest {
					return errUtils.Build(errUtils.ErrPluginImmutableRelease).
						WithExplanationf("%s %s is already listed with a different digest.", release.Name, version.Version).
						WithHint("Releases are immutable; publish the changes under a new version.").Err()
				}
				return nil
			}
		}
		entry.Versions = append(slices.Clone(previous.Versions), version)
		*previous = entry
		return nil
	}
	entry.Versions = []Version{version}
	file.Spec.Entries = append(file.Spec.Entries, entry)
	sort.Slice(file.Spec.Entries, func(i, j int) bool { return file.Spec.Entries[i].Name < file.Spec.Entries[j].Name })
	return nil
}

// normalizeSource keeps catalog references usable as consumer package declarations.
// Publication accepts both ordinary OCI repository names and oci:// references.
func normalizeSource(source string) (string, error) {
	repository, err := name.NewRepository(strings.TrimPrefix(source, "oci://"), name.StrictValidation)
	if err != nil {
		return "", errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	return "oci://" + repository.Name(), nil
}

// NormalizeVersion strips a single leading "v" so "v0.1.0" and "0.1.0" name the same release.
// Catalog versions are stored without the prefix because they are strict semantic versions.
func NormalizeVersion(version string) string {
	defer perf.Track(nil, "plugincatalog.NormalizeVersion")()

	if len(version) > 1 && (version[0] == 'v' || version[0] == 'V') && version[1] >= '0' && version[1] <= '9' {
		return version[1:]
	}
	return version
}

// ParseReleaseReference validates a release reference locally. A tag or digest is required,
// because registries otherwise resolve an implicit "latest" that catalogs must not pin.
func ParseReleaseReference(value string) (name.Reference, error) {
	defer perf.Track(nil, "plugincatalog.ParseReleaseReference")()

	reference, err := name.ParseReference(strings.TrimPrefix(value, "oci://"), name.StrictValidation)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanationf("%q is not a pinned OCI release reference.", value).
			WithHint("Pass `oci://<registry>/<repository>:<tag>` or `oci://<registry>/<repository>@sha256:<digest>`; a tag or digest is required.").Err()
	}
	return reference, nil
}

// Remove unlists one version or an entire entry. Artifact bytes remain available.
// A leading "v" on version is ignored.
func Remove(file *File, name, version string) error {
	defer perf.Track(nil, "catalog.Remove")()

	version = NormalizeVersion(version)
	for index := range file.Spec.Entries {
		entry := &file.Spec.Entries[index]
		if entry.Name != name {
			continue
		}
		if version == "" {
			file.Spec.Entries = slices.Delete(file.Spec.Entries, index, index+1)
			return nil
		}
		before := len(entry.Versions)
		entry.Versions = slices.DeleteFunc(slices.Clone(entry.Versions), func(value Version) bool { return value.Version == version })
		if before == len(entry.Versions) {
			available := make([]string, 0, len(entry.Versions))
			for _, listed := range entry.Versions {
				available = append(available, listed.Version)
			}
			return errUtils.Build(fmt.Errorf("%w: %s %s (available: %s)", errUtils.ErrPluginCatalogVersionNotFound, name, version, strings.Join(available, ", "))).
				WithHint("Run `atmos plugin search` to see the versions this catalog lists.").Err()
		}
		file.Spec.Entries[index].Versions = entry.Versions
		return nil
	}
	listed := make([]string, 0, len(file.Spec.Entries))
	for index := range file.Spec.Entries {
		listed = append(listed, file.Spec.Entries[index].Name)
	}
	builder := errUtils.Build(fmt.Errorf("%w: %s", errUtils.ErrPluginCatalogEntryNotFound, name))
	if len(listed) == 0 {
		return builder.WithHint("The catalog lists no packages yet; add one with `atmos plugin catalog add`.").Err()
	}
	return builder.WithHintf("Listed packages: %s.", strings.Join(listed, ", ")).Err()
}
