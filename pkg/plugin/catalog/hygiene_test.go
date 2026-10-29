package catalog_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func TestNormalizeVersionStripsOnlyALeadingV(t *testing.T) {
	for input, want := range map[string]string{"v0.1.0": "0.1.0", "V1.2.3": "1.2.3", "0.1.0": "0.1.0", "": "", "1.0.0-rc.v2": "1.0.0-rc.v2"} {
		assert.Equal(t, want, catalog.NormalizeVersion(input), input)
	}
}

func TestRemoveMatchesVersionsWithOrWithoutLeadingV(t *testing.T) {
	for _, version := range []string{"1.0.0", "v1.0.0"} {
		file := catalog.New("acme")
		file.Spec.Entries = []catalog.Entry{entry()}
		require.NoError(t, catalog.Remove(file, "acme/auth", version), version)
		require.Len(t, file.Spec.Entries[0].Versions, 1)
		assert.Equal(t, "1.1.0", file.Spec.Entries[0].Versions[0].Version)
	}
}

func TestRemoveReportsWhichVersionIsMissingAndWhatIsAvailable(t *testing.T) {
	file := catalog.New("acme")
	file.Spec.Entries = []catalog.Entry{entry()}
	err := catalog.Remove(file, "acme/auth", "v9.9.9")
	require.ErrorIs(t, err, errUtils.ErrPluginCatalogVersionNotFound)
	require.NotErrorIs(t, err, errUtils.ErrPluginVersionNotFound, "this is not a release-constraint failure")
	assert.Contains(t, err.Error(), "9.9.9")
	assert.Contains(t, err.Error(), "available: 1.0.0, 1.1.0")
	assert.Len(t, file.Spec.Entries[0].Versions, 2, "a failed removal changes nothing")
}

func TestRemoveReportsUnknownPackageWithListedNames(t *testing.T) {
	file := catalog.New("acme")
	file.Spec.Entries = []catalog.Entry{entry()}
	err := catalog.Remove(file, "acme/missing", "")
	require.ErrorIs(t, err, errUtils.ErrPluginCatalogEntryNotFound)
	require.NotErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), " "), "acme/auth")
	empty := catalog.New("empty")
	require.ErrorIs(t, catalog.Remove(empty, "acme/missing", ""), errUtils.ErrPluginCatalogEntryNotFound)
}

func TestAddStoresNormalizedVersionsAndStaysIdempotent(t *testing.T) {
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "auth.provider", Path: []string{"auth", "providers"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	definition := &manifest.Config{}
	definition.Spec.Runtime.Type = "native"
	definition.Spec.Capabilities = map[string]any{"auth": map[string]any{"providers": map[string]any{"acme/auth": map[string]any{}}}}
	release := &pluginoci.Release{Name: "acme/auth", Version: "v1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), Definition: definition}
	file := catalog.New("acme")
	require.NoError(t, catalog.Add(file, "oci://ghcr.io/acme/auth", release, descriptors))
	release.Version = "1.0.0"
	require.NoError(t, catalog.Add(file, "oci://ghcr.io/acme/auth", release, descriptors))
	require.Len(t, file.Spec.Entries[0].Versions, 1)
	assert.Equal(t, "1.0.0", file.Spec.Entries[0].Versions[0].Version)
}

func TestParseReleaseReferenceRequiresATagOrDigestLocally(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, accepted := range []string{"oci://localhost:5000/acme/auth:v1", "localhost:5000/acme/auth:v1", "oci://localhost:5000/acme/auth@" + digest} {
		reference, err := catalog.ParseReleaseReference(accepted)
		require.NoError(t, err, accepted)
		assert.True(t, strings.HasPrefix(reference.Name(), "localhost:5000/acme/auth"), reference.Name())
	}
	for _, rejected := range []string{"oci://localhost:5000/acme/auth", "ghcr.io/acme/auth", "oci://alpine"} {
		_, err := catalog.ParseReleaseReference(rejected)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, rejected)
		hints := strings.Join(errUtils.AllHints(err), " ")
		assert.Contains(t, hints, "tag or digest", rejected)
		assert.NotContains(t, err.Error(), "latest", "the registry's implicit default must not leak into the message")
	}
	_, err := catalog.ParseReleaseReference("not a reference:")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}
