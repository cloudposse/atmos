package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	plugincmd "github.com/cloudposse/atmos/cmd/plugin"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginCoverageValidationFailures(t *testing.T) {
	config := pluginCoverageProject(t)
	cases := []struct {
		name      string
		arguments []string
		sentinel  error
	}{
		{"run argument delimiter", []string{"run", "missing", "argument"}, errUtils.ErrPluginInvalidConfig},
		{"run absent plugin", []string{"run", "missing"}, errUtils.ErrPluginNotInstalled},
		{"frozen explicit source", []string{"install", "oci://example.com/plugin", "--frozen"}, errUtils.ErrPluginFlagConflict},
		{"invalid catalog source", []string{"install", "missing"}, errUtils.ErrPluginInvalidConfig},
		{"unknown catalog", []string{"install", "unknown/plugin"}, errUtils.ErrPluginInvalidConfig},
		{"unknown search registry", []string{"search", "--registry=unknown"}, errUtils.ErrPluginInvalidConfig},
		{"unknown version update", []string{"update", "missing", "--version=1.2.3"}, errUtils.ErrPluginNotInstalled},
		{"missing info", []string{"info", "missing"}, errUtils.ErrPluginNotInstalled},
		{"bad format", []string{"test", "--format=yaml"}, errUtils.ErrInvalidFlagValue},
		{"missing tests", []string{"test"}, errUtils.ErrPluginInvalidConfig},
		{"missing build", []string{"build"}, errUtils.ErrPluginInvalidConfig},
		{"missing manifest", []string{"validate"}, errUtils.ErrPluginInvalidConfig},
		{"password required", []string{"registry", "login", "example.com", "--username=user"}, errUtils.ErrPluginInvalidConfig},
		{"username required", []string{"registry", "login", "example.com", "--password-stdin"}, errUtils.ErrPluginInvalidConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := invokePluginCoverage(t, config, tc.arguments...)
			require.ErrorIs(t, err, tc.sentinel)
		})
	}
}

// The machine-local development link was replaced by path packages in configuration.
func TestPluginDevFlagsWereRemoved(t *testing.T) {
	root := (&plugincmd.CommandProvider{}).GetCommand()
	for _, arguments := range [][]string{{"install"}, {"remove"}} {
		command, _, err := root.Find(arguments)
		require.NoError(t, err)
		require.Nil(t, command.Flags().Lookup("dev"), "%v must not offer --dev", arguments)
	}
}

func TestPluginCoverageFilesystemAndReferenceErrors(t *testing.T) {
	config := pluginCoverageProject(t)
	cases := [][]string{
		{"pack", "--output", filepath.Join(config.BasePath, "layout")},
		{"catalog", "build", "missing", "--source=oci://example.com/plugin", "--output=missing"},
		{"catalog", "publish", "missing", "--to=unknown"},
		{"catalog", "publish", "missing", "--to=oci://example.com/catalog:latest"},
		{"catalog", "add", "unknown", "not a reference"},
		{"catalog", "remove", "unknown", "example/plugin"},
		{"publish", "missing", "--to=oci://example.com/plugin"},
		{"delete", "example.com/plugin:tag"},
		{"registry", "logout", "not a host"},
		{"registry", "login", "not a host", "--username=user", "--password-stdin"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) { _, err := invokePluginCoverage(t, config, args...); require.Error(t, err) })
	}
	invalid := filepath.Join(config.BasePath, "invalid-catalog.yaml")
	require.NoError(t, os.WriteFile(invalid, []byte("not: [valid"), 0o600))
	_, err := invokePluginCoverage(t, config, "catalog", "publish", invalid, "--to=oci://example.com/catalog:latest")
	require.Error(t, err)
	config.Plugins.Policy.Offline = true
	_, err = invokePluginCoverage(t, config, "search")
	require.NoError(t, err)
	config.Plugins.Registries = map[string]schema.PluginRegistry{
		"uncached": {Repositories: []string{"oci://registry.example.com/plugins/example"}},
	}
	_, err = invokePluginCoverage(t, config, "search")
	require.ErrorIs(t, err, errUtils.ErrPluginOfflineArtifact)
	config.Plugins.Policy.Offline = false
	config.Plugins.Registries = map[string]schema.PluginRegistry{"broken": {Catalog: "not an OCI reference"}}
	_, err = invokePluginCoverage(t, config, "registry", "logout", "broken")
	require.Error(t, err)
}

func TestPluginCoverageRegistryCredentials(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Plugins.Registries = map[string]schema.PluginRegistry{"local": {Catalog: "oci://registry.example.com/catalog:latest"}}
	secretFile := filepath.Join(config.BasePath, "password")
	require.NoError(t, os.WriteFile(secretFile, []byte("fixture-password\r\n"), 0o600))
	input, err := os.Open(secretFile)
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	previous := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = previous })
	_, err = invokePluginCoverage(t, config, "registry", "login", "local", "--username=fixture", "--password-stdin")
	require.NoError(t, err)
	store, err := credentials.OpenKeyring(&config.Auth)
	require.NoError(t, err)
	keychain := atmosoci.CredentialKeychain{Store: store}
	registry, err := name.NewRegistry("registry.example.com")
	require.NoError(t, err)
	authenticator, err := keychain.Resolve(registry)
	require.NoError(t, err)
	authorization, err := authenticator.Authorization()
	require.NoError(t, err)
	require.Equal(t, "fixture", authorization.Username)
	require.Equal(t, "fixture-password", authorization.Password)
	_, err = invokePluginCoverage(t, config, "registry", "logout", "local")
	require.NoError(t, err)
	authenticator, err = keychain.Resolve(registry)
	require.NoError(t, err)
	require.Equal(t, authn.Anonymous, authenticator)
	require.NoError(t, os.WriteFile(secretFile, []byte(strings.Repeat("x", (64<<10)+1)), 0o600))
	_, err = input.Seek(0, 0)
	require.NoError(t, err)
	_, err = invokePluginCoverage(t, config, "registry", "login", "registry.example.com", "--username=fixture", "--password-stdin")
	require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
	require.NoError(t, input.Close())
	_, err = invokePluginCoverage(t, config, "registry", "login", "registry.example.com", "--username=fixture", "--password-stdin")
	require.Error(t, err)
}

func checkPluginConformanceFailure(t *testing.T, config *schema.AtmosConfiguration, directory string) {
	t.Helper()
	path := filepath.Join(directory, "plugin.tests.yaml")
	previous, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("version: 1\ncommands:\n  - name: about\n    stdout: wrong expected output\n"), 0o600))
	_, err = invokePluginCoverage(t, config, "test", directory)
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	require.NoError(t, os.WriteFile(path, previous, 0o600))
}

func checkPluginCatalogFailures(t *testing.T, config *schema.AtmosConfiguration, repository, catalogReference, catalogPath string) {
	t.Helper()
	cases := [][]string{
		{"catalog", "add", "local", repository + ":missing"},
		{"catalog", "add", "local", catalogReference},
		{"catalog", "remove", "local", "missing/provider"},
		{"catalog", "remove", "local", "example/oidc", "--version=9.9.9"},
		{"catalog", "remove", catalogReference + "-missing", "example/oidc"},
		{"catalog", "publish", catalogPath, "--to=" + repository + ":0.1.0"},
		{"install", "local/missing"},
	}
	for _, args := range cases {
		_, err := invokePluginCoverage(t, config, args...)
		require.Error(t, err, "%v", args)
	}
	raw, err := os.ReadFile(catalogPath)
	require.NoError(t, err)
	file, err := catalog.Parse(raw)
	require.NoError(t, err)
	duplicate := file.Spec.Entries[0]
	duplicate.Name = "another/oidc"
	file.Spec.Entries = append(file.Spec.Entries, duplicate)
	raw, err = yaml.Marshal(file)
	require.NoError(t, err)
	ambiguous := filepath.Join(config.BasePath, "ambiguous.yaml")
	require.NoError(t, os.WriteFile(ambiguous, raw, 0o600))
	ref := catalogReference + "-ambiguous"
	_, err = invokePluginCoverage(t, config, "catalog", "publish", ambiguous, "--to="+ref)
	require.NoError(t, err)
	config.Plugins.Registries["ambiguous"] = schema.PluginRegistry{Catalog: ref}
	_, err = invokePluginCoverage(t, config, "install", "ambiguous/oidc")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	delete(config.Plugins.Registries, "ambiguous")
}

func TestPluginCoverageKeyringErrors(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Auth.Keyring.Type = "unknown"
	t.Setenv("ATMOS_KEYRING_TYPE", "unknown")
	input, err := os.CreateTemp(t.TempDir(), "password")
	require.NoError(t, err)
	t.Cleanup(func() { _ = input.Close() })
	previous := os.Stdin
	os.Stdin = input
	t.Cleanup(func() { os.Stdin = previous })
	_, err = invokePluginCoverage(t, config, "registry", "login", "registry.example.com", "--username=fixture", "--password-stdin")
	require.Error(t, err)
	_, err = invokePluginCoverage(t, config, "registry", "logout", "registry.example.com")
	require.Error(t, err)
}

func TestPluginCoverageManagerConfigurationErrors(t *testing.T) {
	config := pluginCoverageProject(t)
	_, err := invokePluginCoverage(t, config, "update", "--config=a.yaml", "--config=b.yaml")
	require.Error(t, err)
	config.Plugins.Policy.CacheTTL = "1h"
	_, err = invokePluginCoverage(t, config, "update")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	config.Plugins.Policy.CacheTTL = ""
	require.NoError(t, os.Remove(filepath.Join(config.BasePath, "atmos.yaml")))
	_, err = invokePluginCoverage(t, config, "registry", "add", "fresh", "oci://example.com/catalog:latest")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(config.BasePath, "atmos.yaml"))
}

func TestPluginCoverageRejectsUnsupportedPackage(t *testing.T) {
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "unsupported")
	require.NoError(t, os.Mkdir(directory, 0o755))
	manifest := `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: example/unsupported
  version: 1.0.0
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/plugin
  compatibility:
    protocol: ">=1 <2"
  capabilities:
    unknown:
      example/unsupported: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(manifest), 0o600))
	_, err := invokePluginCoverage(t, config, "validate", directory)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
}

func TestPluginCommandsPreserveProjectWhenConfigurationIsInvalid(t *testing.T) {
	config := pluginCoverageProject(t)
	config.Plugins.Policy.CacheTTL = "1h"
	path := filepath.Join(config.BasePath, "atmos.yaml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, args := range [][]string{
		{"install"},
		{"install", "oci://registry.example.com/plugins/example"},
		{"remove", "example"},
		{"list"},
		{"sign", "registry.example.com/plugins/example@sha256:" + strings.Repeat("a", 64)},
		{"delete", "registry.example.com/plugins/example@sha256:" + strings.Repeat("a", 64), "--yes"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			output, err := invokePluginCoverage(t, config, args...)
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Contains(t, err.Error(), "cache_ttl")
			require.Empty(t, output)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestPluginConfigReadFailureDoesNotCreateReplacement(t *testing.T) {
	config := pluginCoverageProject(t)
	obstacle := filepath.Join(config.BasePath, "not-a-directory")
	require.NoError(t, os.WriteFile(obstacle, []byte("preserve me"), 0o600))
	_, err := invokePluginCoverage(t, config, "update", "--config="+filepath.Join(obstacle, "atmos.yaml"))
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	require.Contains(t, err.Error(), "failed to stat config path")
	contents, err := os.ReadFile(obstacle)
	require.NoError(t, err)
	require.Equal(t, "preserve me", string(contents))
}
