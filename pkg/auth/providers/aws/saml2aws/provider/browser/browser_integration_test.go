package browser

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

// These tests launch a real browser. They download the Playwright driver and Chromium
// (hundreds of MB), so they only run when RUN_PLAYWRIGHT_INTEGRATION=1 is set.

const (
	// resultTimeout bounds the wait for the fixture's sform() to publish the credentials.
	resultTimeout = 10 * time.Second
	// resultPollInterval is how often the fixture's result is polled.
	resultPollInterval = 50 * time.Millisecond
)

func requirePlaywrightIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_PLAYWRIGHT_INTEGRATION") != "1" {
		t.Skipf("Skipping real-browser test (set RUN_PLAYWRIGHT_INTEGRATION=1 to run)")
	}
	if testing.Short() {
		t.Skipf("Skipping real-browser test in short mode")
	}
	// The run is opted in, so an installation failure is a real failure, not a skip.
	if err := playwright.Install(&playwright.RunOptions{Browsers: []string{"chromium"}}); err != nil {
		t.Fatalf("could not install Playwright and Chromium: %v", err)
	}
}

func TestAuthenticateWithRealBrowser_Integration(t *testing.T) {
	requirePlaywrightIntegration(t)

	original := getSAMLResponse
	t.Cleanup(func() { getSAMLResponse = original })
	getSAMLResponse = func(samlPage, *creds.LoginDetails, *Client) (string, error) {
		return testAssertion, nil
	}
	// Use a temporary storage state so the test never reads or rewrites the developer's browser session file.
	originalPath := storageStatePath
	t.Cleanup(func() { storageStatePath = originalPath })
	statePath := filepath.Join(t.TempDir(), "storageState.json")
	storageStatePath = func() (string, error) { return statePath, nil }

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><body>idp</body></html>"))
	}))
	t.Cleanup(ts.Close)

	client := &Client{Headless: true}
	got, err := client.Authenticate(&creds.LoginDetails{URL: ts.URL})
	require.NoError(t, err)
	assert.Equal(t, testAssertion, got)
	assert.FileExists(t, statePath, "the browser state is saved to the temporary path")
}

func TestAuthenticateInvalidBrowserExecutablePath_Integration(t *testing.T) {
	requirePlaywrightIntegration(t)

	client := &Client{Headless: true, BrowserExecutablePath: "FAKEPATH"}
	_, err := client.Authenticate(&creds.LoginDetails{URL: testIdPURL})
	require.ErrorContains(t, err, "FAKEPATH")
}

func TestAutoFill_Integration(t *testing.T) {
	requirePlaywrightIntegration(t)

	pw, err := playwright.Run()
	require.NoError(t, err)
	t.Cleanup(func() { _ = pw.Stop() })
	browser, err := pw.Chromium.Launch()
	require.NoError(t, err)
	t.Cleanup(func() { _ = browser.Close() })

	for _, pageLocation := range []string{
		"example/loginpage.html",
		"example/loginpage-button-submit.html",
		"example/loginpage-without-submit.html",
	} {
		t.Run(pageLocation, func(t *testing.T) {
			data, err := os.ReadFile(pageLocation)
			require.NoError(t, err)

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(data)
			}))
			t.Cleanup(ts.Close)

			context, err := browser.NewContext()
			require.NoError(t, err)
			t.Cleanup(func() { _ = context.Close() })
			page, err := context.NewPage()
			require.NoError(t, err)
			_, err = page.Goto(ts.URL)
			require.NoError(t, err)

			loginDetails := &creds.LoginDetails{URL: ts.URL, Username: "golang", Password: "gopher"}
			require.NoError(t, autoFill(page, loginDetails))

			username, _ := page.Locator("input[name='username']").First().InputValue()
			assert.Equal(t, "golang", username)
			password, _ := page.Locator("input[type='password']").First().InputValue()
			assert.Equal(t, "gopher", password)

			// The form submission is only queued when autoFill returns, so wait for sform() to fill #result.
			resultLocator := page.Locator("div#result")
			require.NoError(t, resultLocator.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible}))
			assert.Eventually(t, func() bool {
				text, _ := resultLocator.InnerText()
				return text == "golang:gopher"
			}, resultTimeout, resultPollInterval)
		})
	}
}
