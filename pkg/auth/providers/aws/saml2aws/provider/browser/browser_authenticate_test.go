package browser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

// fakeContext is a browser context that records how it is used. The embedded interface is nil,
// so any method the code under test is not expected to call panics loudly.
type fakeContext struct {
	playwright.BrowserContext
	page      playwright.Page
	pageErr   error
	closed    int
	saved     int
	saveError error
}

func (f *fakeContext) NewPage() (playwright.Page, error) {
	return f.page, f.pageErr
}

func (f *fakeContext) Close(...playwright.BrowserContextCloseOptions) error {
	f.closed++
	return nil
}

func (f *fakeContext) StorageState(opts ...playwright.BrowserContextStorageStateOptions) (*playwright.StorageState, error) {
	f.saved++
	if f.saveError != nil {
		return nil, f.saveError
	}
	if err := os.WriteFile(*opts[0].Path, []byte(`{"cookies":[]}`), 0o644); err != nil {
		return nil, err
	}
	return &playwright.StorageState{}, nil
}

// fakePage is a page handed to the (stubbed) capture function.
type fakePage struct{ playwright.Page }

// stubStorageStatePath points the storage state at a temporary directory.
func stubStorageStatePath(t *testing.T) string {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "saml2aws", "storageState.json")
	original := storageStatePath
	t.Cleanup(func() { storageStatePath = original })
	storageStatePath = func() (string, error) { return statePath, nil }
	return statePath
}

// stubGetSAMLResponse replaces the capture logic and returns a pointer to the page it received.
func stubGetSAMLResponse(t *testing.T, result string, err error) *samlPage {
	t.Helper()
	original := getSAMLResponse
	t.Cleanup(func() { getSAMLResponse = original })
	var got samlPage
	getSAMLResponse = func(page samlPage, _ *creds.LoginDetails, _ *Client) (string, error) {
		got = page
		return result, err
	}
	return &got
}

func TestAuthenticateNewPageFailureClosesEverything(t *testing.T) {
	ctrl := gomock.NewController(t)
	browserType := NewMockBrowserType(ctrl)
	browser := NewMockBrowser(ctrl)
	stops := fakePlaywright(t, browserType)
	statePath := stubStorageStatePath(t)
	bctx := &fakeContext{pageErr: errTest}

	browserType.EXPECT().Launch(gomock.Any()).Return(browser, nil)
	browser.EXPECT().NewContext(gomock.Any()).Return(bctx, nil)
	browser.EXPECT().Close().Return(nil)

	_, err := (&Client{}).Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, 1, *stops)
	assert.Equal(t, 1, bctx.closed, "the context must be closed")
	assert.Zero(t, bctx.saved, "no state is saved when no page was created")
	_, statErr := os.Stat(statePath)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

// The captured assertion is returned and the session state is saved, without a prior state file.
func TestAuthenticateSuccessSavesStorageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	browserType := NewMockBrowserType(ctrl)
	browser := NewMockBrowser(ctrl)
	stops := fakePlaywright(t, browserType)
	statePath := stubStorageStatePath(t)
	page := &fakePage{}
	bctx := &fakeContext{page: page}
	gotPage := stubGetSAMLResponse(t, "assertion", nil)

	browserType.EXPECT().Launch(gomock.Any()).Return(browser, nil)
	browser.EXPECT().NewContext(gomock.Any()).DoAndReturn(func(opts ...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error) {
		require.Len(t, opts, 1)
		assert.Nil(t, opts[0].StorageStatePath, "no state file exists yet")
		return bctx, nil
	})
	browser.EXPECT().Close().Return(nil)

	got, err := (&Client{Headless: true}).Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.NoError(t, err)
	assert.Equal(t, "assertion", got)
	assert.Same(t, page, (*gotPage).(*fakePage))
	assert.Equal(t, 1, *stops)
	assert.Equal(t, 1, bctx.closed)
	assert.Equal(t, 1, bctx.saved)
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	assert.JSONEq(t, `{"cookies":[]}`, string(data))
}

// An existing state file is loaded, and a capture error is still returned after the state is saved.
func TestAuthenticateLoadsExistingStorageState(t *testing.T) {
	ctrl := gomock.NewController(t)
	browserType := NewMockBrowserType(ctrl)
	browser := NewMockBrowser(ctrl)
	fakePlaywright(t, browserType)
	statePath := stubStorageStatePath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o700))
	require.NoError(t, os.WriteFile(statePath, []byte(`{"cookies":[]}`), 0o600))
	bctx := &fakeContext{page: &fakePage{}}
	stubGetSAMLResponse(t, "", ErrNoSAMLResponse)

	browserType.EXPECT().Launch(gomock.Any()).Return(browser, nil)
	browser.EXPECT().NewContext(gomock.Any()).DoAndReturn(func(opts ...playwright.BrowserNewContextOptions) (playwright.BrowserContext, error) {
		require.Len(t, opts, 1)
		require.NotNil(t, opts[0].StorageStatePath)
		assert.Equal(t, statePath, *opts[0].StorageStatePath)
		return bctx, nil
	})
	browser.EXPECT().Close().Return(nil)

	_, err := (&Client{}).Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.ErrorIs(t, err, ErrNoSAMLResponse)
	assert.Equal(t, 1, bctx.saved, "state is saved even when the capture fails")
}

// A failing storage state save is logged, never turned into an authentication failure.
func TestAuthenticateIgnoresStorageStateSaveFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	browserType := NewMockBrowserType(ctrl)
	browser := NewMockBrowser(ctrl)
	fakePlaywright(t, browserType)
	stubStorageStatePath(t)
	bctx := &fakeContext{page: &fakePage{}, saveError: errTest}
	stubGetSAMLResponse(t, "assertion", nil)

	browserType.EXPECT().Launch(gomock.Any()).Return(browser, nil)
	browser.EXPECT().NewContext(gomock.Any()).Return(bctx, nil)
	browser.EXPECT().Close().Return(nil)

	got, err := (&Client{}).Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.NoError(t, err)
	assert.Equal(t, "assertion", got)
	assert.Equal(t, 1, bctx.saved)
}

// The browser type selects the Playwright engine and the launch channel; the executable path is passed through.
func TestAuthenticateBrowserSelectionAndLaunchOptions(t *testing.T) {
	tests := []struct {
		name         string
		client       Client
		wantEngine   string
		wantChannel  string
		wantExecPath string
	}{
		{name: "default is chromium without a channel", client: Client{}, wantEngine: "chromium"},
		{name: "chromium channel", client: Client{BrowserType: "chromium", Headless: true}, wantEngine: "chromium", wantChannel: "chromium"},
		{name: "firefox", client: Client{BrowserType: "firefox"}, wantEngine: "firefox", wantChannel: "firefox"},
		{name: "webkit", client: Client{BrowserType: "webkit"}, wantEngine: "webkit", wantChannel: "webkit"},
		{name: "system chrome uses the chromium engine", client: Client{BrowserType: "chrome"}, wantEngine: "chromium", wantChannel: "chrome"},
		{name: "executable path", client: Client{BrowserExecutablePath: filepath.Join("opt", "browser")}, wantEngine: "chromium", wantExecPath: filepath.Join("opt", "browser")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			engines := map[string]*MockBrowserType{
				"chromium": NewMockBrowserType(ctrl),
				"firefox":  NewMockBrowserType(ctrl),
				"webkit":   NewMockBrowserType(ctrl),
			}
			original, originalRun, originalStop := installPlaywright, runPlaywright, stopPlaywright
			t.Cleanup(func() { installPlaywright, runPlaywright, stopPlaywright = original, originalRun, originalStop })
			runPlaywright = func(...*playwright.RunOptions) (*playwright.Playwright, error) {
				return &playwright.Playwright{Chromium: engines["chromium"], Firefox: engines["firefox"], WebKit: engines["webkit"]}, nil
			}
			stopPlaywright = func(*playwright.Playwright) error { return nil }

			// Only the expected engine may launch; the launch fails to end the flow early.
			engines[tt.wantEngine].EXPECT().Launch(gomock.Any()).DoAndReturn(func(opts ...playwright.BrowserTypeLaunchOptions) (playwright.Browser, error) {
				require.Len(t, opts, 1)
				assert.Equal(t, tt.client.Headless, *opts[0].Headless)
				if tt.wantChannel == "" {
					assert.Nil(t, opts[0].Channel)
				} else {
					require.NotNil(t, opts[0].Channel)
					assert.Equal(t, tt.wantChannel, *opts[0].Channel)
				}
				if tt.wantExecPath == "" {
					assert.Nil(t, opts[0].ExecutablePath)
				} else {
					require.NotNil(t, opts[0].ExecutablePath)
					assert.Equal(t, tt.wantExecPath, *opts[0].ExecutablePath)
				}
				return nil, errTest
			})

			_, err := tt.client.Authenticate(&creds.LoginDetails{URL: testIdPURL})
			require.ErrorIs(t, err, errTest)
		})
	}
}

// A destination that cannot be replaced leaves no temporary file behind.
func TestSaveStorageStateRenameFailureCleansUp(t *testing.T) {
	ctrl := gomock.NewController(t)
	saver := NewMockstorageStateSaver(ctrl)
	dir := t.TempDir()
	statePath := filepath.Join(dir, "storageState.json")
	// A non-empty directory cannot be replaced by a file on any OS.
	require.NoError(t, os.MkdirAll(filepath.Join(statePath, "child"), 0o700))

	saver.EXPECT().StorageState(gomock.Any()).DoAndReturn(writeStateLikePlaywright(t))

	require.Error(t, saveStorageState(saver, statePath))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the destination directory remains")
}
