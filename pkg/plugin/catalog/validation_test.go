package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func TestCatalogRejectsInvalidDiscoveryMetadata(t *testing.T) {
	tests := map[string]func(*catalog.File){
		"empty name":         func(f *catalog.File) { f.Spec.Entries[0].Name = "" },
		"duplicate name":     func(f *catalog.File) { f.Spec.Entries = append(f.Spec.Entries, f.Spec.Entries[0]) },
		"invalid repository": func(f *catalog.File) { f.Spec.Entries[0].Source = "bad repository" },
		"duplicate version": func(f *catalog.File) {
			f.Spec.Entries[0].Versions = append(f.Spec.Entries[0].Versions, f.Spec.Entries[0].Versions[0])
		},
		"invalid version": func(f *catalog.File) { f.Spec.Entries[0].Versions[0].Version = "one" },
		"invalid digest":  func(f *catalog.File) { f.Spec.Entries[0].Versions[0].Digest = "sha256:bad" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			f := catalog.New("acme")
			f.Spec.Entries = []catalog.Entry{entry()}
			mutate(f)
			raw, err := json.Marshal(f)
			require.NoError(t, err)
			_, err = catalog.Parse(raw)
			require.Error(t, err)
		})
	}
	_, err := catalog.Parse([]byte("invalid: ["))
	require.Error(t, err)
}

func TestCatalogAddsImmutableReleasesAndPreservesVersions(t *testing.T) {
	descriptors := extension.New()
	require.NoError(t, descriptors.Register(extension.Descriptor{Capability: "auth.provider", Path: []string{"auth", "providers"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	definition := &manifest.Config{}
	definition.Spec.Runtime.Type = "native"
	definition.Spec.Capabilities = map[string]any{"auth": map[string]any{"providers": map[string]any{"acme/auth": map[string]any{}}}}
	release := &pluginoci.Release{Name: "acme/auth", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), Definition: definition}
	file := catalog.New("acme")
	require.NoError(t, catalog.Add(file, "oci://ghcr.io/acme/auth", release, descriptors))
	require.NoError(t, catalog.Add(file, "ghcr.io/acme/auth", release, descriptors))
	require.Equal(t, "oci://ghcr.io/acme/auth", file.Spec.Entries[0].Source)
	require.Len(t, file.Spec.Entries[0].Versions, 1)
	require.Equal(t, []string{"auth.provider"}, file.Spec.Entries[0].Capabilities)
	release.Digest = "sha256:" + strings.Repeat("b", 64)
	require.ErrorIs(t, catalog.Add(file, "oci://ghcr.io/acme/auth", release, descriptors), errUtils.ErrPluginImmutableRelease)
	require.ErrorIs(t, catalog.Add(file, "oci://ghcr.io/other/auth", release, descriptors), errUtils.ErrPluginCatalogConflict)
	release.Version = "1.1.0"
	require.NoError(t, catalog.Add(file, "oci://ghcr.io/acme/auth", release, descriptors))
	require.Equal(t, "1.0.0", file.Spec.Entries[0].Versions[0].Version)
	require.Equal(t, "1.1.0", file.Spec.Entries[0].Versions[1].Version)
	release.Name = "aaa/auth"
	require.NoError(t, catalog.Add(file, "oci://ghcr.io/aaa/auth", release, descriptors))
	require.Equal(t, "aaa/auth", file.Spec.Entries[0].Name)
	require.Equal(t, "acme/auth", file.Spec.Entries[1].Name)
	require.Error(t, catalog.Add(file, "oci://ghcr.io/acme/auth", release, extension.New()))
	require.ErrorIs(t, catalog.Add(file, "bad repository", release, descriptors), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, catalog.Remove(file, "acme/auth", "9.0.0"), errUtils.ErrPluginCatalogVersionNotFound)
	require.NoError(t, catalog.Remove(file, "acme/auth", ""))
	require.ErrorIs(t, catalog.Remove(file, "absent", ""), errUtils.ErrPluginCatalogEntryNotFound)
	require.Equal(t, "aaa/auth", file.Spec.Entries[0].Name)
}

func TestCatalogNormalizesSourcesBeforeConsumerInstallation(t *testing.T) {
	for _, source := range []string{"localhost:15555/plugins/oidc", "oci://localhost:15555/plugins/oidc"} {
		t.Run(source, func(t *testing.T) {
			file := catalog.New("local")
			value := entry()
			value.Source = source
			file.Spec.Entries = []catalog.Entry{value}
			raw, err := json.Marshal(file)
			require.NoError(t, err)
			parsed, err := catalog.Parse(raw)
			require.NoError(t, err)
			require.Equal(t, "oci://localhost:15555/plugins/oidc", parsed.Spec.Entries[0].Source)
		})
	}
}

func TestCatalogFilteringAndInvalidSelection(t *testing.T) {
	file := catalog.New("acme")
	file.Spec.Entries = []catalog.Entry{entry()}
	require.Empty(t, catalog.Search(file, catalog.Filter{Capability: "command"}))
	require.Empty(t, catalog.Search(file, catalog.Filter{Query: "does not exist"}))
	value := entry()
	_, err := catalog.Select(&value, "invalid constraint", "linux/amd64")
	require.Error(t, err)
	value.Versions[0].Version = "invalid"
	_, err = catalog.Select(&value, "*", "linux/amd64")
	require.Error(t, err)
}
