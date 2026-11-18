package browser

import (
	"os"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

// fakePlaywright replaces the Playwright entry points with a driver whose Chromium is browserType.
// It returns a pointer to the number of times the driver was stopped.
func fakePlaywright(t *testing.T, browserType playwright.BrowserType) *int {
	t.Helper()
	originalInstall, originalRun, originalStop := installPlaywright, runPlaywright, stopPlaywright
	t.Cleanup(func() {
		installPlaywright, runPlaywright, stopPlaywright = originalInstall, originalRun, originalStop
	})

	stops := 0
	installPlaywright = func(...*playwright.RunOptions) error {
		t.Fatal("install must not run when DownloadBrowser is false")
		return nil
	}
	runPlaywright = func(...*playwright.RunOptions) (*playwright.Playwright, error) {
		return &playwright.Playwright{Chromium: browserType}, nil
	}
	stopPlaywright = func(*playwright.Playwright) error {
		stops++
		return nil
	}
	return &stops
}

// Regression: upstream stopped the Playwright driver only in a defer registered after the
// page was created, so a failed browser launch left the driver process running.
func TestAuthenticateStopsPlaywrightWhenLaunchFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	browserType := NewMockBrowserType(ctrl)
	stops := fakePlaywright(t, browserType)

	browserType.EXPECT().Launch(gomock.Any()).Return(nil, errTest)

	_, err := (&Client{}).Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, 1, *stops, "the Playwright driver must be stopped")
}

// Regression: a failed context creation leaked both the browser and the driver.
func TestAuthenticateClosesBrowserWhenNewContextFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	browserType := NewMockBrowserType(ctrl)
	browser := NewMockBrowser(ctrl)
	stops := fakePlaywright(t, browserType)

	browserType.EXPECT().Launch(gomock.Any()).Return(browser, nil)
	browser.EXPECT().NewContext(gomock.Any()).Return(nil, errTest)
	browser.EXPECT().Close().Return(nil)

	_, err := (&Client{}).Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, 1, *stops, "the Playwright driver must be stopped")
}

// Regression: a storage-state path failure leaked both the browser and the driver.
func TestAuthenticateClosesBrowserWhenStorageStatePathFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	browserType := NewMockBrowserType(ctrl)
	browser := NewMockBrowser(ctrl)
	stops := fakePlaywright(t, browserType)

	original := storageStatePath
	t.Cleanup(func() { storageStatePath = original })
	storageStatePath = func() (string, error) { return "", errTest }

	browserType.EXPECT().Launch(gomock.Any()).Return(browser, nil)
	browser.EXPECT().Close().Return(nil)

	_, err := (&Client{}).Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.ErrorIs(t, err, errTest)
	assert.Equal(t, 1, *stops, "the Playwright driver must be stopped")
}

// A failed install never starts the driver.
func TestAuthenticateInstallFailureDoesNotStartPlaywright(t *testing.T) {
	stops := fakePlaywright(t, nil)
	installPlaywright = func(...*playwright.RunOptions) error { return errTest }
	runPlaywright = func(...*playwright.RunOptions) (*playwright.Playwright, error) {
		t.Fatal("Playwright must not start after a failed install")
		return nil, nil
	}

	_, err := (&Client{}).Authenticate(&creds.LoginDetails{URL: testIdPURL, DownloadBrowser: true})
	require.ErrorIs(t, err, errTest)
	assert.Zero(t, *stops)
}

// Install and Run receive the same options built by runOptions.
func TestAuthenticatePassesRunOptionsToInstallAndRun(t *testing.T) {
	stops := fakePlaywright(t, nil)
	var installed, ran *playwright.RunOptions
	installPlaywright = func(opts ...*playwright.RunOptions) error {
		installed = opts[0]
		return nil
	}
	runPlaywright = func(opts ...*playwright.RunOptions) (*playwright.Playwright, error) {
		ran = opts[0]
		return nil, errTest
	}

	client := &Client{BrowserDriverDir: t.TempDir()}
	_, err := client.Authenticate(&creds.LoginDetails{URL: testIdPURL, DownloadBrowser: true})
	require.ErrorIs(t, err, errTest)
	require.NotNil(t, installed)
	assert.Same(t, installed, ran)
	assert.Equal(t, client.BrowserDriverDir, installed.DriverDirectory)
	assert.Zero(t, *stops, "a driver that failed to start has nothing to stop")
}

// Regression: upstream installed with zero-value options, so the Playwright CLI wrote its
// download progress to stdout and downloaded every browser although only one is launched.
func TestRunOptions(t *testing.T) {
	tests := []struct {
		name        string
		client      Client
		wantBrowser []string
		wantNoShell bool
	}{
		{name: "default headed chromium", client: Client{}, wantBrowser: []string{"chromium"}, wantNoShell: true},
		{name: "default headless chromium needs the headless shell", client: Client{Headless: true}, wantBrowser: []string{"chromium"}},
		{name: "chromium channel uses the full build", client: Client{BrowserType: "chromium", Headless: true}, wantBrowser: []string{"chromium"}, wantNoShell: true},
		{name: "firefox", client: Client{BrowserType: "firefox"}, wantBrowser: []string{"firefox"}},
		{name: "webkit", client: Client{BrowserType: "webkit"}, wantBrowser: []string{"webkit"}},
		{name: "system browser channel keeps upstream defaults", client: Client{BrowserType: "msedge"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.client.runOptions()
			assert.Same(t, os.Stderr, opts.Stdout, "install progress must not go to stdout")
			assert.Equal(t, tt.wantBrowser, opts.Browsers)
			assert.Equal(t, tt.wantNoShell, opts.NoInstallShell)
			assert.False(t, opts.OnlyInstallShell)
		})
	}
}
