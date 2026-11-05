package lock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func validEntry() Entry {
	return Entry{Plugin: "example/oidc", Source: "oci://registry.example/oidc", Constraint: "^1.0.0", Version: "1.2.3", IndexDigest: digest.FromString("index").String(), Platforms: map[string]string{"linux/amd64": digest.FromString("platform").String()}, Runtime: manifest.Runtime{Type: "native", Protocol: 1, Executable: "bin/plugin"}, Exports: []Export{{Capability: "auth.provider", Name: "example/oidc"}}, Verification: Verification{Signature: "unverified"}}
}

func TestLockRoundTripAndFrozenAgreement(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	file, err := Load(directory)
	require.NoError(t, err)
	require.Empty(t, file.Spec.Packages)
	file.Spec.Packages["corporate"] = validEntry()
	before := time.Now().Add(-time.Second)
	require.NoError(t, Save(directory, file))
	loaded, err := Load(directory)
	require.NoError(t, err)
	require.Equal(t, file.Spec.Packages, loaded.Spec.Packages)
	require.True(t, loaded.Spec.GeneratedAt.After(before))
	declarations := map[string]schema.PluginPackage{"corporate": {Source: validEntry().Source, Version: "^1.0.0"}}
	require.NoError(t, CheckDeclarations(loaded, declarations))
	require.ErrorIs(t, CheckDeclarations(loaded, nil), errUtils.ErrPluginFrozenDrift)
	for _, values := range []map[string]schema.PluginPackage{
		{"renamed": declarations["corporate"]},
		{"corporate": {Source: "oci://other/release", Version: "^1.0.0"}},
		{"corporate": {Source: validEntry().Source, Version: "^2.0.0"}},
	} {
		require.ErrorIs(t, CheckDeclarations(loaded, values), errUtils.ErrPluginFrozenDrift)
	}
	entry := validEntry()
	release := &pluginoci.Release{Name: entry.Plugin, Version: entry.Version, Digest: entry.IndexDigest, Platforms: map[string]v1.Descriptor{"linux/amd64": {Digest: v1.Hash{Algorithm: "sha256", Hex: strings.TrimPrefix(entry.Platforms["linux/amd64"], "sha256:")}}}}
	require.NoError(t, CheckRelease(&entry, release))
	release.Platforms["linux/amd64"] = v1.Descriptor{Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}}
	require.ErrorIs(t, CheckRelease(&entry, release), errUtils.ErrPluginFrozenDrift)
	release.Digest = digest.FromString("moved index").String()
	require.ErrorIs(t, CheckRelease(&entry, release), errUtils.ErrPluginFrozenDrift)
}

func TestLockRejectsCorruptOrUntrustedEntries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		change   func(*Entry)
		expected error
	}{
		{"missing identity", func(e *Entry) { e.Plugin = "" }, errUtils.ErrPluginInvalidConfig},
		{"mutable non OCI source", func(e *Entry) { e.Source = "https://example.com/plugin" }, errUtils.ErrPluginInvalidConfig},
		{"escaping executable", func(e *Entry) { e.Runtime.Executable = "../outside" }, errUtils.ErrPluginInvalidConfig},
		{"no platforms", func(e *Entry) { e.Platforms = nil }, errUtils.ErrPluginInvalidConfig},
		{"wrong digest algorithm", func(e *Entry) { e.IndexDigest = "sha512:" + strings.Repeat("a", 64) }, errUtils.ErrPluginInvalidConfig},
		{"invalid hex digest", func(e *Entry) { e.IndexDigest = "sha256:" + strings.Repeat("z", 64) }, errUtils.ErrPluginInvalidConfig},
		{"non canonical digest", func(e *Entry) { e.IndexDigest = "sha256:" + strings.Repeat("A", 64) }, errUtils.ErrPluginInvalidConfig},
		{"bad platform digest", func(e *Entry) { e.Platforms["linux/amd64"] = "sha256:short" }, errUtils.ErrPluginInvalidConfig},
		{"empty export", func(e *Entry) { e.Exports = []Export{{Name: "example/oidc"}} }, errUtils.ErrPluginInvalidConfig},
		{"duplicate export", func(e *Entry) { e.Exports = append(e.Exports, e.Exports[0]) }, errUtils.ErrPluginInvalidConfig},
		{"invalid constraint", func(e *Entry) { e.Constraint = "not semver" }, errUtils.ErrPluginInvalidConfig},
		{"invalid version", func(e *Entry) { e.Version = "invalid" }, errUtils.ErrPluginFrozenDrift},
		{"version outside constraint", func(e *Entry) { e.Version = "2.0.0" }, errUtils.ErrPluginFrozenDrift},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			entry := validEntry()
			test.change(&entry)
			file := New()
			file.Spec.Packages["corporate"] = entry
			require.NoError(t, Save(directory, file))
			loaded, err := Load(directory)
			require.ErrorIs(t, err, test.expected)
			require.Nil(t, loaded)
		})
	}
}

func TestLockRejectsInvalidFilesAndWriteFailures(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	file := New()
	file.Spec.LockVersion = 2
	raw, err := yaml.Marshal(file)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(Path(directory), raw, 0o600))
	_, err = Load(directory)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NoError(t, os.WriteFile(Path(directory), []byte("{invalid yaml"), 0o600))
	_, err = Load(directory)
	require.Error(t, err)
	require.NoError(t, os.Remove(Path(directory)))
	require.NoError(t, os.Mkdir(Path(directory), 0o700))
	_, err = Load(directory)
	require.Error(t, err)
	require.Error(t, Save(directory, New()))
	_, err = Load(filepath.Join(directory, "missing"))
	require.NoError(t, err)
}
