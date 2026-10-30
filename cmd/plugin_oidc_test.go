package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/api"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/testkit"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
)

func runPluginCommandHelper() int {
	preparePluginRegistry = func(ctx context.Context, config *schema.AtmosConfiguration, options ...bootstrap.Option) (func(), error) {
		browser := api.LocalBrowser{Interactive: true, OpenURL: func(target string) error {
			client := &http.Client{Timeout: 10 * time.Second}
			response, err := client.Get(target)
			if err != nil {
				return err
			}
			defer response.Body.Close()
			_, err = io.Copy(io.Discard, response.Body)
			return err
		}}
		options = append(options, bootstrap.WithInteraction(browser, func() bool { return true }))
		return bootstrap.Prepare(ctx, config, options...)
	}
	if err := Execute(); err != nil {
		ui.Error(err.Error())
		return 1
	}
	return 0
}

func TestPluginOIDCAcrossCLIProcesses(t *testing.T) {
	_ = NewTestKit(t)
	if testing.Short() {
		t.Skip("builds and runs the generated native plugin")
	}
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Dir(filepath.Dir(file))
	directory := t.TempDir()
	pluginDir := filepath.Join(directory, "plugin")
	consumer := filepath.Join(directory, "consumer")
	require.NoError(t, os.MkdirAll(consumer, 0o755))
	environment := pluginTestEnvironment(t, directory)
	run := func(arguments ...string) string { return runPluginCLI(t, consumer, environment, arguments...) }
	run("plugin", "create", pluginDir, "--defaults", "--sdk-path", filepath.Join(root, "sdk"))
	buildOutput := runPluginCLI(t, pluginDir, environment, "build")
	require.Contains(t, buildOutput, "Building plugin")
	require.Contains(t, buildOutput, "Compiling plugin")
	require.Contains(t, buildOutput, "Plugin built successfully")
	require.Contains(t, buildOutput, filepath.Join("bin", "plugin"), "the executable is shown relative to the working directory")
	issuer, err := testkit.NewIssuer()
	require.NoError(t, err)
	t.Cleanup(issuer.Close)
	var exchanges atomic.Int32
	broker := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.True(t, strings.HasPrefix(request.Header.Get("Authorization"), "Bearer "))
		exchanges.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"access_key_id": "pilot-access", "secret_access_key": "pilot-secret", "session_token": "pilot-session", "expiration": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "region": "us-east-1"})
	}))
	t.Cleanup(broker.Close)
	config := pluginConsumerConfig(directory, issuer.Server.URL, broker.URL)
	raw, err := yaml.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(consumer, "atmos.yaml"), raw, 0o600))
	require.Contains(t, run("plugin", "test", pluginDir, "--format=json"), "oidc-lifecycle")
	testOutput := runPluginCLI(t, pluginDir, environment, "test")
	require.Contains(t, testOutput, "isolated fixtures")
	require.Contains(t, testOutput, "Plugin contract")
	require.Contains(t, testOutput, "OIDC lifecycle")
	require.Contains(t, testOutput, "PKCE login")
	require.Contains(t, testOutput, "Command")
	require.Contains(t, testOutput, "3 plugin checks passed")
	require.NotContains(t, testOutput, "manifest-handler-agreement", "machine identifiers must not replace explanations")
	verifyGeneratedPluginPublish(t, pluginDir, environment)
	verifyRegisteredPluginChecks(t, pluginDir)
	// A developer's own profile runs the local build; its name carries no meaning to Atmos.
	profileDirectory := filepath.Join(consumer, ".atmos", "profiles", "local-oidc-build")
	require.NoError(t, os.MkdirAll(profileDirectory, 0o755))
	profile := "plugins:\n  packages:\n    corporate:\n      path: " + filepath.ToSlash(pluginDir) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(profileDirectory, "atmos.yaml"), []byte(profile), 0o600))
	environment = append(environment, "ATMOS_PROFILE=local-oidc-build")
	commandOutput, commandUI := runPluginCLIStreams(t, consumer, environment, "plugin", "run", "corporate", "--")
	require.Contains(t, commandOutput, "OIDC PKCE login and AWS credential exchange")
	require.Contains(t, commandOutput, "No authentication was performed.")
	require.Contains(t, commandOutput, "atmos auth login --identity <identity>")
	for _, text := range []string{"Running", "corporate", "about", "Display the plugin name and supported authentication flow"} {
		require.Contains(t, commandUI, text, "the UI must explain which plugin command is running")
	}
	require.NotContains(t, commandOutput, "Running", "Atmos progress must not pollute plugin stdout")
	require.Equal(t, 1, strings.Count(commandUI, "runs from local path"), "one warning per overridden alias: %s", commandUI)
	require.Contains(t, commandUI, "lock, signature and version checks are bypassed")
	require.NotContains(t, commandOutput, "local path", "the warning belongs on the UI stream")
	login := run("auth", "login", "--identity=broker")
	require.Equal(t, 1, strings.Count(login, "runs from local path"), "an ordinary command that activates the package warns once: %s", login)
	require.NotContains(t, login, "pilot-secret")
	require.NotContains(t, login, "pilot-session")
	run("auth", "login", "--identity=broker")
	authorizations, refreshes, _ := issuer.Counts()
	require.Equal(t, 1, authorizations)
	require.Zero(t, refreshes)
	require.EqualValues(t, 1, exchanges.Load())
	expirePluginSession(t, &config.Auth)
	runConcurrentPluginLogins(t, consumer, environment)
	authorizations, refreshes, _ = issuer.Counts()
	require.Equal(t, 1, authorizations)
	require.Equal(t, 1, refreshes)
	run("auth", "logout", "--provider", "corporate", "--keychain", "--force")
	_, _, revocations := issuer.Counts()
	require.Equal(t, 1, revocations)
	run("auth", "login", "--identity=broker")
	authorizations, _, _ = issuer.Counts()
	require.Equal(t, 2, authorizations, "logout must force a new browser authorization")
	t.Run("automatic release installation", func(t *testing.T) {
		verifyAutomaticPluginConsumer(t, pluginDir, issuer.Server.URL, broker.URL)
	})
}

func pluginTestEnvironment(t *testing.T, directory string) []string {
	t.Helper()
	// Automatic installation of an unlocked package is refused in CI, so subprocesses must not look like CI.
	preserved := telemetry.PreserveCIEnvVars()
	t.Cleanup(func() { telemetry.RestoreCIEnvVars(preserved) })
	t.Setenv("ATMOS_KEYRING_TYPE", "file")
	t.Setenv("ATMOS_XDG_DATA_HOME", filepath.Join(directory, "data"))
	t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(directory, "config"))
	t.Setenv("ATMOS_KEYRING_PASSWORD", "local-test-fixture-password")
	t.Setenv("ATMOS_KEYRING_FILE_PATH", filepath.Join(directory, "keyring"))
	environment := append([]string(nil), os.Environ()...)
	return append(
		environment,
		"_ATMOS_PLUGIN_COMMAND_HELPER=1", "ATMOS_TELEMETRY_ENABLED=false", "ATMOS_VERSION_CHECK_ENABLED=false",
		"ATMOS_EXPERIMENTAL=true", "ATMOS_XDG_DATA_HOME="+filepath.Join(directory, "data"),
		"ATMOS_XDG_CACHE_HOME="+filepath.Join(directory, "cache"), "ATMOS_XDG_CONFIG_HOME="+filepath.Join(directory, "config"),
	)
}

func runPluginCLI(t *testing.T, directory string, environment []string, arguments ...string) string {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir, command.Env = directory, environment
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%v\n%s", arguments, output)
	return string(output)
}

func runPluginCLIStreams(t *testing.T, directory string, environment []string, arguments ...string) (string, string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir, command.Env = directory, environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	require.NoError(t, command.Run(), "%v\n%s\n%s", arguments, stdout.String(), stderr.String())
	return stdout.String(), stderr.String()
}

func pluginConsumerConfig(directory, issuer, broker string) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{BasePath: ".", Plugins: schema.PluginsConfig{CacheDir: filepath.Join(directory, "plugins")}, Auth: schema.AuthConfig{
		Realm: "plugin-cli-pilot", Keyring: schema.KeyringConfig{Type: "file", Spec: map[string]any{"path": filepath.Join(directory, "keyring")}},
		Providers:  map[string]schema.Provider{"corporate": {Kind: "example/oidc", Spec: map[string]any{"issuer": issuer, "client_id": "test-client", "scopes": []string{"openid", "profile"}}}},
		Identities: map[string]schema.Identity{"broker": {Kind: "example/oidc-broker", Via: &schema.IdentityVia{Provider: "corporate"}, Principal: map[string]any{"endpoint": broker}}},
	}}
}

func expirePluginSession(t *testing.T, auth *schema.AuthConfig) {
	t.Helper()
	store, err := credentials.OpenKeyring(auth)
	require.NoError(t, err)
	keys, err := store.List()
	require.NoError(t, err)
	for _, key := range keys {
		if !strings.HasPrefix(key, "atmos_plugin_") {
			require.NoError(t, store.Delete(key))
		}
	}
	require.NoError(t, testkit.ExpireOIDCSession(store))
	files, err := awsCloud.NewAWSFileManager("", auth.Realm)
	require.NoError(t, err)
	require.NoError(t, files.WriteCredentials("corporate", "broker", &types.AWSCredentials{AccessKeyID: "expired", SecretAccessKey: "expired", SessionToken: "expired", Expiration: time.Unix(1, 0).UTC().Format(time.RFC3339)}))
}

// Concurrent CLI processes must reuse the single rotated session under the provider lock.
func runConcurrentPluginLogins(t *testing.T, directory string, environment []string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	var commands []*exec.Cmd
	outputs := make([]bytes.Buffer, 3)
	for index := range outputs {
		command := exec.CommandContext(ctx, executable, "auth", "login", "--identity=broker")
		command.Dir, command.Env = directory, environment
		command.Stdout, command.Stderr = &outputs[index], &outputs[index]
		require.NoError(t, command.Start())
		commands = append(commands, command)
	}
	for index, command := range commands {
		require.NoError(t, command.Wait(), outputs[index].String())
	}
}
