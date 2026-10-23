// Package oci defines the plugin release format on the shared OCI transport.
package oci

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
)

const (
	// ConfigMediaType identifies a plugin platform image's static configuration.
	ConfigMediaType types.MediaType = "application/vnd.atmos.plugin.config.v1+json"
	// BundleMediaType identifies a bounded, uncompressed package archive.
	BundleMediaType    types.MediaType = "application/vnd.atmos.plugin.bundle.v1+tar"
	annotationName                     = "dev.atmos.plugin.name"
	annotationVersion                  = "dev.atmos.plugin.version"
	annotationManifest                 = "dev.atmos.plugin.manifest.digest"
	manifestLabel                      = "dev.atmos.plugin.manifest"
	maxBundleSize                      = 256 << 20
)

// Option adjusts how packing and publishing verify a release.
type Option func(*options)

type options struct {
	checkExecutableFormat bool
}

// WithExecutableFormatCheck requires every platform executable to be a native
// ELF, Mach-O, or PE binary for its declared operating system and architecture.
// The path guard and the exact payload file set are always enforced.
func WithExecutableFormatCheck() Option {
	defer perf.Track(nil, "pluginoci.WithExecutableFormatCheck")()

	return func(o *options) { o.checkExecutableFormat = true }
}

func newOptions(opts []Option) *options {
	configured := &options{}
	for _, opt := range opts {
		opt(configured)
	}
	return configured
}

// Pack assembles prebuilt platform binaries into an OCI layout. It never builds
// source or starts a plugin, and refuses to overwrite an existing output.
func Pack(directory, output string, descriptors *extension.Registry, opts ...Option) error {
	defer perf.Track(nil, "pluginoci.Pack")()

	index, err := BuildIndex(directory, descriptors, opts...)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	if err := os.MkdirAll(filepath.Dir(output), executableFileMode); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(output), ".plugin-pack-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err := atmosoci.SaveIndex(temporary, index); err != nil {
		return err
	}
	return os.Rename(temporary, output)
}

// BuildIndex assembles a deterministic release with one payload per declared
// platform. Every payload uses exactly the same manifest and schema bytes.
func BuildIndex(directory string, descriptors *extension.Registry, opts ...Option) (v1.ImageIndex, error) {
	defer perf.Track(nil, "pluginoci.BuildIndex")()

	configured := newOptions(opts)
	definition, err := manifest.Load(directory)
	if err != nil {
		return nil, err
	}
	if err := checkDeclaredPaths(declaredExecutables(definition)...); err != nil {
		return nil, err
	}
	if _, err := descriptors.Validate(directory, definition.Spec.Capabilities); err != nil {
		return nil, err
	}
	if len(definition.Spec.Platforms) == 0 {
		return nil, fmt.Errorf("%w: declare spec.platforms before packing", errUtils.ErrPluginInvalidConfig)
	}
	files, err := packageFiles(directory)
	if err != nil {
		return nil, err
	}
	annotations := releaseAnnotations(definition, files["plugin.yaml"])
	platforms := make([]string, 0, len(definition.Spec.Platforms))
	for platform := range definition.Spec.Platforms {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	additions := make([]mutate.IndexAddendum, 0, len(platforms))
	for _, platform := range platforms {
		parsed, err := parsePlatform(platform)
		if err != nil {
			return nil, err
		}
		img, err := platformImage(directory, definition, parsed, files, configured)
		if err != nil {
			return nil, err
		}
		additions = append(additions, mutate.IndexAddendum{Add: img, Descriptor: v1.Descriptor{Platform: parsed, Annotations: annotations}})
	}
	index := mutate.AppendManifests(empty.Index, additions...)
	return mutate.Annotations(index, annotations).(v1.ImageIndex), nil
}

// declaredExecutables lists the runtime and every per-platform executable path.
func declaredExecutables(definition *manifest.Config) []string {
	paths := []string{definition.Spec.Runtime.Executable}
	for _, platform := range slices.Sorted(maps.Keys(definition.Spec.Platforms)) {
		paths = append(paths, definition.Spec.Platforms[platform].Executable)
	}
	return paths
}

func parsePlatform(value string) (*v1.Platform, error) {
	target, err := pluginplatform.Parse(value)
	if err != nil {
		return nil, err
	}
	return &v1.Platform{OS: target.OS, Architecture: target.Architecture, Variant: target.Variant}, nil
}

func releaseAnnotations(definition *manifest.Config, raw []byte) map[string]string {
	digest := sha256.Sum256(raw)
	return map[string]string{annotationName: definition.Metadata.Name, annotationVersion: definition.Metadata.Version, annotationManifest: "sha256:" + hex.EncodeToString(digest[:])}
}

func platformImage(directory string, definition *manifest.Config, parsed *v1.Platform, files map[string][]byte, configured *options) (v1.Image, error) {
	selection := definition.Spec.Platforms[parsed.String()]
	executable, err := readPackageFile(directory, selection.Executable, maxBundleSize)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(executable)
	if selection.SHA256 != "" && strings.TrimPrefix(selection.SHA256, "sha256:") != hex.EncodeToString(digest[:]) {
		return nil, errUtils.ErrOCIIntegrity
	}
	if configured.checkExecutableFormat {
		if err := checkRuntimeExecutable(definition.Spec.Runtime.Executable, parsed); err != nil {
			return nil, err
		}
		if err := checkExecutableFormat(bytes.NewReader(executable), parsed); err != nil {
			return nil, err
		}
	}
	archive, err := bundle(files, definition.Spec.Runtime.Executable, executable)
	if err != nil {
		return nil, err
	}
	img, err := mutate.AppendLayers(empty.Image, static.NewLayer(archive, BundleMediaType))
	if err != nil {
		return nil, err
	}
	config, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}
	config.OS, config.Architecture, config.Variant = parsed.OS, parsed.Architecture, parsed.Variant
	config.Config.Labels = map[string]string{manifestLabel: string(files["plugin.yaml"])}
	img, err = mutate.ConfigFile(img, config)
	if err != nil {
		return nil, err
	}
	img = mutate.ConfigMediaType(mutate.MediaType(img, types.OCIManifestSchema1), ConfigMediaType)
	return mutate.Annotations(img, releaseAnnotations(definition, files["plugin.yaml"])).(v1.Image), nil
}
