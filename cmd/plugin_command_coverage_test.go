package cmd

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	containerregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	plugincmd "github.com/cloudposse/atmos/cmd/plugin"
	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// invokePluginCoverage exercises the registered command's own argument, flag and
// execution contracts after supplying the resolved configuration normally set by startup.
func invokePluginCoverage(t *testing.T, config *schema.AtmosConfiguration, arguments ...string) (string, error) {
	t.Helper()
	_ = NewTestKit(t)
	plugincmd.SetAtmosConfig(config)
	t.Cleanup(func() { plugincmd.SetAtmosConfig(nil) })
	command, remaining, err := (&plugincmd.CommandProvider{}).GetCommand().Find(arguments)
	require.NoError(t, err)
	previousFlags := command.Flags()
	command.ResetFlags()
	command.Flags().AddFlagSet(previousFlags)
	command.Flags().VisitAll(resetFlagToDefault)
	previousContext := command.Context()
	t.Cleanup(func() { command.SetContext(previousContext) })
	command.SetContext(t.Context())
	require.NoError(t, command.ParseFlags(remaining))
	args := command.Flags().Args()
	if command.Args != nil {
		if err := command.Args(command, args); err != nil {
			return "", err
		}
	}
	if err := command.ValidateRequiredFlags(); err != nil {
		return "", err
	}
	output, err := os.CreateTemp(t.TempDir(), "output")
	require.NoError(t, err)
	previous := os.Stdout
	os.Stdout = output
	atmosio.Reset()
	defer func() { os.Stdout = previous; atmosio.Reset(); _ = output.Close() }()
	err = command.RunE(command, args)
	_, seekErr := output.Seek(0, 0)
	require.NoError(t, seekErr)
	raw, readErr := io.ReadAll(output)
	require.NoError(t, readErr)
	return string(raw), err
}

func pluginCoverageProject(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	_ = NewTestKit(t)
	directory := t.TempDir()
	t.Chdir(directory)
	_ = pluginTestEnvironment(t, directory)
	config := &schema.AtmosConfiguration{BasePath: directory, BasePathConfigDir: directory, Plugins: schema.PluginsConfig{CacheDir: filepath.Join(directory, "cache"), Discovery: schema.PluginDiscoveryConfig{DefaultRegistry: new(false)}}, Auth: schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "file", Spec: map[string]any{"path": filepath.Join(directory, "keyring")}}}}
	writePluginCoverageConfig(t, config)
	return config
}

func writePluginCoverageConfig(t *testing.T, config *schema.AtmosConfiguration) {
	t.Helper()
	raw, err := yaml.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(config.BasePath, "atmos.yaml"), raw, 0o600))
}

func reloadPluginCoverageConfig(t *testing.T, config *schema.AtmosConfiguration) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(config.BasePath, "atmos.yaml"))
	require.NoError(t, err)
	var loaded schema.AtmosConfiguration
	require.NoError(t, yaml.Unmarshal(raw, &loaded))
	loaded.BasePathConfigDir = config.BasePathConfigDir
	*config = loaded
}

func TestPluginCoverageAuthoringAndLifecycle(t *testing.T) {
	_ = NewTestKit(t)
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "plugin")
	run := func(args ...string) string {
		t.Helper()
		output, err := invokePluginCoverage(t, config, args...)
		require.NoError(t, err, "%v", args)
		return output
	}
	run("create", directory, "--defaults", "--sdk-path", filepath.Join(repository, "sdk"), "--display-name=Corporate Login", "--description=Corporate OIDC provider", "--author=Platform Team", "--repository=https://example.com/platform/oidc")
	definition, err := manifest.Load(directory)
	require.NoError(t, err)
	require.Equal(t, "Corporate Login", definition.Metadata.DisplayName)
	require.Equal(t, "Corporate OIDC provider", definition.Metadata.Description)
	require.Equal(t, "Platform Team", definition.Metadata.Author)
	require.Equal(t, "https://example.com/platform/oidc", definition.Metadata.Repository)
	require.FileExists(t, filepath.Join(directory, "go.mod"))
	require.Contains(t, run("validate", directory), "example/oidc")
	run("build", directory)
	require.FileExists(t, filepath.Join(directory, definition.Spec.Runtime.Executable))
	require.Contains(t, run("test", directory, "--format=json"), "oidc-lifecycle")
	run("test", directory, "--run=command/")
	checkPluginConformanceFailure(t, config, directory)
	// A profile that sets path runs the local build instead of any release.
	config.Plugins.Packages = map[string]schema.PluginPackage{"example": {Path: directory}}
	config.ActiveProfiles = []string{"sandbox"}
	listing := run("list")
	require.Contains(t, listing, "example")
	require.Contains(t, listing, "path")
	require.Contains(t, run("info", "example"), directory)
	require.Contains(t, run("run", "example", "--"), "OIDC PKCE")
	_, err = invokePluginCoverage(t, config, "remove", "example")
	require.ErrorIs(t, err, errUtils.ErrPluginPathPackage)
	require.Contains(t, formatPluginError(err), "sandbox")
	config.Plugins.Packages = nil
	config.ActiveProfiles = nil
	layout := filepath.Join(config.BasePath, "layout")
	run("pack", directory, "--output", layout)
	server := httptest.NewServer(containerregistry.New(containerregistry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	repositoryRef := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/plugins/oidc"
	catalogRef := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/catalog:latest"
	run("registry", "add", "local", catalogRef)
	reloadPluginCoverageConfig(t, config)
	require.Contains(t, run("registry", "list"), catalogRef)
	require.Contains(t, run("publish", layout, "--to", strings.TrimPrefix(repositoryRef, "oci://")), "example/oidc")
	// Catalog maintenance is an explicit operation, separate from release publication.
	run("catalog", "add", "local", repositoryRef+":0.1.0")
	require.Contains(t, run("search", "oidc", "--registry=local", "--capability=auth.provider", "--runtime=native"), "example/oidc")
	catalogPath := filepath.Join(config.BasePath, "catalog.yaml")
	run("catalog", "build", layout, "--source", repositoryRef, "--output", catalogPath, "--name=corporate")
	raw, err := os.ReadFile(catalogPath)
	require.NoError(t, err)
	built, err := catalog.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "corporate", built.Metadata.Name)
	_, err = invokePluginCoverage(t, config, "catalog", "publish", catalogPath, "--to=local")
	require.ErrorIs(t, err, errUtils.ErrPluginCatalogConflict)
	run("catalog", "publish", catalogPath, "--to="+catalogRef+"-built")
	checkPluginCatalogFailures(t, config, repositoryRef, catalogRef, catalogPath)
	run("install", "local/oidc", "--as=corporate")
	reloadPluginCoverageConfig(t, config)
	require.Equal(t, repositoryRef, config.Plugins.Packages["corporate"].Source)
	require.NoError(t, pluginconfig.Validate(&config.Plugins))
	locked, err := pluginlock.Load(config.BasePath)
	require.NoError(t, err)
	require.Equal(t, "example/oidc", locked.Spec.Packages["corporate"].Plugin)
	require.Equal(t, repositoryRef, locked.Spec.Packages["corporate"].Source)
	require.Contains(t, run("info", "corporate"), "example/oidc")
	run("install", "--frozen")
	run("update", "corporate", "--version=>=0.1.0")
	reloadPluginCoverageConfig(t, config)
	run("update", "corporate")
	run("update")
	run("catalog", "remove", "local", "example/oidc", "--version=0.1.0")
	require.NotContains(t, run("search", "--format=json", "--refresh"), `"version": "0.1.0"`)
	run("catalog", "add", "local", repositoryRef+":0.1.0")
	require.Contains(t, run("search", "--refresh"), "example/oidc")
	run("catalog", "remove", "local", "example/oidc")
	run("remove", "corporate")
	reloadPluginCoverageConfig(t, config)
	require.NotContains(t, config.Plugins.Packages, "corporate")
	run("registry", "remove", "local")
	reloadPluginCoverageConfig(t, config)
	require.NotContains(t, config.Plugins.Registries, "local")
	entry := locked.Spec.Packages["corporate"]
	run("delete", repositoryRef+"@"+entry.IndexDigest, "--yes")
}

func TestPluginCoverageRejectsMissingConfiguration(t *testing.T) {
	_ = NewTestKit(t)
	cases := [][]string{{"install"}, {"list"}, {"info", "missing"}, {"run", "missing"}, {"remove", "missing"}, {"update"}, {"search"}, {"registry", "list"}, {"registry", "add", "example", "oci://example.com/catalog:latest"}, {"registry", "remove", "example"}, {"registry", "logout", "example.com"}, {"registry", "login", "example.com", "--username=user", "--password-stdin"}, {"publish", "missing", "--to=oci://example.com/repo"}, {"catalog", "build", "missing", "--source=oci://example.com/repo", "--output=missing"}, {"catalog", "publish", "missing", "--to=local"}, {"catalog", "add", "local", "invalid"}, {"catalog", "remove", "local", "example/plugin"}}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, err := invokePluginCoverage(t, nil, args...)
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		})
	}
}

// Keep imports and schema references checked at compile time when contracts evolve.
var (
	_ = schema.PluginRegistry{Catalog: ""}
	_ = schema.PluginPackage{Source: ""}
)

func TestPluginPreparationFailureStopsCommandStartup(t *testing.T) {
	config := pluginCoverageProject(t)
	previousPrepare, previousExit := preparePluginRegistry, errUtils.OsExit
	t.Cleanup(func() {
		clearPlugins()
		preparePluginRegistry = previousPrepare
		errUtils.OsExit = previousExit
	})
	prepared := false
	preparePluginRegistry = func(_ context.Context, loaded *schema.AtmosConfiguration, _ ...bootstrap.Option) (func(), error) {
		prepared = true
		require.Equal(t, config.BasePath, loaded.BasePath)
		return nil, errUtils.ErrPluginInvalidConfig
	}
	var exits []int
	errUtils.OsExit = func(code int) { exits = append(exits, code) }

	_, stderr := captureStdoutStderr(t, func() { RootCmd.PersistentPreRun(RootCmd, nil) })
	require.True(t, prepared, "startup must prepare referenced plugins")
	require.Equal(t, []int{1}, exits, "plugin preparation failure must stop startup")
	require.Contains(t, stderr, errUtils.ErrPluginInvalidConfig.Error())
	require.Nil(t, releasePlugins, "failed plugin setup must not register a cleanup callback")
}

func TestPluginPreparationExemptsOnlyTopLevelLifecycleCommands(t *testing.T) {
	_ = NewTestKit(t)
	previous := preparePluginRegistry
	t.Cleanup(func() {
		clearPlugins()
		preparePluginRegistry = previous
	})
	preparePluginRegistry = func(context.Context, *schema.AtmosConfiguration, ...bootstrap.Option) (func(), error) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	tests := []struct {
		path   string
		exempt bool
	}{
		{"plugin", true},
		{"plugin registry list", true},
		{"version", true},
		{"packer version", false},
		{"helm plugin", false},
		{"deploy version", false},
		{"deploy plugin list", false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			command := &cobra.Command{Use: "atmos"}
			for _, name := range strings.Fields(test.path) {
				child := &cobra.Command{Use: name}
				command.AddCommand(child)
				command = child
			}
			err := preparePlugins(command, &schema.AtmosConfiguration{})
			if test.exempt {
				require.NoError(t, err, "lifecycle commands must remain usable with broken plugins")
			} else {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, "nested command names must not bypass plugin preparation")
			}
		})
	}
}
