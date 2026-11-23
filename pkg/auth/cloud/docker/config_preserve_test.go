package docker

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testECRRegistry = "123456789012.dkr.ecr.us-east-1.amazonaws.com"
	testECRUser     = "AWS"
	testECRPassword = "ecr-token"
)

// richDockerConfig is a realistic ~/.docker/config.json with every field the
// old auths-only model dropped (issue #3720, finding 2.2).
const richDockerConfig = `{
  "auths": {
    "https://index.docker.io/v1/": {
      "auth": "dXNlcjpwYXNz",
      "email": "dev@example.com"
    },
    "myregistry.azurecr.io": {
      "identitytoken": "refresh-token-value"
    }
  },
  "credHelpers": {
    "gcr.io": "gcloud"
  },
  "currentContext": "desktop-linux",
  "plugins": {
    "debug": {
      "hooks": "exec"
    }
  },
  "proxies": {
    "default": {
      "httpProxy": "http://proxy.example.com:3128",
      "noProxy": "localhost,127.0.0.1"
    }
  },
  "HttpHeaders": {
    "X-Meta": "a&b<c>"
  },
  "x-unknown-key": {
    "nested": [1, 2.50, "text"]
  }
}`

// writeTestConfig writes content as the Docker config.json in an isolated
// DOCKER_CONFIG directory and returns a manager for it.
func writeTestConfig(t *testing.T, content string, perm os.FileMode) *ConfigManager {
	t.Helper()
	dir := setupTestDockerConfigDir(t)
	if content != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, configFileName), []byte(content), perm))
	}
	manager, err := NewConfigManager()
	require.NoError(t, err)
	return manager
}

// decodeGeneric decodes JSON into a generic value for semantic comparison.
func decodeGeneric(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var v map[string]any
	require.NoError(t, json.Unmarshal(data, &v))
	return v
}

func readConfigFile(t *testing.T, m *ConfigManager) []byte {
	t.Helper()
	data, err := os.ReadFile(m.GetConfigPath())
	require.NoError(t, err)
	return data
}

func expectedInlineAuth() string {
	return base64.StdEncoding.EncodeToString([]byte(testECRUser + ":" + testECRPassword))
}

// TestConfigManager_WriteRemove_PreservesUnknownFields reproduces #3720 2.2:
// WriteAuth and RemoveAuth must not drop credHelpers, currentContext, plugins,
// proxies, per-registry identitytoken/email, or unknown keys.
func TestConfigManager_WriteRemove_PreservesUnknownFields(t *testing.T) {
	manager := writeTestConfig(t, richDockerConfig, 0o600)
	original := decodeGeneric(t, []byte(richDockerConfig))

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	afterWrite := decodeGeneric(t, readConfigFile(t, manager))
	auths, ok := afterWrite["auths"].(map[string]any)
	require.True(t, ok, "auths must be an object")
	assert.Equal(t, map[string]any{"auth": expectedInlineAuth()}, auths[testECRRegistry])

	// Everything except the new auths entry must be untouched.
	delete(auths, testECRRegistry)
	assert.Equal(t, original, afterWrite)

	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	afterRemove := readConfigFile(t, manager)
	assert.Equal(t, original, decodeGeneric(t, afterRemove))
	// Raw JSON values are preserved verbatim: no number re-formatting, no HTML escaping.
	assert.Contains(t, string(afterRemove), "2.50")
	assert.Contains(t, string(afterRemove), `"a&b<c>"`)
}

// TestConfigManager_WriteRemove_NoAuthsKey verifies a round trip on a config
// without an auths key. The only residue is an empty auths object (the shape
// docker/cli itself always writes), which Docker reads as "no inline auths".
func TestConfigManager_WriteRemove_NoAuthsKey(t *testing.T) {
	content := `{"currentContext": "default"}`
	manager := writeTestConfig(t, content, 0o600)

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))
	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	assert.Equal(t, map[string]any{
		"currentContext": "default",
		"auths":          map[string]any{},
	}, decodeGeneric(t, readConfigFile(t, manager)))
}

// TestConfigManager_SaveConfig_OmitsAbsentEmptyAuths verifies that saving a
// config that never had an auths key, and still has no entries, does not add one.
func TestConfigManager_SaveConfig_OmitsAbsentEmptyAuths(t *testing.T) {
	content := `{"currentContext": "default"}`
	manager := writeTestConfig(t, content, 0o600)

	config, err := manager.loadConfig()
	require.NoError(t, err)
	require.NoError(t, manager.saveConfig(config))

	assert.Equal(t, decodeGeneric(t, []byte(content)), decodeGeneric(t, readConfigFile(t, manager)))
}

// TestConfigManager_RemoveAuth_NoFileDoesNotCreate verifies that removing an
// entry when no config.json exists does not create one.
func TestConfigManager_RemoveAuth_NoFileDoesNotCreate(t *testing.T) {
	manager := writeTestConfig(t, "", 0o600)

	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	_, err := os.Stat(manager.GetConfigPath())
	assert.True(t, os.IsNotExist(err), "RemoveAuth must not create config.json")
}

// TestConfigManager_LoadConfig_WhitespaceFile verifies that an empty or
// whitespace-only config.json is treated as an empty config, as docker/cli does.
func TestConfigManager_LoadConfig_WhitespaceFile(t *testing.T) {
	manager := writeTestConfig(t, " \n", 0o600)

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	registries, err := manager.GetAuthenticatedRegistries()
	require.NoError(t, err)
	assert.Equal(t, []string{testECRRegistry}, registries)
}

// TestConfigManager_WriteAuth_PreservesFileMode verifies that an existing
// config.json keeps its permission bits and a new one is created 0600.
func TestConfigManager_WriteAuth_PreservesFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("POSIX permission bits are not meaningful on Windows")
	}

	t.Run("existing file keeps mode", func(t *testing.T) {
		manager := writeTestConfig(t, `{}`, 0o640)
		require.NoError(t, os.Chmod(manager.GetConfigPath(), 0o640))

		require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

		info, err := os.Stat(manager.GetConfigPath())
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	})

	t.Run("new file is 0600", func(t *testing.T) {
		manager := writeTestConfig(t, "", 0o600)

		require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

		info, err := os.Stat(manager.GetConfigPath())
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(dockerConfigFilePerm), info.Mode().Perm())
	})
}

// TestConfigManager_WriteAuth_FollowsSymlink verifies that a symlinked
// config.json (e.g. managed by a dotfiles repo) stays a symlink and the
// target file receives the update.
func TestConfigManager_WriteAuth_FollowsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("creating symlinks on Windows requires elevated privileges")
	}

	dir := setupTestDockerConfigDir(t)
	target := filepath.Join(t.TempDir(), "real-config.json")
	require.NoError(t, os.WriteFile(target, []byte(`{"currentContext":"default"}`), 0o600))
	link := filepath.Join(dir, configFileName)
	require.NoError(t, os.Symlink(target, link))

	manager, err := NewConfigManager()
	require.NoError(t, err)
	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "config.json must remain a symlink")

	data, err := os.ReadFile(target)
	require.NoError(t, err)
	got := decodeGeneric(t, data)
	assert.Equal(t, "default", got["currentContext"])
	assert.Contains(t, got["auths"], testECRRegistry)
}

// TestConfigManager_CredsStore_RoutesThroughHelper reproduces the #3720 2.2
// caveat: with a global credsStore, docker/cli reads credentials from the
// helper and ignores inline auths, so the login must go through the helper,
// and credsStore itself must be preserved.
func TestConfigManager_CredsStore_RoutesThroughHelper(t *testing.T) {
	callsDir := installFakeCredentialHelper(t, "ok")
	content := `{
  "auths": {"other.example.com": {"auth": "b3RoZXI6b3RoZXI="}},
  "credsStore": "` + fakeHelperName + `",
  "credHelpers": {"gcr.io": "gcloud"},
  "currentContext": "desktop-linux"
}`
	manager := writeTestConfig(t, content, 0o600)
	original := decodeGeneric(t, []byte(content))

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	calls := readFakeHelperCalls(t, callsDir)
	require.Len(t, calls, 1)
	assert.Equal(t, "store", calls[0].Action)
	var stored map[string]string
	require.NoError(t, json.Unmarshal([]byte(calls[0].Input), &stored))
	assert.Equal(t, map[string]string{
		"ServerURL": testECRRegistry,
		"Username":  testECRUser,
		"Secret":    testECRPassword,
	}, stored)

	afterWrite := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, fakeHelperName, afterWrite["credsStore"], "credsStore must be preserved")
	auths, ok := afterWrite["auths"].(map[string]any)
	require.True(t, ok)
	// Like docker login, only an empty placeholder lands in auths: no inline secret.
	assert.Equal(t, map[string]any{}, auths[testECRRegistry])
	assert.NotContains(t, string(readConfigFile(t, manager)), expectedInlineAuth())

	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	calls = readFakeHelperCalls(t, callsDir)
	require.Len(t, calls, 2)
	assert.Equal(t, "erase", calls[1].Action)
	assert.Equal(t, testECRRegistry, calls[1].Input)
	assert.Equal(t, original, decodeGeneric(t, readConfigFile(t, manager)))
}

// TestConfigManager_CredHelperEmpty_UsesInlineAuth verifies docker/cli's rule
// that an empty per-registry credHelpers entry routes that registry to the
// file store (inline auths) even when a global credsStore is set.
func TestConfigManager_CredHelperEmpty_UsesInlineAuth(t *testing.T) {
	callsDir := installFakeCredentialHelper(t, "ok")
	content := `{"credsStore": "` + fakeHelperName + `", "credHelpers": {"` + testECRRegistry + `": ""}}`
	manager := writeTestConfig(t, content, 0o600)

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	assert.Empty(t, readFakeHelperCalls(t, callsDir), "helper must not be called")
	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, fakeHelperName, got["credsStore"])
	assert.Equal(t, map[string]any{testECRRegistry: ""}, got["credHelpers"])
	assert.Equal(t, map[string]any{testECRRegistry: map[string]any{"auth": expectedInlineAuth()}}, got["auths"])
}
