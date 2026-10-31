package oci_test

import (
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func TestReleasePreservesAuthoredMetadataAcrossPlatforms(t *testing.T) {
	directory, descriptors := packageFixture(t)
	definition, err := manifest.Load(directory)
	require.NoError(t, err)
	definition.Metadata.DisplayName = "Corporate Identity"
	definition.Metadata.Description = "OIDC login for corporate infrastructure"
	definition.Metadata.Author = "Infrastructure team"
	definition.Metadata.Repository = "https://github.com/example/atmos-oidc"
	raw, err := yaml.Marshal(definition)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), raw, 0o600))
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	require.Equal(t, definition.Metadata, release.Definition.Metadata)
	require.Equal(t, "example/fixture", release.Name, "display name must not change canonical package identity")
	for _, platform := range []string{"linux/amd64", "darwin/arm64"} {
		destination := t.TempDir()
		require.NoError(t, release.Extract(platform, destination))
		installed, err := manifest.Load(destination)
		require.NoError(t, err)
		require.Equal(t, definition.Metadata, installed.Metadata)
	}
}

const (
	annotationSource      = "org.opencontainers.image.source"
	annotationTitle       = "org.opencontainers.image.title"
	annotationDescription = "org.opencontainers.image.description"
	annotationImgVersion  = "org.opencontainers.image.version"
)

func writeMetadata(t *testing.T, directory string, mutate func(*manifest.Config)) {
	t.Helper()
	definition, err := manifest.Load(directory)
	require.NoError(t, err)
	mutate(definition)
	raw, err := yaml.Marshal(definition)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), raw, 0o600))
}

// indexAnnotations returns the annotations on the index, each platform descriptor, and each platform manifest.
func indexAnnotations(t *testing.T, index v1.ImageIndex) []map[string]string {
	t.Helper()
	root, err := index.IndexManifest()
	require.NoError(t, err)
	result := []map[string]string{root.Annotations}
	for position := range root.Manifests {
		descriptor := &root.Manifests[position]
		result = append(result, descriptor.Annotations)
		image, err := index.Image(descriptor.Digest)
		require.NoError(t, err)
		imageManifest, err := image.Manifest()
		require.NoError(t, err)
		result = append(result, imageManifest.Annotations)
	}
	require.Len(t, result, 5)
	return result
}

func TestReleaseLinksPackageToRepositoryFromManifestMetadata(t *testing.T) {
	directory, descriptors := packageFixture(t)
	writeMetadata(t, directory, func(definition *manifest.Config) {
		definition.Metadata.DisplayName = "Fixture"
		definition.Metadata.Description = "A fixture plugin"
		definition.Metadata.Repository = "https://github.com/example/atmos-fixture"
	})
	index, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	for _, annotations := range indexAnnotations(t, index) {
		require.Equal(t, "https://github.com/example/atmos-fixture", annotations[annotationSource])
		require.Equal(t, "Fixture", annotations[annotationTitle])
		require.Equal(t, "A fixture plugin", annotations[annotationDescription])
		require.Equal(t, "1.2.3", annotations[annotationImgVersion])
	}
	_, err = pluginoci.Inspect(index, "")
	require.NoError(t, err, "derived annotations are part of the verified release shape")
}

func TestReleaseOmitsSourceWithoutHTTPSRepository(t *testing.T) {
	for name, repository := range map[string]string{"absent": "", "ssh": "git@github.com:example/atmos-fixture.git", "plain http": "http://github.com/example/x", "credentials": "https://user:token@github.com/example/x"} {
		t.Run(name, func(t *testing.T) {
			directory, descriptors := packageFixture(t)
			writeMetadata(t, directory, func(definition *manifest.Config) { definition.Metadata.Repository = repository })
			index, err := pluginoci.BuildIndex(directory, descriptors)
			require.NoError(t, err)
			for _, annotations := range indexAnnotations(t, index) {
				require.NotContains(t, annotations, annotationSource, "an unlinkable or credentialed repository must never be published")
			}
			_, err = pluginoci.Inspect(index, "")
			require.NoError(t, err)
		})
	}
}

func TestWithSourceOverridesManifestRepository(t *testing.T) {
	directory, descriptors := packageFixture(t)
	writeMetadata(t, directory, func(definition *manifest.Config) { definition.Metadata.Repository = "https://github.com/example/old" })
	index, err := pluginoci.BuildIndex(directory, descriptors, pluginoci.WithSource("https://github.com/example/new"))
	require.NoError(t, err)
	for _, annotations := range indexAnnotations(t, index) {
		require.Equal(t, "https://github.com/example/new", annotations[annotationSource])
	}
	_, err = pluginoci.Inspect(index, "")
	require.NoError(t, err)
	plain, err := pluginoci.BuildIndex(directory, descriptors)
	require.NoError(t, err)
	require.NotEqual(t, mustDigest(t, plain), mustDigest(t, index), "the explicit source is part of the release digest")
}

func TestWithSourceRejectsUnsafeValues(t *testing.T) {
	directory, descriptors := packageFixture(t)
	for _, source := range []string{"http://github.com/example/x", "https://user:token@github.com/example/x", "https://github.com/example/x?token=1", "https://github.com/example/x#frag", "ssh://github.com/example/x", "not a url", "https://"} {
		t.Run(source, func(t *testing.T) {
			_, err := pluginoci.BuildIndex(directory, descriptors, pluginoci.WithSource(source))
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		})
	}
}

func TestInspectRejectsTamperedSourceAnnotation(t *testing.T) {
	directory, descriptors := packageFixture(t)
	index, err := pluginoci.BuildIndex(directory, descriptors, pluginoci.WithSource("https://github.com/example/new"))
	require.NoError(t, err)
	for _, forged := range []string{"https://user:token@github.com/example/new", "http://github.com/example/new"} {
		root, err := index.IndexManifest()
		require.NoError(t, err)
		annotations := map[string]string{}
		for key, value := range root.Annotations {
			annotations[key] = value
		}
		annotations[annotationSource] = forged
		tampered := mutate.Annotations(index, annotations).(v1.ImageIndex)
		_, err = pluginoci.Inspect(tampered, "")
		require.Error(t, err, forged)
	}
}

func TestGitHubActionsSourceIsDerivedOnlyFromExplicitEnvironment(t *testing.T) {
	environment := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	require.Equal(t, "https://github.com/cloudposse/atmos", pluginoci.GitHubActionsSource(environment(map[string]string{"GITHUB_REPOSITORY": "cloudposse/atmos"})))
	require.Equal(t, "https://ghe.example.com/team/repo", pluginoci.GitHubActionsSource(environment(map[string]string{"GITHUB_SERVER_URL": "https://ghe.example.com/", "GITHUB_REPOSITORY": "team/repo"})))
	require.Empty(t, pluginoci.GitHubActionsSource(environment(nil)), "outside GitHub Actions nothing is derived")
	require.Empty(t, pluginoci.GitHubActionsSource(environment(map[string]string{"GITHUB_REPOSITORY": "not-a-slug"})))
	require.Empty(t, pluginoci.GitHubActionsSource(environment(map[string]string{"GITHUB_SERVER_URL": "http://insecure.example.com", "GITHUB_REPOSITORY": "a/b"})))
}

func mustDigest(t *testing.T, index v1.ImageIndex) string {
	t.Helper()
	digest, err := index.Digest()
	require.NoError(t, err)
	return digest.String()
}
