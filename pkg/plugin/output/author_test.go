package output

import (
	"encoding/json"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

func TestAuthorResultsHumanDefaultsAndMachineFormats(t *testing.T) {
	definition := &manifest.Config{}
	definition.Metadata.Name = "acme/oidc"
	definition.Metadata.Version = "0.1.0"
	release := &pluginoci.Release{Name: definition.Metadata.Name, Version: definition.Metadata.Version, Digest: "sha256:release", Platforms: map[string]v1.Descriptor{"linux/amd64": {}, "darwin/arm64": {}}, Definition: definition}
	bindings := []extension.Binding{{Capability: "auth.provider", Name: "acme/oidc", Metadata: map[string]any{"schema": "schemas/provider.json"}}}
	inventory := []pluginoci.BundleFile{
		{Platform: "linux/amd64", Path: "bin/plugin", Size: 12 << 20, Mode: "0755"},
		{Platform: "linux/amd64", Path: "plugin.yaml", Size: 912, Mode: "0644"},
	}
	tests := []struct {
		name, success, detail string
		render                func(string) error
	}{
		{"validate", "Validated", "auth.provider", func(format string) error { return Validation(definition, bindings, format) }},
		{"pack", "Packed", "release-layout", func(format string) error { return Packed(definition, "release-layout", inventory, format) }},
		{"publish", "Published", "registry.example.com/plugins/oidc", func(format string) error {
			return Published(PublishedRelease{Release: release, Repository: "oci://registry.example.com/plugins/oidc", Signed: true, Files: inventory}, format)
		}},
		{"sign", "Signed", "sha256:release", func(format string) error {
			return Signed(release, "registry.example.com/plugins/oidc@sha256:release", format)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, format := range []string{"table", "json", "yaml", "csv", "tsv"} {
				t.Run(format, func(t *testing.T) {
					stdout, stderr := capture(t)
					require.NoError(t, tt.render(format))
					require.Contains(t, stdout.String(), tt.detail)
					if format == "table" {
						require.Contains(t, stderr.String(), tt.success)
						require.Contains(t, stderr.String(), "acme/oidc")
						require.False(t, json.Valid(stdout.Bytes()))
					} else {
						require.Empty(t, stderr.String(), "explicit machine formats must not emit success decoration")
					}
					if format == "json" {
						require.True(t, json.Valid(stdout.Bytes()))
					}
				})
			}
		})
	}
}

func TestPublicationDoesNotClaimUnsignedReleaseWasSigned(t *testing.T) {
	stdout, stderr := capture(t)
	release := &pluginoci.Release{Name: "acme/oidc", Version: "0.1.0", Digest: "sha256:release"}
	require.NoError(t, Published(PublishedRelease{Release: release, Repository: "registry.example.com/oidc"}, "table"))
	require.Contains(t, stdout.String(), "Not signed by this command")
	require.NotContains(t, stdout.String(), "Signed with Sigstore")
	require.NotContains(t, stdout.String(), "Catalog")
	require.Contains(t, stderr.String(), "Published")
	stdout.Reset()
	stderr.Reset()
	require.NoError(t, Published(PublishedRelease{Release: release, Repository: "registry.example.com/oidc"}, "json"))
	var result PublishedRelease
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.False(t, result.Signed)
	require.Equal(t, release.Digest, result.Digest)
	require.Equal(t, "registry.example.com/oidc", result.Repository)
	require.Empty(t, stderr.String())
}

func TestPackAndPublishShowTheFilesBeingPackaged(t *testing.T) {
	definition := &manifest.Config{}
	definition.Metadata.Name = "acme/oidc"
	definition.Metadata.Version = "0.1.0"
	inventory := []pluginoci.BundleFile{
		{Platform: "linux/amd64", Path: "bin/plugin", Size: 12 << 20, Mode: "0755"},
		{Platform: "linux/amd64", Path: "plugin.yaml", Size: 912, Mode: "0644"},
	}
	release := &pluginoci.Release{Name: "acme/oidc", Version: "0.1.0", Digest: "sha256:release"}
	renderers := map[string]func(string) error{
		"pack": func(format string) error { return Packed(definition, "layout", inventory, format) },
		"publish": func(format string) error {
			return Published(PublishedRelease{Release: release, Repository: "r/x", Files: inventory}, format)
		},
	}
	for name, render := range renderers {
		t.Run(name+"/table", func(t *testing.T) {
			stdout, _ := capture(t)
			require.NoError(t, render("table"))
			require.Contains(t, stdout.String(), "Files (linux/amd64)")
			require.Contains(t, stdout.String(), "bin/plugin  12.0 MiB  0755")
			require.Contains(t, stdout.String(), "plugin.yaml  912 B  0644")
		})
		t.Run(name+"/json", func(t *testing.T) {
			stdout, _ := capture(t)
			require.NoError(t, render("json"))
			var result struct {
				Files []pluginoci.BundleFile `json:"files"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
			require.Equal(t, inventory, result.Files)
		})
	}
	t.Run("empty inventory is an empty array", func(t *testing.T) {
		stdout, _ := capture(t)
		require.NoError(t, Packed(definition, "layout", nil, "json"))
		require.Contains(t, stdout.String(), `"files": []`)
	})
}

func TestSignedResultsIncludeRepositoryQualifiedReference(t *testing.T) {
	release := &pluginoci.Release{Name: "acme/oidc", Version: "0.1.0", Digest: "sha256:release"}
	reference := "registry.example.com/plugins/oidc@" + release.Digest
	for _, format := range []string{"json", "yaml", "table"} {
		t.Run(format, func(t *testing.T) {
			stdout, stderr := capture(t)
			require.NoError(t, Signed(release, reference, format))
			if format == "table" {
				require.Contains(t, stdout.String(), "Reference")
				require.Contains(t, stdout.String(), reference)
				require.Contains(t, stdout.String(), release.Name)
				require.Contains(t, stderr.String(), "Signed")
				return
			}
			var result map[string]any
			if format == "json" {
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
			} else {
				require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &result))
			}
			require.Equal(t, reference, result["reference"], "signing must identify the repository where the signature was written")
			require.Equal(t, release.Name, result["name"], "existing release fields remain at the top level")
			require.Equal(t, release.Version, result["version"])
			require.Equal(t, release.Digest, result["digest"])
			require.Empty(t, stderr.String(), "structured output must not include success decoration")
		})
	}
}
