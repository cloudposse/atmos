package docker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

var errFakeHelper = errors.New("fake helper error")

// newMockedManager returns a manager over content whose credential helper is a gomock mock.
func newMockedManager(t *testing.T, content string) (*ConfigManager, *MockCredentialHelper) {
	t.Helper()
	manager := writeTestConfig(t, content, 0o600)
	helper := NewMockCredentialHelper(gomock.NewController(t))
	manager.helper = helper
	return manager, helper
}

// TestConfigManager_WriteAuth_PerRegistryHelperWins verifies docker/cli
// precedence: credHelpers[registry] wins over the global credsStore.
func TestConfigManager_WriteAuth_PerRegistryHelperWins(t *testing.T) {
	content := `{"credsStore": "desktop", "credHelpers": {"` + testECRRegistry + `": "custom"}}`
	manager, helper := newMockedManager(t, content)
	helper.EXPECT().Store("custom", testECRRegistry, testECRUser, testECRPassword).Return(nil)

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, map[string]any{testECRRegistry: map[string]any{}}, got["auths"])
	assert.Equal(t, "desktop", got["credsStore"])
}

// TestConfigManager_WriteAuth_HelperForOtherRegistryIgnored verifies that a
// credHelpers entry for a different registry does not route this one.
func TestConfigManager_WriteAuth_HelperForOtherRegistryIgnored(t *testing.T) {
	manager, _ := newMockedManager(t, `{"credHelpers": {"gcr.io": "gcloud"}}`)

	// The strict mock fails the test on any helper call.
	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, map[string]any{testECRRegistry: map[string]any{"auth": expectedInlineAuth()}}, got["auths"])
}

// TestConfigManager_WriteAuth_HelperFailureLeavesConfigUnchanged verifies that
// a helper failure is loud (ErrDockerCredentialHelper) and nothing is written.
func TestConfigManager_WriteAuth_HelperFailureLeavesConfigUnchanged(t *testing.T) {
	content := `{"credsStore": "desktop", "auths": {"other.io": {"auth": "eDp5"}}}`
	manager, helper := newMockedManager(t, content)
	helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).Return(errFakeHelper)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.ErrorIs(t, err, errFakeHelper)
	assert.Equal(t, content, string(readConfigFile(t, manager)), "config.json must be untouched")
}

// TestConfigManager_RemoveAuth_HelperErase verifies that RemoveAuth erases the
// helper credential and the placeholder for each registry it was given.
func TestConfigManager_RemoveAuth_HelperErase(t *testing.T) {
	content := `{"credsStore": "desktop", "auths": {"` + testECRRegistry + `": {}, "keep.io": {}}}`
	manager, helper := newMockedManager(t, content)
	helper.EXPECT().Erase("desktop", testECRRegistry).Return(nil)

	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, map[string]any{"keep.io": map[string]any{}}, got["auths"])
	assert.Equal(t, "desktop", got["credsStore"])
}

// TestConfigManager_RemoveAuth_HelperEraseFailureKeepsEntry verifies that a
// failed erase is returned and its entry kept for a retry, while the other
// registries in the same call are still removed.
func TestConfigManager_RemoveAuth_HelperEraseFailureKeepsEntry(t *testing.T) {
	content := `{"credsStore": "desktop", "credHelpers": {"inline.io": ""}, ` +
		`"auths": {"` + testECRRegistry + `": {}, "inline.io": {"auth": "eDp5"}}}`
	manager, helper := newMockedManager(t, content)
	helper.EXPECT().Erase("desktop", testECRRegistry).Return(errFakeHelper)

	err := manager.RemoveAuth(testECRRegistry, "inline.io")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.ErrorIs(t, err, errFakeHelper)
	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Equal(t, map[string]any{testECRRegistry: map[string]any{}}, got["auths"])
}

// TestConfigManager_RemoveAuth_SkipsHelperWithoutEntry verifies that RemoveAuth
// never asks the helper to erase a registry config.json has no entry for, and
// does not rewrite the file when nothing changed.
func TestConfigManager_RemoveAuth_SkipsHelperWithoutEntry(t *testing.T) {
	content := `{"credsStore": "desktop", "auths": {"other.io": {}}}`
	manager, _ := newMockedManager(t, content)

	// The strict mock fails the test on any helper call.
	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	assert.Equal(t, content, string(readConfigFile(t, manager)), "config.json must be untouched")
}

// TestConfigManager_SaveConfig_MarshalFailure verifies that an unencodable
// config is surfaced as ErrDockerConfigWrite and nothing is written.
func TestConfigManager_SaveConfig_MarshalFailure(t *testing.T) {
	content := `{"auths": {"` + testECRRegistry + `": {"auth": "eDp5"}}}`
	manager := writeTestConfig(t, content, 0o600)

	config, err := manager.loadConfig()
	require.NoError(t, err)
	config.setAuth("broken.io", json.RawMessage(`{not json`))

	err = manager.saveConfig(config)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.Equal(t, content, string(readConfigFile(t, manager)))
}

// TestConfigManager_CredentialHelper_DefaultsToExec verifies that a manager
// built without NewConfigManager still gets the PATH-based helper runner.
func TestConfigManager_CredentialHelper_DefaultsToExec(t *testing.T) {
	manager := &ConfigManager{}
	_, ok := manager.credentialHelper().(*execCredentialHelper)
	assert.True(t, ok)

	// Isolate from the user's Docker directory: the constructor creates it.
	setupTestDockerConfigDir(t)
	created, err := NewConfigManager()
	require.NoError(t, err)
	_, ok = created.credentialHelper().(*execCredentialHelper)
	assert.True(t, ok)
}

// TestWithHelperHint verifies the sentinel is added exactly when missing.
func TestWithHelperHint(t *testing.T) {
	plain := withHelperHint(errFakeHelper, "desktop")
	assert.ErrorIs(t, plain, errUtils.ErrDockerCredentialHelper)
	assert.ErrorIs(t, plain, errFakeHelper)

	already := errors.Join(errUtils.ErrDockerCredentialHelper, errFakeHelper)
	wrapped := withHelperHint(already, "desktop")
	assert.ErrorIs(t, wrapped, errUtils.ErrDockerCredentialHelper)
	assert.ErrorIs(t, wrapped, errFakeHelper)
}

// TestExecCredentialHelper runs the default runner against the fake helper
// (the test binary itself), never a real docker-credential-* program.
func TestExecCredentialHelper(t *testing.T) {
	t.Run("store sends protocol payload", func(t *testing.T) {
		callsDir := installFakeCredentialHelper(t, "ok")

		require.NoError(t, newExecCredentialHelper().Store(fakeHelperName, testECRRegistry, testECRUser, testECRPassword))

		calls := readFakeHelperCalls(t, callsDir)
		require.Len(t, calls, 1)
		assert.Equal(t, helperActionStore, calls[0].Action)
		var payload helperCredentials
		require.NoError(t, json.Unmarshal([]byte(calls[0].Input), &payload))
		assert.Equal(t, helperCredentials{ServerURL: testECRRegistry, Username: testECRUser, Secret: testECRPassword}, payload)
	})

	t.Run("store failure", func(t *testing.T) {
		installFakeCredentialHelper(t, "fail")

		err := newExecCredentialHelper().Store(fakeHelperName, testECRRegistry, testECRUser, testECRPassword)
		assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	})

	t.Run("get returns the stored credential", func(t *testing.T) {
		callsDir := installFakeCredentialHelper(t, "ok")

		user, secret, found, err := newExecCredentialHelper().Get(fakeHelperName, testECRRegistry)

		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, fakeHelperPrevUser, user)
		assert.Equal(t, fakeHelperPrevSecret, secret)
		calls := readFakeHelperCalls(t, callsDir)
		require.Len(t, calls, 1)
		assert.Equal(t, fakeHelperCall{Action: helperActionGet, Input: testECRRegistry}, calls[0])
	})

	t.Run("get of unknown credential is not found", func(t *testing.T) {
		installFakeCredentialHelper(t, "notfound")

		_, _, found, err := newExecCredentialHelper().Get(fakeHelperName, testECRRegistry)

		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("get from a helper that cannot store is not found", func(t *testing.T) {
		installFakeCredentialHelper(t, "notimplemented")

		_, _, found, err := newExecCredentialHelper().Get(fakeHelperName, testECRRegistry)

		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("get failure", func(t *testing.T) {
		installFakeCredentialHelper(t, "fail")

		_, _, _, err := newExecCredentialHelper().Get(fakeHelperName, testECRRegistry)

		assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	})

	t.Run("erase sends server URL", func(t *testing.T) {
		callsDir := installFakeCredentialHelper(t, "ok")

		require.NoError(t, newExecCredentialHelper().Erase(fakeHelperName, testECRRegistry))

		calls := readFakeHelperCalls(t, callsDir)
		require.Len(t, calls, 1)
		assert.Equal(t, fakeHelperCall{Action: helperActionErase, Input: testECRRegistry}, calls[0])
	})

	t.Run("erase of unknown credential is a no-op", func(t *testing.T) {
		installFakeCredentialHelper(t, "notfound")

		assert.NoError(t, newExecCredentialHelper().Erase(fakeHelperName, testECRRegistry))
	})

	t.Run("erase failure", func(t *testing.T) {
		installFakeCredentialHelper(t, "fail")

		err := newExecCredentialHelper().Erase(fakeHelperName, testECRRegistry)
		assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	})

	t.Run("missing helper", func(t *testing.T) {
		installFakeCredentialHelper(t, "ok")

		err := newExecCredentialHelper().Store("not-installed", testECRRegistry, testECRUser, testECRPassword)
		assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	})

	t.Run("helper name with path separator is refused", func(t *testing.T) {
		callsDir := installFakeCredentialHelper(t, "ok")

		for _, name := range []string{"../" + fakeHelperName, `..\` + fakeHelperName} {
			err := newExecCredentialHelper().Store(name, testECRRegistry, testECRUser, testECRPassword)
			assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
		}
		assert.Empty(t, readFakeHelperCalls(t, callsDir))
	})

	t.Run("hung helper times out", func(t *testing.T) {
		installFakeCredentialHelper(t, "hang")
		runner := newExecCredentialHelper()
		runner.timeout = 200 * time.Millisecond

		start := time.Now()
		err := runner.Store(fakeHelperName, testECRRegistry, testECRUser, testECRPassword)

		assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, time.Since(start), 20*time.Second)
	})
}

// TestConfigManager_FakeHelperFailure_EndToEnd verifies the full WriteAuth path
// against a failing helper on PATH: loud error, untouched config.json.
func TestConfigManager_FakeHelperFailure_EndToEnd(t *testing.T) {
	installFakeCredentialHelper(t, "fail")
	content := `{"credsStore": "` + fakeHelperName + `"}`
	manager := writeTestConfig(t, content, 0o600)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.Equal(t, content, string(readConfigFile(t, manager)))
}

// TestParseRawConfig covers decoding edge cases of the lossless config view.
func TestParseRawConfig(t *testing.T) {
	invalid := []struct {
		name    string
		content string
	}{
		{"auths not an object", `{"auths": []}`},
		{"auths a string", `{"auths": "x"}`},
		{"credsStore not a string", `{"credsStore": 1}`},
		{"credHelpers not a map of strings", `{"credHelpers": {"a": 1}}`},
		{"top level not an object", `[]`},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRawConfig([]byte(tc.content))
			assert.ErrorIs(t, err, errUtils.ErrDockerConfigRead)
		})
	}

	t.Run("null document", func(t *testing.T) {
		cfg, err := parseRawConfig([]byte("null"))
		require.NoError(t, err)
		assert.Empty(t, cfg.registries())
		data, err := cfg.marshal()
		require.NoError(t, err)
		assert.JSONEq(t, `{}`, string(data))
	})

	t.Run("null values are unset", func(t *testing.T) {
		cfg, err := parseRawConfig([]byte(`{"auths": null, "credsStore": null, "credHelpers": null}`))
		require.NoError(t, err)
		assert.Empty(t, cfg.credentialHelperFor(testECRRegistry))
		assert.True(t, cfg.hadAuths)
	})

	t.Run("registries are sorted", func(t *testing.T) {
		cfg, err := parseRawConfig([]byte(`{"auths": {"b.io": {}, "a.io": {}, "c.io": {}}}`))
		require.NoError(t, err)
		assert.Equal(t, []string{"a.io", "b.io", "c.io"}, cfg.registries())
	})
}

// TestConfigManager_SaveConfig_RenameTargetIsDirectory verifies that an atomic
// write failure is surfaced as ErrDockerConfigWrite.
func TestConfigManager_SaveConfig_RenameTargetIsDirectory(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, configFileName)
	require.NoError(t, os.MkdirAll(filepath.Join(configPath, "child"), 0o700))

	manager := &ConfigManager{configDir: dir, configPath: configPath}
	err := manager.saveConfig(newRawConfig())

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
}

// TestConfigManager_RemoveAuth_SaveFailureIsReturned verifies that a write
// failure after a removal is surfaced as ErrDockerConfigWrite.
func TestConfigManager_RemoveAuth_SaveFailureIsReturned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("directory write-permission bits are not enforced the same way on Windows")
	}
	if os.Getuid() == 0 {
		t.Skipf("permission checks do not apply when running as root")
	}

	manager := writeTestConfig(t, `{"auths": {"`+testECRRegistry+`": {"auth": "eDp5"}}}`, 0o600)
	// Create the lock file first, then make the directory read-only so only the
	// atomic rename of the new config.json fails.
	require.NoError(t, manager.withConfigLock(func() error { return nil }))
	dir := manager.GetConfigDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := manager.RemoveAuth(testECRRegistry)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
}
