package cmd

import (
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

// verifyAutomaticPluginConsumer uses a clean consumer with declarations only;
// the first auth command must fetch the generated plugin before creating auth.
func verifyAutomaticPluginConsumer(t *testing.T, pluginDirectory, issuer, broker string) {
	t.Helper()
	directory := t.TempDir()
	environment := pluginTestEnvironment(t, directory)
	descriptors, err := bootstrap.Descriptors()
	require.NoError(t, err)
	index, err := pluginoci.BuildIndex(pluginDirectory, descriptors)
	require.NoError(t, err)
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	repository, err := name.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/example/oidc")
	require.NoError(t, err)
	require.NoError(t, atmosoci.NewClient(nil).WriteIndex(t.Context(), repository.Tag("0.1.0"), index))
	config := pluginConsumerConfig(directory, issuer, broker)
	config.Plugins.Packages = map[string]schema.PluginPackage{"corporate": {Source: "oci://" + repository.Name(), Version: "^0.1.0"}}
	raw, err := yaml.Marshal(config)
	require.NoError(t, err)
	configPath := filepath.Join(directory, "atmos.yaml")
	require.NoError(t, os.WriteFile(configPath, raw, 0o600))
	require.Contains(t, runPluginCLI(t, directory, environment, "plugin", "list"), "missing")
	_, err = os.Stat(pluginlock.Path(directory))
	require.ErrorIs(t, err, os.ErrNotExist, "inspection must not install or lock a release")
	runConcurrentPluginLogins(t, directory, environment)
	locked, err := os.ReadFile(pluginlock.Path(directory))
	require.NoError(t, err)
	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, raw, after)
	server.Close()
	// Registry availability is irrelevant after the exact package is cached.
	runPluginCLI(t, directory, environment, "auth", "login", "--identity=broker")
	require.Contains(t, runPluginCLI(t, directory, environment, "plugin", "run", "corporate", "--"), "OIDC PKCE login")
	after, err = os.ReadFile(pluginlock.Path(directory))
	require.NoError(t, err)
	require.Equal(t, locked, after)
	runPluginCLI(t, directory, environment, "auth", "logout", "--provider", "corporate", "--keychain", "--force")
}
