package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
