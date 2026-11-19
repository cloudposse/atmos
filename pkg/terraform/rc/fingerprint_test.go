package rc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestManagedFingerprintProvenance(t *testing.T) {
	for _, key := range userCLIConfigEnvKeys {
		t.Setenv(key, "")
	}
	cfg := &schema.AtmosConfiguration{}
	generated := func(url, identity string) (string, []byte, string, func() error) {
		t.Helper()
		actual := map[string]any{"host": map[string]any{"registry.example.com": map[string]any{"services": map[string]any{"modules.v1": url}}}}
		stable := map[string]any{"host": map[string]any{"registry.example.com": map[string]any{"services": map[string]any{"modules.v1": "https://managed.invalid/"}}}}
		env, cleanup, err := GenerateManaged(actual, stable, identity, cfg, &schema.ConfigAndStacksInfo{})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, cleanup()) })
		path := strings.TrimPrefix(env[0], EnvKeyTFCLIConfigFile+"=")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		return path, content, FingerprintContent(path, content), cleanup
	}
	path, a, first, cleanup := generated("https://127.0.0.1:1234/", "root=A;cert=A")
	_, b, second, _ := generated("https://127.0.0.1:5678/", "root=A;cert=A")
	assert.NotEqual(t, a, b)
	assert.Equal(t, first, second)
	_, _, changed, _ := generated("https://127.0.0.1:5678/", "root=B;cert=A")
	assert.NotEqual(t, first, changed)
	_, _, trust, _ := generated("https://127.0.0.1:5678/", "root=A;cert=B")
	assert.NotEqual(t, first, trust)
	edited := append(append([]byte(nil), a...), []byte("\n# user edit\n")...)
	assert.Equal(t, string(edited), FingerprintContent(path, edited), "modified generated content must never use stale identity")
	assert.Equal(t, string(a), FingerprintContent("unregistered-user-file", a), "identical arbitrary user content must not acquire provenance")
	require.NoError(t, cleanup())
	assert.Equal(t, string(a), FingerprintContent(path, a), "cleanup removes provenance as well as the file")
}

func TestManagedFingerprintUserPrecedenceAndErrors(t *testing.T) {
	for _, key := range userCLIConfigEnvKeys {
		t.Setenv(key, "")
	}
	cfg := &schema.AtmosConfiguration{}
	invalid := map[string]any{"provider_installation": "invalid"}
	_, _, err := GenerateManaged(invalid, invalid, "", cfg, nil)
	require.Error(t, err)
	_, _, err = GenerateManaged(invalid, map[string]any{}, "", cfg, nil)
	require.Error(t, err)
	t.Setenv(EnvKeyTFCLIConfigFile, "user-config")
	env, cleanup, err := GenerateManaged(invalid, invalid, "", cfg, nil)
	require.NoError(t, err)
	assert.Empty(t, env)
	require.NoError(t, cleanup())
}

func TestManagedFingerprintSemanticRCChanges(t *testing.T) {
	for _, key := range userCLIConfigEnvKeys {
		t.Setenv(key, "")
	}
	cfg := &schema.AtmosConfiguration{}
	hash := func(config map[string]any) string {
		t.Helper()
		env, cleanup, err := GenerateManaged(config, config, "same-cache", cfg, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, cleanup()) })
		path := strings.TrimPrefix(env[0], EnvKeyTFCLIConfigFile+"=")
		contents, err := os.ReadFile(path)
		require.NoError(t, err)
		return FingerprintContent(path, contents)
	}
	initial := hash(map[string]any{"plugin_cache_dir": "first-cache"})
	assert.NotEqual(t, initial, hash(map[string]any{"plugin_cache_dir": "second-cache"}))
	assert.NotEqual(t, initial, hash(map[string]any{"credentials": map[string]any{"private.example.com": map[string]any{"token": "dummy-token"}}}))
}

func TestManagedFingerprintConcurrentCleanup(t *testing.T) {
	for _, key := range userCLIConfigEnvKeys {
		t.Setenv(key, "")
	}
	type outcome struct {
		identity string
		clean    bool
		err      error
	}
	results := make(chan outcome, 16)
	for i := range 16 {
		go func() {
			config := map[string]any{"plugin_cache_dir": fmt.Sprintf("ephemeral-%d", i)}
			semantic := map[string]any{"plugin_cache_dir": "stable"}
			env, cleanup, err := GenerateManaged(config, semantic, "identity", &schema.AtmosConfiguration{}, nil)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			path := strings.TrimPrefix(env[0], EnvKeyTFCLIConfigFile+"=")
			content, err := os.ReadFile(path)
			if err != nil {
				_ = cleanup()
				results <- outcome{err: err}
				return
			}
			identity := FingerprintContent(path, content)
			err = cleanup()
			results <- outcome{identity: identity, clean: FingerprintContent(path, content) == string(content), err: err}
		}()
	}
	var first string
	for range 16 {
		result := <-results
		require.NoError(t, result.err)
		assert.True(t, result.clean)
		if first == "" {
			first = result.identity
		}
		assert.Equal(t, first, result.identity)
	}
}

func TestGenerateManagedWithoutSemanticContributionUsesActualContent(t *testing.T) {
	for _, key := range userCLIConfigEnvKeys {
		t.Setenv(key, "")
	}
	config := map[string]any{"plugin_cache_dir": "user-cache"}
	env, cleanup, err := GenerateManaged(config, nil, "unused-identity", &schema.AtmosConfiguration{}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	require.Len(t, env, 2)
	path := strings.TrimPrefix(env[0], EnvKeyTFCLIConfigFile+"=")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), `plugin_cache_dir = "user-cache"`)
	assert.Equal(t, string(content), FingerprintContent(path, content), "ordinary generated user configuration must not receive proxy provenance")
}

func TestGenerateManagedTempFailureDoesNotExportConfiguration(t *testing.T) {
	for _, key := range userCLIConfigEnvKeys {
		t.Setenv(key, "")
	}
	parent := t.TempDir()
	blocked := filepath.Join(parent, "not-a-directory")
	require.NoError(t, os.WriteFile(blocked, []byte("sentinel"), 0o600))
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, blocked)
	}
	config := map[string]any{"plugin_cache_dir": "user-cache"}
	env, cleanup, err := GenerateManaged(config, config, "identity", &schema.AtmosConfiguration{}, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	assert.Empty(t, env)
	require.NoError(t, cleanup())
	content, err := os.ReadFile(blocked)
	require.NoError(t, err)
	assert.Equal(t, "sentinel", string(content))
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "not-a-directory", entries[0].Name())
}
