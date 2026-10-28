package oci

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_registry_test.go -package=oci github.com/cloudposse/atmos/pkg/oci Registry
//go:generate go run go.uber.org/mock/mockgen -destination=mock_image_test.go -package=oci github.com/google/go-containerregistry/pkg/v1 Image,ImageIndex,Layer

const validationManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: acme/fixture, version: 1.0.0}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  platforms:
    linux/amd64: {executable: bin/plugin}
  capabilities:
    fixtures:
      acme/fixture: {}
`

func validationFixture(t *testing.T) (string, *extension.Registry) {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("compiled plugin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(validationManifest), 0o644))
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "fixture", Path: []string{"fixtures"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	return directory, descriptors
}

func TestPublishImmutableVersionAndVerification(t *testing.T) {
	directory, descriptors := validationFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, Pack(directory, layout, descriptors))
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	destination := strings.TrimPrefix(server.URL, "http://") + "/acme/plugin"
	client := atmosoci.NewClient(nil)
	first, err := Publish(t.Context(), client, layout, destination, descriptors)
	require.NoError(t, err)
	second, err := Publish(t.Context(), client, layout, destination, descriptors)
	require.NoError(t, err)
	require.Equal(t, first.Digest, second.Digest)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("different executable"), 0o755))
	changed := filepath.Join(t.TempDir(), "changed")
	require.NoError(t, Pack(directory, changed, descriptors))
	_, err = Publish(t.Context(), client, changed, destination, descriptors)
	require.ErrorIs(t, err, errUtils.ErrPluginImmutableRelease)
	_, err = Publish(t.Context(), client, layout, "bad repository", descriptors)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = Publish(t.Context(), client, t.TempDir(), destination, descriptors)
	require.Error(t, err)
	_, err = Publish(t.Context(), client, layout, destination, extension.New())
	require.Error(t, err)
}

func TestPublishPropagatesTransportErrorsAndDetectsConcurrentReplacement(t *testing.T) {
	directory, descriptors := validationFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, Pack(directory, layout, descriptors))
	for _, phase := range []string{"head", "write", "verify", "replacement"} {
		t.Run(phase, func(t *testing.T) {
			client := NewMockRegistry(gomock.NewController(t))
			configurePublishFailure(client, phase)
			_, err := Publish(t.Context(), client, layout, "ghcr.io/acme/plugin", descriptors)
			if phase == "replacement" {
				require.ErrorIs(t, err, errUtils.ErrPluginImmutableRelease)
			} else {
				require.ErrorIs(t, err, os.ErrPermission)
			}
		})
	}
}

func TestPackageRejectsUnsafeAndOversizedInputs(t *testing.T) {
	directory, descriptors := validationFixture(t)
	for _, value := range []string{"linux", "linux/amd64/extra/part", "linux//v1", "linux /amd64", "linux/amd64\\variant"} {
		_, err := parsePlatform(value)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
	platform, err := parsePlatform("linux/arm/v7")
	require.NoError(t, err)
	require.Equal(t, "v7", platform.Variant)
	_, err = readPackageFile(directory, "../escape", 10)
	require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden)
	_, err = readPackageFile(directory, "bin/plugin", 2)
	require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
	_, err = readPackageFile(directory, "bin", 100)
	require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden, "directories are not regular files")
	_, err = readPackageFile(filepath.Join(directory, "missing"), "plugin", 100)
	require.Error(t, err)
	_, err = bundle(map[string][]byte{}, "../plugin", nil)
	require.ErrorIs(t, err, errUtils.ErrPluginBundlePathForbidden)
	_, err = bundle(map[string][]byte{"plugin.yaml": {}}, "plugin.yaml", nil)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.False(t, equalFiles(map[string][]byte{"a": []byte("one")}, map[string][]byte{}))
	require.False(t, equalFiles(map[string][]byte{"a": []byte("one")}, map[string][]byte{"a": []byte("two")}))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "schemas"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", "invalid.txt"), []byte("data"), 0o644))
	_, err = BuildIndex(directory, descriptors)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NoError(t, os.Remove(filepath.Join(directory, "schemas", "invalid.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(strings.ReplaceAll(validationManifest, "executable: bin/plugin}", "executable: bin/plugin, sha256: bad}")), 0o644))
	_, err = BuildIndex(directory, descriptors)
	require.Error(t, err)
}

func TestInspectRejectsInvalidIndexAndPlatformMetadata(t *testing.T) {
	for _, raw := range [][]byte{[]byte("invalid"), []byte(`{"mediaType":"wrong"}`), bytes.Repeat([]byte(" "), maxMetadataSize+1)} {
		index := NewMockImageIndex(gomock.NewController(t))
		index.EXPECT().RawManifest().Return(raw, nil)
		_, err := Inspect(index, "")
		require.Error(t, err)
	}
	index := NewMockImageIndex(gomock.NewController(t))
	index.EXPECT().RawManifest().Return(nil, os.ErrPermission)
	_, err := Inspect(index, "")
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorIs(t, validatePlatformDescriptor(&v1.Descriptor{}), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, validatePlatformDescriptor(&v1.Descriptor{MediaType: types.OCIManifestSchema1, Platform: &v1.Platform{OS: "bad os", Architecture: "amd64"}}), errUtils.ErrPluginInvalidConfig)
	release := &Release{Platforms: map[string]v1.Descriptor{"linux/amd64": {}}}
	descriptor := &v1.Descriptor{MediaType: types.OCIManifestSchema1, Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}}
	require.ErrorIs(t, inspectPlatform(release, descriptor, nil), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, matchPlatformDefinition(&Release{}, &v1.ConfigFile{}), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, matchPlatformDefinition(&Release{Manifest: []byte("one")}, &v1.ConfigFile{Config: v1.Config{Labels: map[string]string{manifestLabel: "two"}}}), errUtils.ErrOCIIntegrity)
	definition, err := manifest.Parse([]byte(validationManifest))
	require.NoError(t, err)
	annotations := releaseAnnotations(definition, []byte(validationManifest))
	require.Error(t, inspectDefinition(&Release{Manifest: []byte("invalid")}, annotations))
	require.ErrorIs(t, inspectDefinition(&Release{Manifest: []byte(validationManifest)}, map[string]string{}), errUtils.ErrOCIIntegrity)
	require.ErrorIs(t, inspectDefinition(&Release{Manifest: []byte(validationManifest)}, annotations), errUtils.ErrOCIIntegrity)
	require.ErrorIs(t, inspectDefinition(&Release{Manifest: []byte(validationManifest), Platforms: map[string]v1.Descriptor{"darwin/arm64": {}}}, annotations), errUtils.ErrOCIIntegrity)
}

func TestVerifyInstalledRejectsChangedMissingAndNonRegularFiles(t *testing.T) {
	directory, descriptors := validationFixture(t)
	index, err := BuildIndex(directory, descriptors)
	require.NoError(t, err)
	release, err := Inspect(index, "")
	require.NoError(t, err)
	destination := t.TempDir()
	require.NoError(t, release.Extract("linux/amd64", destination))
	require.NoError(t, release.VerifyInstalled("linux/amd64", destination))
	require.ErrorIs(t, release.VerifyInstalled("missing", destination), errUtils.ErrPluginNotInstalled)
	require.NoError(t, os.WriteFile(filepath.Join(destination, "bin", "plugin"), []byte("changed! plugin"), 0o755))
	require.ErrorIs(t, release.VerifyInstalled("linux/amd64", destination), errUtils.ErrOCIIntegrity)
	require.NoError(t, os.Remove(filepath.Join(destination, "plugin.yaml")))
	require.Error(t, release.VerifyInstalled("linux/amd64", destination))
	require.Error(t, verifyArchiveFiles(destination, []byte("invalid archive")))
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "directory", Typeflag: tar.TypeDir, Mode: 0o755}))
	require.NoError(t, writer.Close())
	require.ErrorIs(t, verifyArchiveFiles(destination, buffer.Bytes()), errUtils.ErrOCIIntegrity)
}

func TestVerifiedManifestRejectsCorruptionAndWrongMediaType(t *testing.T) {
	for _, raw := range [][]byte{[]byte("invalid"), []byte(`{"config":{"mediaType":"wrong"}}`)} {
		image := NewMockImage(gomock.NewController(t))
		image.EXPECT().RawManifest().Return(raw, nil)
		digest, size, err := v1.SHA256(bytes.NewReader(raw))
		require.NoError(t, err)
		_, err = verifiedManifest(image, &v1.Descriptor{Digest: digest, Size: size})
		require.Error(t, err)
	}
	image := NewMockImage(gomock.NewController(t))
	image.EXPECT().RawManifest().Return(nil, os.ErrPermission)
	_, err := verifiedManifest(image, &v1.Descriptor{})
	require.ErrorIs(t, err, os.ErrPermission)
	image = NewMockImage(gomock.NewController(t))
	image.EXPECT().RawManifest().Return([]byte("modified"), nil)
	_, err = verifiedManifest(image, &v1.Descriptor{Size: 8})
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
}

func TestPlatformConfigVerifiesContentAndPlatformBinding(t *testing.T) {
	for _, raw := range [][]byte{[]byte("invalid"), []byte(`{"os":"darwin","architecture":"arm64"}`)} {
		image := NewMockImage(gomock.NewController(t))
		image.EXPECT().RawConfigFile().Return(raw, nil)
		digest, size, err := v1.SHA256(bytes.NewReader(raw))
		require.NoError(t, err)
		_, err = inspectPlatformConfig(image, &v1.Manifest{Config: v1.Descriptor{Digest: digest, Size: size}}, &v1.Platform{OS: "linux", Architecture: "amd64"})
		require.Error(t, err)
	}
	image := NewMockImage(gomock.NewController(t))
	image.EXPECT().RawConfigFile().Return(nil, os.ErrPermission)
	_, err := inspectPlatformConfig(image, &v1.Manifest{}, &v1.Platform{})
	require.ErrorIs(t, err, os.ErrPermission)
	image = NewMockImage(gomock.NewController(t))
	image.EXPECT().RawConfigFile().Return([]byte("modified"), nil)
	_, err = inspectPlatformConfig(image, &v1.Manifest{Config: v1.Descriptor{Size: 8}}, &v1.Platform{})
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
}

func TestInspectRejectsImageAnnotationsAndMissingImage(t *testing.T) {
	directory, descriptors := validationFixture(t)
	original, err := BuildIndex(directory, descriptors)
	require.NoError(t, err)
	root, err := original.IndexManifest()
	require.NoError(t, err)
	descriptor := root.Manifests[0]
	image, err := original.Image(descriptor.Digest)
	require.NoError(t, err)
	changed := mutate.Annotations(image, map[string]string{"unexpected": "value"}).(v1.Image)
	rebuilt := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: changed, Descriptor: v1.Descriptor{Platform: descriptor.Platform, Annotations: root.Annotations}})
	rebuilt = mutate.Annotations(rebuilt, root.Annotations).(v1.ImageIndex)
	_, err = Inspect(rebuilt, "")
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	index := NewMockImageIndex(gomock.NewController(t))
	raw, err := json.Marshal(root)
	require.NoError(t, err)
	index.EXPECT().RawManifest().Return(raw, nil)
	index.EXPECT().Image(descriptor.Digest).Return(nil, os.ErrNotExist)
	_, err = Inspect(index, "")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func configurePublishFailure(client *MockRegistry, phase string) {
	if phase == "head" {
		client.EXPECT().Head(gomock.Any(), gomock.Any()).Return(nil, os.ErrPermission)
		return
	}
	client.EXPECT().Head(gomock.Any(), gomock.Any()).Return(nil, &transport.Error{StatusCode: 404})
	if phase == "write" {
		client.EXPECT().WriteIndex(gomock.Any(), gomock.Any(), gomock.Any()).Return(os.ErrPermission)
		return
	}
	client.EXPECT().WriteIndex(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	if phase == "verify" {
		client.EXPECT().Head(gomock.Any(), gomock.Any()).Return(nil, os.ErrPermission)
		return
	}
	client.EXPECT().Head(gomock.Any(), gomock.Any()).Return(&v1.Descriptor{Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}}, nil)
}

func TestExtractionRejectsManifestMismatchAndUnwritableDestination(t *testing.T) {
	directory, descriptors := validationFixture(t)
	index, err := BuildIndex(directory, descriptors)
	require.NoError(t, err)
	release, err := Inspect(index, "")
	require.NoError(t, err)
	archive, err := bundle(map[string][]byte{"plugin.yaml": []byte(strings.ReplaceAll(validationManifest, "acme/fixture", "other/fixture"))}, "bin/plugin", []byte("compiled plugin"))
	require.NoError(t, err)
	require.ErrorIs(t, release.extractPayload(archive, t.TempDir()), errUtils.ErrOCIIntegrity,
		"payload metadata must match the inspected image config even when its file set is valid")
	archive, err = bundle(map[string][]byte{"plugin.yaml": release.Manifest}, "bin/plugin", []byte("compiled plugin"))
	require.NoError(t, err)
	blocked := filepath.Join(t.TempDir(), "destination")
	require.NoError(t, os.WriteFile(blocked, []byte("preserve"), 0o600))
	require.Error(t, release.extractPayload(archive, blocked))
	unchanged, err := os.ReadFile(blocked)
	require.NoError(t, err)
	require.Equal(t, "preserve", string(unchanged))
}

func TestBundleRejectsPathsTooLongForReleaseArchive(t *testing.T) {
	path := "schemas/" + strings.Repeat("a", 200) + "/" + strings.Repeat("b", 200) + ".json"
	raw, err := bundle(map[string][]byte{path: []byte("{}")}, "bin/plugin", []byte("compiled plugin"))
	require.Error(t, err)
	require.Nil(t, raw, "invalid archive paths must not return a partial release")
}
