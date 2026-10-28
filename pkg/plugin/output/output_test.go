package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:generate go run go.uber.org/mock/mockgen -destination mock_streams_test.go -package output github.com/cloudposse/atmos/pkg/io Streams

func capture(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("ATMOS_FORCE_TTY", "false")
	t.Setenv("ATMOS_FORCE_COLOR", "false")
	t.Setenv("NO_COLOR", "1")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	previous := iolib.GetContext()
	streams := NewMockStreams(gomock.NewController(t))
	streams.EXPECT().RawOutput().Return(stdout).AnyTimes()
	streams.EXPECT().RawError().Return(stderr).AnyTimes()
	ctx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ctx)
	ui.InitFormatter(ctx)
	t.Cleanup(func() {
		data.InitWriter(previous)
		ui.InitFormatter(previous)
	})
	return stdout, stderr
}

func TestSearchPreservesStructuredMetadata(t *testing.T) {
	result := SearchResult{Registry: "corporate", Entry: catalog.Entry{
		Name: "acme/oidc", Description: "Sign in, exchange credentials", Publisher: "acme",
		Capabilities: []string{"auth.provider", "command"}, Runtime: "native",
		Source: "oci://ghcr.io/acme/oidc",
		Versions: []catalog.Version{
			{Version: "1.2.0", Digest: "sha256:release", Platforms: []string{"linux/amd64", "darwin/arm64"}},
		},
	}}
	for _, outputFormat := range []string{"json", "yaml"} {
		t.Run(outputFormat, func(t *testing.T) {
			stdout, stderr := capture(t)
			require.NoError(t, Search([]SearchResult{result}, outputFormat))
			var actual []SearchResult
			if outputFormat == "json" {
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
			} else {
				require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &actual))
				require.NotContains(t, stdout.String(), "entry:")
			}
			require.Equal(t, []SearchResult{result}, actual)
			require.Empty(t, stderr.String(), "machine output must not include status messages")
		})
	}
}

func TestSearchDelimitedOutputQuotesDescriptions(t *testing.T) {
	for _, outputFormat := range []string{"csv", "tsv"} {
		t.Run(outputFormat, func(t *testing.T) {
			stdout, stderr := capture(t)
			description := "Custom auth, with \"quotes\"\nand\ttabs"
			require.NoError(t, Search([]SearchResult{{Registry: "corporate", Entry: catalog.Entry{
				Name: "acme/auth", Description: description, Capabilities: []string{"auth.provider", "command"},
			}}}, outputFormat))
			reader := csv.NewReader(stdout)
			if outputFormat == "tsv" {
				reader.Comma = '\t'
			}
			rows, err := reader.ReadAll()
			require.NoError(t, err)
			require.Equal(t, [][]string{
				{"Plugin", "Capabilities", "Registry", "Description"},
				{"acme/auth", "auth.provider, command", "corporate", description},
			}, rows)
			require.Empty(t, stderr.String())
		})
	}
}

func TestListLabelsPathPackagesFromTheirLocalManifestNotTheLock(t *testing.T) {
	stdout, stderr := capture(t)
	release := &pluginlock.Entry{Plugin: "acme/auth", Version: "1.2.0"}
	statuses := []manage.Status{
		{Alias: "auth", State: "installed", Release: release},
		{Alias: "local-auth", State: "path", Source: "path", Plugin: "acme/auth", Version: "1.3.0-dev", Release: release},
		{Alias: "unbuilt", State: "broken", Source: "path", Hint: "Build it."},
		{Alias: "pending", State: "missing"},
		{Alias: "off", State: "disabled", Release: release},
	}
	require.NoError(t, List(statuses, "csv"))
	rows, err := csv.NewReader(stdout).ReadAll()
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"Alias", "Plugin", "Version", "Status"},
		{"auth", "acme/auth", "1.2.0", "installed"},
		{"local-auth", "acme/auth", "1.3.0-dev", "path"},
		{"unbuilt", "-", "-", "broken"},
		{"pending", "-", "-", "missing"},
		{"off", "acme/auth", "1.2.0", "disabled"},
	}, rows)
	require.Contains(t, stderr.String(), "Build it.", "a broken path package prints its repair hint on the UI stream")
}

func TestListStructuredOutputRetainsLockDetails(t *testing.T) {
	status := manage.Status{
		Alias: "auth", State: "installed", Path: "/cache/auth",
		Release: &pluginlock.Entry{
			Plugin: "acme/auth", Version: "1.2.0", IndexDigest: "sha256:release",
			Platforms: map[string]string{"linux/amd64": "sha256:platform"},
			Exports:   []pluginlock.Export{{Capability: "auth.provider", Name: "acme/oidc"}},
		},
	}
	for _, outputFormat := range []string{"json", "yaml"} {
		t.Run(outputFormat, func(t *testing.T) {
			stdout, stderr := capture(t)
			require.NoError(t, List([]manage.Status{status}, outputFormat))
			var actual []manage.Status
			if outputFormat == "json" {
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
			} else {
				require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &actual))
			}
			require.Equal(t, []manage.Status{status}, actual)
			require.Empty(t, stderr.String())
		})
	}
}

func TestRegistriesSortedAndStructured(t *testing.T) {
	registries := map[string]schema.PluginRegistry{
		"zulu":      {Catalog: "oci://ghcr.io/zulu/plugins:stable", Auth: "docker"},
		"alpha":     {Host: "ghcr.io", Catalog: "oci://ghcr.io/alpha/plugins:stable", Repositories: []string{"oci://ghcr.io/alpha/one", "oci://ghcr.io/alpha/two"}, Identity: "registry-reader"},
		"host-only": {Host: "private.example.com"},
	}
	for _, outputFormat := range []string{"table", "json", "yaml"} {
		t.Run(outputFormat, func(t *testing.T) {
			stdout, stderr := capture(t)
			require.NoError(t, Registries(registries, outputFormat))
			if outputFormat == "table" {
				output := stdout.String()
				require.NotContains(t, output, "\x1b", "piped tables must be plain text")
				require.Contains(t, output, registries["alpha"].Catalog)
				require.Contains(t, output, registries["zulu"].Catalog)
				require.Contains(t, output, "registry-reader")
				require.Contains(t, output, "oci://ghcr.io/alpha/one")
				require.Contains(t, output, "oci://ghcr.io/alpha/two")
				require.Contains(t, output, "private.example.com")
				require.Less(t, strings.Index(output, "alpha"), strings.Index(output, "zulu"))
			} else {
				var actual map[string]schema.PluginRegistry
				if outputFormat == "json" {
					require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
				} else {
					require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &actual))
				}
				require.Equal(t, registries, actual)
			}
			require.Empty(t, stderr.String())
		})
	}
}

func TestInfoDistinguishesPathMetadataFromLockedRelease(t *testing.T) {
	status := &manage.Status{
		Alias: "auth", State: "path", Source: "path", Path: "/local/auth",
		Release: &pluginlock.Entry{
			Plugin: "acme/auth", Version: "1.2.0", Source: "oci://ghcr.io/acme/auth",
			Constraint: "^1.0.0", IndexDigest: "sha256:release", Verification: pluginlock.Verification{Signature: "unverified"},
			Platforms: map[string]string{"linux/amd64": "sha256:platform"},
			Exports:   []pluginlock.Export{{Capability: "auth.provider", Name: "acme/oidc"}},
		},
		Manifest: &manifest.Config{},
	}
	status.Manifest.Metadata.Name = "acme/auth"
	status.Manifest.Metadata.Version = "1.3.0"
	status.Manifest.Metadata.Description = "Local development"
	for _, outputFormat := range []string{"json", "yaml", "csv"} {
		t.Run(outputFormat, func(t *testing.T) {
			stdout, stderr := capture(t)
			require.NoError(t, Info(status, outputFormat))
			if outputFormat == "csv" {
				rows, err := csv.NewReader(stdout).ReadAll()
				require.NoError(t, err)
				require.Contains(t, rows, []string{"Version", "1.3.0"})
				require.Contains(t, rows, []string{"Locked version", "1.2.0"})
				require.Contains(t, rows, []string{"Status", "path"})
				require.Contains(t, rows, []string{"Origin", "plugins.packages.auth.path (local build; lock, signature and version checks are bypassed)"})
				require.Contains(t, rows, []string{"Index digest", "sha256:release"})
				require.Contains(t, rows, []string{"Source", "oci://ghcr.io/acme/auth"})
			} else {
				var actual manage.Status
				if outputFormat == "json" {
					require.NoError(t, json.Unmarshal(stdout.Bytes(), &actual))
				} else {
					require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &actual))
				}
				require.Equal(t, *status, actual)
			}
			require.Empty(t, stderr.String())
		})
	}
	require.ErrorIs(t, Info(nil, "json"), errUtils.ErrPluginNotInstalled)
}

func TestEmptyCollectionsAndInvalidFormats(t *testing.T) {
	commands := []struct {
		name  string
		run   func(string) error
		empty string
	}{
		{"search", func(outputFormat string) error { return Search(nil, outputFormat) }, "[]"},
		{"list", func(outputFormat string) error { return List(nil, outputFormat) }, "[]"},
		{"registries", func(outputFormat string) error { return Registries(nil, outputFormat) }, "{}"},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			stdout, stderr := capture(t)
			for _, outputFormat := range []string{"json", "yaml"} {
				require.NoError(t, command.run(outputFormat))
				require.Equal(t, command.empty, strings.TrimSpace(stdout.String()))
				require.Empty(t, stderr.String())
				stdout.Reset()
			}
			require.NoError(t, command.run("table"))
			require.Empty(t, stdout.String())
			require.Contains(t, stderr.String(), "No ")
			stderr.Reset()
			require.ErrorIs(t, command.run("xml"), errUtils.ErrInvalidFlagValue)
			require.Empty(t, stdout.String())
			require.Empty(t, stderr.String())
		})
	}
}
