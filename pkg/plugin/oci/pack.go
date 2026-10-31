// Package oci defines the plugin release format on the shared OCI transport.
package oci

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

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
	BundleMediaType        types.MediaType = "application/vnd.atmos.plugin.bundle.v1+tar"
	annotationName                         = "dev.atmos.plugin.name"
	annotationVersion                      = "dev.atmos.plugin.version"
	annotationManifest                     = "dev.atmos.plugin.manifest.digest"
	annotationSource                       = "org.opencontainers.image.source"
	annotationTitle                        = "org.opencontainers.image.title"
	annotationDescription                  = "org.opencontainers.image.description"
	annotationImageVersion                 = "org.opencontainers.image.version"
	maxSourceLength                        = 512
	manifestLabel                          = "dev.atmos.plugin.manifest"
	maxBundleSize                          = 256 << 20
)

// Option adjusts how packing and publishing verify a release.
type Option func(*options)

type options struct {
	checkExecutableFormat bool
	source                string
}

// WithSource sets the org.opencontainers.image.source annotation, the metadata
// registries such as GHCR use to link a package to its repository. It overrides
// the repository declared in the plugin manifest. The value must be a plain
// https URL: credentials, query strings, and fragments are rejected so a
// release never publishes anything private.
func WithSource(source string) Option {
	defer perf.Track(nil, "pluginoci.WithSource")()

	return func(o *options) { o.source = source }
}

// GitHubActionsSource derives the repository URL from the GitHub Actions
// environment (GITHUB_SERVER_URL, defaulting to https://github.com, and
// GITHUB_REPOSITORY). It returns an empty string outside GitHub Actions or for
// a value that is not a safe https URL. Callers opt in explicitly; packing
// itself never reads the environment.
func GitHubActionsSource(getenv func(string) string) string {
	defer perf.Track(nil, "pluginoci.GitHubActionsSource")()

	slug := getenv("GITHUB_REPOSITORY")
	if strings.Count(slug, "/") != 1 || strings.HasPrefix(slug, "/") || strings.HasSuffix(slug, "/") {
		return ""
	}
	server := strings.TrimRight(getenv("GITHUB_SERVER_URL"), "/")
	if server == "" {
		server = "https://github.com"
	}
	source, ok := publishableSource(server + "/" + slug)
	if !ok {
		return ""
	}
	return source
}

// publishableSource reports whether a repository reference is safe to publish as the
// image source: an https URL with a host and without credentials, query, or fragment.
func publishableSource(raw string) (string, bool) {
	if raw == "" || len(raw) > maxSourceLength || strings.ContainsFunc(raw, unicode.IsSpace) {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || !plainHTTPSURL(parsed) {
		return "", false
	}
	return raw, true
}

// plainHTTPSURL reports whether a parsed URL is https with a host and carries nothing but a location.
func plainHTTPSURL(parsed *url.URL) bool {
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == ""
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
	if configured.source != "" {
		source, ok := publishableSource(configured.source)
		if !ok {
			return nil, fmt.Errorf("%w: the package source must be an https URL without credentials, query, or fragment", errUtils.ErrPluginInvalidConfig)
		}
		annotations[annotationSource] = source
	}
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
		img, err := platformImage(&releaseBuild{directory: directory, definition: definition, files: files, annotations: annotations, options: configured}, parsed)
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

// releaseAnnotations derives every annotation from the verified manifest bytes, so a
// release can be re-checked from the manifest alone. The OCI image annotations let
// registries show the package title and link it to its repository.
func releaseAnnotations(definition *manifest.Config, raw []byte) map[string]string {
	digest := sha256.Sum256(raw)
	annotations := map[string]string{
		annotationName: definition.Metadata.Name, annotationVersion: definition.Metadata.Version, annotationManifest: "sha256:" + hex.EncodeToString(digest[:]),
		annotationTitle: definition.Metadata.Name, annotationImageVersion: definition.Metadata.Version,
	}
	if definition.Metadata.DisplayName != "" {
		annotations[annotationTitle] = definition.Metadata.DisplayName
	}
	if definition.Metadata.Description != "" {
		annotations[annotationDescription] = definition.Metadata.Description
	}
	if source, ok := publishableSource(definition.Metadata.Repository); ok {
		annotations[annotationSource] = source
	}
	return annotations
}

// releaseBuild carries the inputs shared by every platform image of one release.
type releaseBuild struct {
	directory   string
	definition  *manifest.Config
	files       map[string][]byte
	annotations map[string]string
	options     *options
}

func platformImage(build *releaseBuild, parsed *v1.Platform) (v1.Image, error) {
	directory, definition, files, configured := build.directory, build.definition, build.files, build.options
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
		if err := checkPackedExecutable(definition.Spec.Runtime.Executable, executable, parsed); err != nil {
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
	return mutate.Annotations(img, build.annotations).(v1.Image), nil
}

func checkPackedExecutable(name string, executable []byte, platform *v1.Platform) error {
	if err := checkRuntimeExecutable(name, platform); err != nil {
		return err
	}
	return checkExecutableFormat(bytes.NewReader(executable), platform)
}
