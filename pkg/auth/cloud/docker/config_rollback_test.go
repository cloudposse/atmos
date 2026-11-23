package docker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// breakConfigSave replaces config.json with a non-empty directory so the atomic
// rename in saveConfig fails on every platform, after loadConfig has run.
func breakConfigSave(t *testing.T, m *ConfigManager) {
	t.Helper()
	require.NoError(t, os.Remove(m.GetConfigPath()))
	require.NoError(t, os.MkdirAll(filepath.Join(m.GetConfigPath(), "child"), 0o700))
}

// TestConfigManager_WriteAuth_SaveFailureErasesHelperCredential verifies that
// when the helper store succeeds but config.json cannot be saved, the
// just-stored credential is erased from the helper. Without the auths
// placeholder, RemoveAuth would never erase it, orphaning it in the keychain.
func TestConfigManager_WriteAuth_SaveFailureErasesHelperCredential(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop"}`)
	helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).
		DoAndReturn(func(_, _, _, _ string) error {
			breakConfigSave(t, manager)
			return nil
		})
	helper.EXPECT().Erase("desktop", testECRRegistry).Return(nil)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.NotErrorIs(t, err, errUtils.ErrDockerCredentialHelper, "a successful rollback must not report a helper error")
}

// TestConfigManager_WriteAuth_SaveFailureRollbackFailure verifies that a failed
// rollback is reported together with the original save error.
func TestConfigManager_WriteAuth_SaveFailureRollbackFailure(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop"}`)
	helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).
		DoAndReturn(func(_, _, _, _ string) error {
			breakConfigSave(t, manager)
			return nil
		})
	helper.EXPECT().Erase("desktop", testECRRegistry).Return(errFakeHelper)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.ErrorIs(t, err, errFakeHelper)
}

// TestConfigManager_WriteAuth_SaveFailureRestoresPreviousHelperCredential
// verifies that when the helper already held a credential for the registry,
// a failed config.json save restores it instead of erasing it.
func TestConfigManager_WriteAuth_SaveFailureRestoresPreviousHelperCredential(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop", "auths": {"`+testECRRegistry+`": {}}}`)
	gomock.InOrder(
		helper.EXPECT().Get("desktop", testECRRegistry).Return(&HelperCredential{Username: "old-user", Secret: "old-secret"}, nil),
		helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).
			DoAndReturn(func(_, _, _, _ string) error {
				breakConfigSave(t, manager)
				return nil
			}),
		helper.EXPECT().Store("desktop", testECRRegistry, "old-user", "old-secret").Return(nil),
	)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.NotErrorIs(t, err, errUtils.ErrDockerCredentialHelper, "a successful restore must not report a helper error")
}

// TestConfigManager_WriteAuth_SaveFailureRestoreFailure verifies that a failed
// restore is reported together with the original save error.
func TestConfigManager_WriteAuth_SaveFailureRestoreFailure(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop", "auths": {"`+testECRRegistry+`": {}}}`)
	gomock.InOrder(
		helper.EXPECT().Get("desktop", testECRRegistry).Return(&HelperCredential{Username: "old-user", Secret: "old-secret"}, nil),
		helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).
			DoAndReturn(func(_, _, _, _ string) error {
				breakConfigSave(t, manager)
				return nil
			}),
		helper.EXPECT().Store("desktop", testECRRegistry, "old-user", "old-secret").Return(errFakeHelper),
	)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.ErrorIs(t, err, errFakeHelper)
}

// TestConfigManager_WriteAuth_SaveFailureErasesWhenListedButEmpty verifies that
// a listed registry whose helper holds no credential is erased on rollback.
func TestConfigManager_WriteAuth_SaveFailureErasesWhenListedButEmpty(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop", "auths": {"`+testECRRegistry+`": {}}}`)
	gomock.InOrder(
		helper.EXPECT().Get("desktop", testECRRegistry).Return(nil, nil),
		helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).
			DoAndReturn(func(_, _, _, _ string) error {
				breakConfigSave(t, manager)
				return nil
			}),
		helper.EXPECT().Erase("desktop", testECRRegistry).Return(nil),
	)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
}

// TestConfigManager_WriteAuth_SaveFailureLeavesUnreadablePreviousCredential
// verifies the negative path: when the previous credential cannot be read, the
// rollback neither erases nor overwrites the helper.
func TestConfigManager_WriteAuth_SaveFailureLeavesUnreadablePreviousCredential(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop", "auths": {"`+testECRRegistry+`": {}}}`)
	gomock.InOrder(
		helper.EXPECT().Get("desktop", testECRRegistry).Return(nil, errFakeHelper),
		helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).
			DoAndReturn(func(_, _, _, _ string) error {
				breakConfigSave(t, manager)
				return nil
			}),
	)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	assert.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	assert.NotErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
}

// TestConfigManager_WriteAuth_StoreFailureRestoresPreviousCredential verifies
// that a failed helper Store restores the credential the helper held before.
func TestConfigManager_WriteAuth_StoreFailureRestoresPreviousCredential(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop", "auths": {"`+testECRRegistry+`": {}}}`)
	gomock.InOrder(
		helper.EXPECT().Get("desktop", testECRRegistry).Return(&HelperCredential{Username: "old-user", Secret: "old-secret"}, nil),
		helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).Return(errFakeHelper),
		helper.EXPECT().Store("desktop", testECRRegistry, "old-user", "old-secret").Return(nil),
	)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.ErrorIs(t, err, errFakeHelper)
}

// TestConfigManager_WriteAuth_StoreFailureRestoreFailure verifies that a failed
// restore is joined with the Store error.
func TestConfigManager_WriteAuth_StoreFailureRestoreFailure(t *testing.T) {
	errRestore := errors.New("restore failed")
	manager, helper := newMockedManager(t, `{"credsStore": "desktop", "auths": {"`+testECRRegistry+`": {}}}`)
	gomock.InOrder(
		helper.EXPECT().Get("desktop", testECRRegistry).Return(&HelperCredential{Username: "old-user", Secret: "old-secret"}, nil),
		helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).Return(errFakeHelper),
		helper.EXPECT().Store("desktop", testECRRegistry, "old-user", "old-secret").Return(errRestore),
	)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	assert.ErrorIs(t, err, errFakeHelper)
	assert.ErrorIs(t, err, errRestore)
}

// TestConfigManager_WriteAuth_StoreFailureWithoutPreviousCredential verifies the
// negative path: with no known prior credential the helper is not touched again.
func TestConfigManager_WriteAuth_StoreFailureWithoutPreviousCredential(t *testing.T) {
	manager, helper := newMockedManager(t, `{"credsStore": "desktop"}`)
	helper.EXPECT().Store("desktop", testECRRegistry, testECRUser, testECRPassword).Return(errFakeHelper)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

	assert.ErrorIs(t, err, errFakeHelper)
}

// TestConfigManager_SaveConfig_DanglingSymlinkStaysSymlink verifies that a
// config.json symlink whose target does not exist yet is written through, so
// the link survives and the target is created.
func TestConfigManager_SaveConfig_DanglingSymlinkStaysSymlink(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "real")
	require.NoError(t, os.MkdirAll(targetDir, 0o700))
	target := filepath.Join(targetDir, "docker-config.json")
	configPath := filepath.Join(dir, configFileName)
	if err := os.Symlink(filepath.Join("real", "docker-config.json"), configPath); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	manager := &ConfigManager{configDir: dir, configPath: configPath}
	config := newRawConfig()
	config.setAuth(testECRRegistry, inlineAuthEntry(expectedInlineAuth()))
	require.NoError(t, manager.saveConfig(config))

	info, err := os.Lstat(configPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "config.json must remain a symlink")
	written, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Contains(t, string(written), testECRRegistry)
}

// TestResolveConfigTarget covers the plain file and cycle paths, which are
// returned unchanged.
func TestResolveConfigTarget(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	assert.Equal(t, missing, resolveConfigTarget(missing))

	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	if err := os.Symlink(b, a); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}
	require.NoError(t, os.Symlink(a, b))
	assert.Equal(t, a, resolveConfigTarget(a))
}
