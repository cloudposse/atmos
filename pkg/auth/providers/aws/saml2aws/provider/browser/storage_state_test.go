package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// writeStateLikePlaywright writes the state file the way Playwright does (world-readable).
func writeStateLikePlaywright(t *testing.T) func(...playwright.BrowserContextStorageStateOptions) (*playwright.StorageState, error) {
	t.Helper()
	return func(opts ...playwright.BrowserContextStorageStateOptions) (*playwright.StorageState, error) {
		require.Len(t, opts, 1)
		require.NotNil(t, opts[0].Path)
		require.NoError(t, os.WriteFile(*opts[0].Path, []byte(`{"cookies":[]}`), 0o644))
		return &playwright.StorageState{}, nil
	}
}

// The storage state holds IdP session cookies, so it must not stay world-readable.
func TestSaveStorageStateRestrictsPermissions(t *testing.T) {
	ctrl := gomock.NewController(t)
	saver := NewMockstorageStateSaver(ctrl)
	statePath := filepath.Join(t.TempDir(), ".aws", "saml2aws", "storageState.json")

	saver.EXPECT().StorageState(gomock.Any()).DoAndReturn(writeStateLikePlaywright(t))

	require.NoError(t, saveStorageState(saver, statePath))

	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	assert.JSONEq(t, `{"cookies":[]}`, string(data))

	if runtime.GOOS == "windows" {
		return // Unix permission bits are not meaningful on Windows.
	}
	fileInfo, err := os.Stat(statePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(storageStateFilePerm), fileInfo.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Dir(statePath))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(storageStateDirPerm), dirInfo.Mode().Perm())
}

func TestSaveStorageStateReturnsSaveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	saver := NewMockstorageStateSaver(ctrl)
	statePath := filepath.Join(t.TempDir(), "storageState.json")

	saver.EXPECT().StorageState(gomock.Any()).Return(nil, errTest)

	require.ErrorIs(t, saveStorageState(saver, statePath), errTest)
	_, err := os.Stat(statePath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestSaveStorageStateReturnsMkdirError(t *testing.T) {
	ctrl := gomock.NewController(t)
	saver := NewMockstorageStateSaver(ctrl) // No StorageState call is expected.

	// A regular file where the parent directory should be makes MkdirAll fail on every OS.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	require.Error(t, saveStorageState(saver, filepath.Join(blocker, "storageState.json")))
}
