package oci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

const maxPlatforms = 64

// Release records immutable metadata for all platforms without executing code.
type Release struct {
	Name       string                   `json:"name" yaml:"name"`
	Version    string                   `json:"version" yaml:"version"`
	Digest     string                   `json:"digest" yaml:"digest"`
	Platforms  map[string]v1.Descriptor `json:"platforms" yaml:"platforms"`
	Definition *manifest.Config         `json:"manifest" yaml:"manifest"`
	Index      v1.ImageIndex            `json:"-" yaml:"-"`
	Manifest   []byte                   `json:"-" yaml:"-"`

	// files caches the payload inventory once it has been read from the layers.
	files []BundleFile
}

// Inspect verifies the release index and every platform's static metadata.
// Only the selected platform's executable layer needs to be downloaded later.
func Inspect(index v1.ImageIndex, expected string) (*Release, error) {
	defer perf.Track(nil, "pluginoci.Inspect")()

	root, digest, err := inspectIndex(index, expected)
	if err != nil {
		return nil, err
	}
	release := &Release{Digest: digest.String(), Index: index, Platforms: make(map[string]v1.Descriptor)}
	for itemIndex := range root.Manifests {
		descriptor := root.Manifests[itemIndex]
		if err := inspectPlatform(release, &descriptor, root.Annotations); err != nil {
			return nil, err
		}
	}
	if err := inspectDefinition(release, root.Annotations); err != nil {
		return nil, err
	}
	return release, nil
}

func inspectPlatform(release *Release, descriptor *v1.Descriptor, annotations map[string]string) error {
	if err := validatePlatformDescriptor(descriptor); err != nil {
		return err
	}
	platform := descriptor.Platform.String()
	if _, exists := release.Platforms[platform]; exists {
		return errUtils.ErrPluginInvalidConfig
	}
	if !maps.Equal(descriptor.Annotations, annotations) {
		return errUtils.ErrOCIIntegrity
	}
	img, err := release.Index.Image(descriptor.Digest)
	if err != nil {
		return err
	}
	imageManifest, err := verifiedManifest(img, descriptor)
	if err != nil {
		return err
	}
	if !maps.Equal(imageManifest.Annotations, annotations) {
		return errUtils.ErrOCIIntegrity
	}
	config, err := inspectPlatformConfig(img, imageManifest, descriptor.Platform)
	if err != nil {
		return err
	}
	if err := matchPlatformDefinition(release, config); err != nil {
		return err
	}
	release.Platforms[platform] = *descriptor
	return nil
}

func verifiedManifest(img v1.Image, descriptor *v1.Descriptor) (*v1.Manifest, error) {
	raw, err := img.RawManifest()
	if err != nil {
		return nil, err
	}
	if _, err := atmosoci.ReadVerifiedBlob(bytes.NewReader(raw), descriptor, maxMetadataSize); err != nil {
		return nil, err
	}
	var manifest v1.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	if manifest.Config.MediaType != ConfigMediaType || len(manifest.Layers) != 1 || manifest.Layers[0].MediaType != BundleMediaType {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	return &manifest, nil
}

// Extract verifies and materializes one platform into a fresh staging directory.
// Callers atomically promote the directory only after extension validation.
func (r *Release) Extract(platform, directory string) error {
	defer perf.Track(nil, "pluginoci.Release.Extract")()

	descriptor, found := r.Platforms[platform]
	if !found {
		return fmt.Errorf("%w: platform %s is not available", errUtils.ErrPluginInvalidConfig, platform)
	}
	img, err := r.Index.Image(descriptor.Digest)
	if err != nil {
		return err
	}
	definition, err := verifiedManifest(img, &descriptor)
	if err != nil {
		return err
	}
	layer, err := img.LayerByDigest(definition.Layers[0].Digest)
	if err != nil {
		return err
	}
	reader, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer reader.Close()
	raw, err := atmosoci.ReadVerifiedBlob(reader, &definition.Layers[0], maxBundleSize)
	if err != nil {
		return err
	}
	return r.extractPayload(raw, directory)
}

func (r *Release) extractPayload(raw []byte, directory string) error {
	// Third-party releases get the same path rules as local packs before any byte is written.
	if r.Definition == nil {
		return errUtils.ErrPluginInvalidConfig
	}
	rules := &bundleRules{executable: r.Definition.Spec.Runtime.Executable, exactSet: true}
	if _, err := scanBundle(bytes.NewReader(raw), rules); err != nil {
		return err
	}
	if err := atmosoci.ExtractTar(raw, directory); err != nil {
		return err
	}
	actual, err := manifest.ReadFile(directory, "plugin.yaml")
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, r.Manifest) {
		return errUtils.ErrOCIIntegrity
	}
	return nil
}

func inspectDefinition(release *Release, annotations map[string]string) error {
	definition, err := manifest.Parse(release.Manifest)
	if err != nil {
		return err
	}
	if !maps.Equal(annotations, releaseAnnotations(definition, release.Manifest)) {
		return errUtils.ErrOCIIntegrity
	}
	if len(definition.Spec.Platforms) != len(release.Platforms) {
		return errUtils.ErrOCIIntegrity
	}
	for platform := range release.Platforms {
		if _, found := definition.Spec.Platforms[platform]; !found {
			return errUtils.ErrOCIIntegrity
		}
	}
	release.Name, release.Version, release.Definition = definition.Metadata.Name, definition.Metadata.Version, definition
	return nil
}

func inspectPlatformConfig(img v1.Image, imageManifest *v1.Manifest, platform *v1.Platform) (*v1.ConfigFile, error) {
	raw, err := img.RawConfigFile()
	if err != nil {
		return nil, err
	}
	if _, err := atmosoci.ReadVerifiedBlob(bytes.NewReader(raw), &imageManifest.Config, maxMetadataSize); err != nil {
		return nil, err
	}
	var config v1.ConfigFile
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	if config.OS != platform.OS || config.Architecture != platform.Architecture || config.Variant != platform.Variant {
		return nil, errUtils.ErrOCIIntegrity
	}
	return &config, nil
}

func inspectIndex(index v1.ImageIndex, expected string) (*v1.IndexManifest, v1.Hash, error) {
	raw, err := index.RawManifest()
	if err != nil {
		return nil, v1.Hash{}, err
	}
	if len(raw) > maxMetadataSize {
		return nil, v1.Hash{}, errUtils.ErrPluginPayloadTooLarge
	}
	digest, _, err := v1.SHA256(bytes.NewReader(raw))
	if err != nil {
		return nil, v1.Hash{}, err
	}
	if expected != "" && digest.String() != expected {
		return nil, v1.Hash{}, errUtils.ErrOCIIntegrity
	}
	var root v1.IndexManifest
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, v1.Hash{}, err
	}
	if root.MediaType != types.OCIImageIndex || len(root.Manifests) == 0 || len(root.Manifests) > maxPlatforms {
		return nil, v1.Hash{}, errUtils.ErrPluginInvalidConfig
	}

	return &root, digest, nil
}

func matchPlatformDefinition(release *Release, config *v1.ConfigFile) error {
	definition := []byte(config.Config.Labels[manifestLabel])
	if len(definition) == 0 {
		return errUtils.ErrPluginInvalidConfig
	}
	if release.Manifest == nil {
		release.Manifest = definition
	}
	if !bytes.Equal(release.Manifest, definition) {
		return errUtils.ErrOCIIntegrity
	}

	return nil
}

func validatePlatformDescriptor(descriptor *v1.Descriptor) error {
	if descriptor.Platform == nil || descriptor.MediaType != types.OCIManifestSchema1 {
		return errUtils.ErrPluginInvalidConfig
	}
	platform := descriptor.Platform.String()
	if _, err := parsePlatform(platform); err != nil {
		return err
	}
	return nil
}
