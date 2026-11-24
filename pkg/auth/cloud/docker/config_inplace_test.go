package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
)

// refuseRename makes the manager's atomic config.json replace fail with errno, as the kernel
// does for a single-file bind mount of config.json (EBUSY, devcontainers and Kubernetes subPath
// mounts). Reads, stats and in-place writes still go to the real file system.
func refuseRename(t *testing.T, m *ConfigManager, errno syscall.Errno) {
	t.Helper()
	mock := filesystem.NewMockFileSystem(gomock.NewController(t))
	osFS := filesystem.NewOSFileSystem()
	mock.EXPECT().WriteFileAtomic(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(name string, _ []byte, _ os.FileMode) error {
			return &os.LinkError{Op: "rename", Old: name + ".tmp", New: name, Err: errno}
		}).AnyTimes()
	mock.EXPECT().Stat(gomock.Any()).DoAndReturn(osFS.Stat).AnyTimes()
	mock.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(osFS.WriteFile).AnyTimes()
	m.fs = mock
}

// TestConfigManager_BindMountedConfig_LoginAndLogout reproduces the 1.235.0 upgrade regression:
// with config.json mounted as a single file, the temp file + rename save was refused, so ECR/ACR
// login failed and logout left the credential behind. The save now falls back to writing the
// same bytes in place, as v1.234.1 did.
func TestConfigManager_BindMountedConfig_LoginAndLogout(t *testing.T) {
	manager := writeTestConfig(t, `{"auths": {"ghcr.io": {"auth": "dXNlcjp0b2tlbg=="}}}`, 0o600)
	refuseRename(t, manager, syscall.EBUSY)

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))
	view := decodeTypedConfig(t, readConfigFile(t, manager))
	assert.Equal(t, expectedInlineAuth(), view.Auths[testECRRegistry].Auth, "login must write the token")
	assert.Contains(t, view.Auths, "ghcr.io", "other registries are kept")

	require.NoError(t, manager.RemoveAuth(testECRRegistry))
	view = decodeTypedConfig(t, readConfigFile(t, manager))
	assert.NotContains(t, view.Auths, testECRRegistry, "logout must remove the token")
	assert.Contains(t, view.Auths, "ghcr.io")
}

// TestConfigManager_ReadOnlyDockerDir_WritableConfig covers the second case of the same
// regression on a real file system: the Docker config directory is read-only but config.json
// itself is writable, so the temp file cannot be created and the save falls back to an in-place
// write.
func TestConfigManager_ReadOnlyDockerDir_WritableConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission bits are not enforced the same way on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("permission checks do not apply when running as root")
	}
	manager := writeTestConfig(t, `{"auths": {}}`, 0o600)
	// The lock file must exist before the directory becomes read-only.
	require.NoError(t, manager.withConfigLock(func() error { return nil }))
	dir := manager.GetConfigDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	require.NoError(t, manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword))
	assert.Equal(t, expectedInlineAuth(), decodeTypedConfig(t, readConfigFile(t, manager)).Auths[testECRRegistry].Auth)

	require.NoError(t, manager.RemoveAuth(testECRRegistry))
	assert.NotContains(t, decodeTypedConfig(t, readConfigFile(t, manager)).Auths, testECRRegistry)
}

// TestConfigManager_SaveFailsWhenInPlaceWriteFailsToo is the negative path: when the in-place
// write is refused as well, the save fails with ErrDockerConfigWrite and nothing is reported as
// written.
func TestConfigManager_SaveFailsWhenInPlaceWriteFailsToo(t *testing.T) {
	manager := writeTestConfig(t, `{"auths": {"`+testECRRegistry+`": {"auth": "eDp5"}}}`, 0o600)
	mock := filesystem.NewMockFileSystem(gomock.NewController(t))
	mock.EXPECT().WriteFileAtomic(gomock.Any(), gomock.Any(), gomock.Any()).Return(&os.LinkError{Op: "rename", Err: syscall.EBUSY})
	mock.EXPECT().Stat(gomock.Any()).DoAndReturn(os.Stat)
	mock.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(&os.PathError{Op: "open", Err: syscall.EROFS})
	manager.fs = mock

	err := manager.RemoveAuth(testECRRegistry)
	require.ErrorIs(t, err, errUtils.ErrDockerConfigWrite)
	require.ErrorIs(t, err, syscall.EROFS)
	assert.Contains(t, decodeTypedConfig(t, readConfigFile(t, manager)).Auths, testECRRegistry, "the entry is still there")
}

// TestConfigManager_WriteAuth_EcrLoginHelperIsNotRead reproduces the #3742 follow-up: for a
// registry routed to docker-credential-ecr-login, Atmos must not run `get` before `store`. The
// real ecr-login `get` calls ECR with the ambient AWS credentials, which is slow, may use the wrong
// account, or fails before the login can proceed. The `store` still runs, and its "not
// implemented" reply leaves the registry to the helper.
func TestConfigManager_WriteAuth_EcrLoginHelperIsNotRead(t *testing.T) {
	for name, content := range map[string]string{
		"credHelpers": `{"auths": {"` + testECRRegistry + `": {}}, "credHelpers": {"` + testECRRegistry + `": "ecr-login"}}`,
		"credsStore":  `{"auths": {"` + testECRRegistry + `": {}}, "credsStore": "ecr-login"}`,
	} {
		t.Run(name, func(t *testing.T) {
			manager, helper := newMockedManager(t, content)
			// No Get is expected; gomock fails the test on an unexpected call.
			helper.EXPECT().Store("ecr-login", testECRRegistry, testECRUser, testECRPassword).
				Return(fmt.Errorf("%w: not implemented", errUtils.ErrDockerCredentialHelperStoreUnsupported))

			err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)

			_, managed := HelperManagedNotice(err)
			assert.True(t, managed, "the registry is left to ecr-login")
			assert.Equal(t, content, string(readConfigFile(t, manager)), "config.json is untouched")
		})
	}
}

// TestConfigManager_WriteAuth_EcrLoginStoreFailureLeavesHelperAlone verifies that when the
// ecr-login store fails for another reason, no previous credential was read, so nothing is
// restored or erased.
func TestConfigManager_WriteAuth_EcrLoginStoreFailureLeavesHelperAlone(t *testing.T) {
	content := `{"auths": {"` + testECRRegistry + `": {}}, "credsStore": "ecr-login"}`
	manager, helper := newMockedManager(t, content)
	helper.EXPECT().Store("ecr-login", testECRRegistry, testECRUser, testECRPassword).Return(errFakeHelper)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)
	require.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.Equal(t, content, string(readConfigFile(t, manager)))
}

// TestConfigManager_WriteAuth_OtherHelpersAreStillRead is the negative path: a helper that stores
// credentials (here osxkeychain) is still read first, so a failed store can restore the previous
// credential.
func TestConfigManager_WriteAuth_OtherHelpersAreStillRead(t *testing.T) {
	content := `{"auths": {"` + testECRRegistry + `": {}}, "credsStore": "osxkeychain"}`
	manager, helper := newMockedManager(t, content)
	gomock.InOrder(
		helper.EXPECT().Get("osxkeychain", testECRRegistry).Return(&HelperCredential{Username: "old", Secret: "old-secret"}, nil),
		helper.EXPECT().Store("osxkeychain", testECRRegistry, testECRUser, testECRPassword).Return(errFakeHelper),
		helper.EXPECT().Store("osxkeychain", testECRRegistry, "old", "old-secret").Return(nil),
	)

	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)
	require.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
}

// decodeTypedConfig decodes config.json into the typed auths view.
func decodeTypedConfig(t *testing.T, data []byte) dockerConfig {
	t.Helper()
	var view dockerConfig
	require.NoError(t, json.Unmarshal(data, &view))
	return view
}
