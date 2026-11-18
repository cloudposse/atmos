package browser

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/mxschmitt/playwright-go"
	"github.com/sirupsen/logrus"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_page_test.go -package=$GOPACKAGE
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_request_test.go -package=$GOPACKAGE github.com/mxschmitt/playwright-go Request,BrowserType,Browser

var logger = logrus.WithField("provider", "browser")

const DEFAULT_TIMEOUT float64 = 300000

// storageStateDirPerm is the permission for the directory holding the browser storage state,
// which contains IdP session cookies.
const storageStateDirPerm = 0o700

// storageStateFilePerm is the permission for the browser storage state file.
const storageStateFilePerm = 0o600

// ErrNoSAMLResponse is returned when the browser flow ends without capturing the AWS sign-in request.
var ErrNoSAMLResponse = errors.New("no SAML response captured from the AWS sign-in request")

// errSAMLRequestCaptured ends an ExpectRequest wait when the OnRequest callback already captured the request.
var errSAMLRequestCaptured = errors.New("AWS sign-in request already captured")

// samlPage is the subset of playwright.Page used to capture the SAML response.
// Atmos fork: narrowed from playwright.Page so the capture logic can be unit-tested with mocks.
type samlPage interface {
	Goto(url string, options ...playwright.PageGotoOptions) (playwright.Response, error)
	OnRequest(fn func(playwright.Request))
	ExpectRequest(urlOrPredicate any, cb func() error, options ...playwright.PageExpectRequestOptions) (playwright.Request, error)
}

// storageStatePath returns the path of the persisted browser storage state.
// Atmos fork: built with filepath.Join and the Atmos homedir package instead of a
// forward-slash fmt.Sprintf, which produced mixed separators on Windows.
var storageStatePath = func() (string, error) {
	userHomeDir, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userHomeDir, ".aws", "saml2aws", "storageState.json"), nil
}

// Client client for browser based Identity Provider
type Client struct {
	BrowserType           string
	BrowserExecutablePath string
	Headless              bool
	// Setup alternative directory to download playwright browsers to
	BrowserDriverDir string
	Timeout          int
	BrowserAutoFill  bool
}

// New create new browser based client
func New(idpAccount *cfg.IDPAccount) (*Client, error) {
	return &Client{
		Headless:              idpAccount.Headless,
		BrowserDriverDir:      idpAccount.BrowserDriverDir,
		BrowserType:           strings.ToLower(idpAccount.BrowserType),
		BrowserExecutablePath: idpAccount.BrowserExecutablePath,
		Timeout:               idpAccount.Timeout,
		BrowserAutoFill:       idpAccount.BrowserAutoFill,
	}, nil
}

// contains checks if a string is present in a slice
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

// validBrowserTypes are the browser channels Playwright can launch.
var validBrowserTypes = []string{"chromium", "firefox", "webkit", "chrome", "chrome-beta", "chrome-dev", "chrome-canary", "msedge", "msedge-beta", "msedge-dev", "msedge-canary"}

// validateBrowserType rejects an unknown browser type.
// Atmos fork: runs before Playwright is installed or started, so a typo fails fast.
func (cl *Client) validateBrowserType() error {
	if len(cl.BrowserType) > 0 && !contains(validBrowserTypes, cl.BrowserType) {
		return fmt.Errorf("invalid browser-type: '%s', only %s are allowed", cl.BrowserType, validBrowserTypes)
	}
	return nil
}

// Atmos fork: the Playwright entry points are package variables so tests can verify that
// every resource is released on each early return without starting a real driver.
var (
	installPlaywright = playwright.Install
	runPlaywright     = playwright.Run
	stopPlaywright    = func(pw *playwright.Playwright) error { return pw.Stop() }
)

// runOptions returns the Playwright driver options used to install and run the driver.
//
// Atmos fork: upstream used zero-value RunOptions, so `playwright install` printed its
// download progress to stdout (corrupting the caller's data output, for example
// `eval $(atmos auth env)`) and downloaded every Playwright browser (Chromium, the Chromium
// headless shell, Firefox, WebKit and FFmpeg) although only one browser is launched.
func (cl *Client) runOptions() *playwright.RunOptions {
	opts := &playwright.RunOptions{
		DriverDirectory: cl.BrowserDriverDir,
		Stdout:          os.Stderr,
	}
	switch cl.BrowserType {
	case "", "chromium":
		opts.Browsers = []string{"chromium"}
		// Only the default headless Chromium launches the headless shell; a headed browser
		// and the explicit "chromium" channel use the full Chromium build.
		opts.NoInstallShell = cl.BrowserType != "" || !cl.Headless
	case "firefox", "webkit":
		opts.Browsers = []string{cl.BrowserType}
	}
	// Other browser types are channels of a browser installed on the system (Chrome, Edge);
	// keep upstream's behavior of installing the default browsers for them.
	return opts
}

func (cl *Client) Authenticate(loginDetails *creds.LoginDetails) (string, error) {
	if err := cl.validateBrowserType(); err != nil {
		return "", err
	}

	runOptions := cl.runOptions()

	// Optionally download browser drivers if specified
	if loginDetails.DownloadBrowser {
		err := installPlaywright(runOptions)
		if err != nil {
			return "", err
		}
	}

	pw, err := runPlaywright(runOptions)
	if err != nil {
		return "", err
	}
	// Atmos fork: upstream released Playwright, the browser and the context only in one
	// defer registered after the page was created, so a failed Launch, storage-state lookup,
	// NewContext or NewPage leaked the driver process (and the browser). Each resource is
	// now released by its own defer as soon as it exists. Defers run in reverse order, which
	// keeps upstream's shutdown order: save state, close context, close browser, stop driver.
	defer func() {
		if err := stopPlaywright(pw); err != nil {
			logger.Info("Error when stopping pm", err)
		}
	}()

	// TODO: provide some overrides for this window
	launchOptions := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(cl.Headless),
	}

	if cl.BrowserType != "" {
		logger.Info(fmt.Sprintf("Setting browser type: %s", cl.BrowserType))
		launchOptions.Channel = playwright.String(cl.BrowserType)
	}

	// Default browser is Chromium as it is widely supported for Identity providers,
	// It can also be set to the other playwright browsers: Firefox and WebKit
	browserType := pw.Chromium
	switch cl.BrowserType {
	case "firefox":
		browserType = pw.Firefox
	case "webkit":
		browserType = pw.WebKit
	}

	// You can set the path to a browser executable to run instead of the playwright-go bundled one. If `executablePath`
	// is a relative path, then it is resolved relative to the current working directory.
	// Note that Playwright only works with the bundled Chromium, Firefox or WebKit, use at your own risk. see:
	if len(cl.BrowserExecutablePath) > 0 {
		logger.Info(fmt.Sprintf("Setting browser executable path: %s", cl.BrowserExecutablePath))
		launchOptions.ExecutablePath = &cl.BrowserExecutablePath
	}

	// currently using the main browsers supported by Playwright: Chromium, Firefox or Webkit
	//
	// this is a sandboxed browser window so password managers and addons are separate
	browser, err := browserType.Launch(launchOptions)
	if err != nil {
		return "", err
	}
	defer func() {
		logger.Info("clean up browser")
		if err := browser.Close(); err != nil {
			logger.Info("Error when closing browser", err)
		}
	}()

	// create Context Optionsf
	contextOptions := playwright.BrowserNewContextOptions{}

	// load saved storageState if present and add to contextOptions
	statePath, err := storageStatePath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(statePath); err == nil {
		contextOptions.StorageStatePath = playwright.String(statePath)
	}

	// Create new broswer context
	context, err := browser.NewContext(contextOptions)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := context.Close(); err != nil {
			logger.Info("Error when closing context", err)
		}
	}()

	page, err := context.NewPage()
	if err != nil {
		return "", err
	}

	defer func() {
		logger.Info("saving storage state")
		if err := saveStorageState(context, statePath); err != nil {
			logger.Info("Error saving storage state", err)
		}
	}()

	return getSAMLResponse(page, loginDetails, cl)
}

// storageStateSaver is the subset of playwright.BrowserContext used to persist the storage state.
type storageStateSaver interface {
	StorageState(path ...playwright.BrowserContextStorageStateOptions) (*playwright.StorageState, error)
}

// saveStorageState writes the browser storage state (IdP session cookies) to statePath.
// Atmos fork: Playwright creates (os.Create, 0644) and writes the state file directly, so the cookies
// were world-readable until a later chmod, and a failed save could clobber the previous state. The state
// is saved to a private (0600) temporary file in a private (0700) directory and only moved over statePath
// after a successful save, so a failure leaves the previous state untouched.
func saveStorageState(saver storageStateSaver, statePath string) error {
	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, storageStateDirPerm); err != nil {
		return err
	}
	// MkdirAll does not tighten an existing directory.
	if err := os.Chmod(dir, storageStateDirPerm); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(statePath)+".*.tmp") // Created with 0600.
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if _, err := saver.StorageState(playwright.BrowserContextStorageStateOptions{Path: playwright.String(tmpPath)}); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, storageStateFilePerm); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, statePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// getSAMLResponse navigates to the IdP and captures the SAMLResponse posted to the AWS sign-in endpoint.
//
// Atmos fork fixes over upstream v2.36.19:
//   - A timeout or error from ExpectRequest returned a nil request that was then dereferenced (panic).
//   - A PostData error was swallowed because the wrong error variable was returned.
//   - The request callback and the caller shared data without synchronization (data race).
//   - An empty capture now fails with ErrNoSAMLResponse instead of returning an empty assertion.
//   - A request the callback captured just before ExpectRequest started listening is used instead
//     of waiting for the full timeout and failing.
var getSAMLResponse = func(page samlPage, loginDetails *creds.LoginDetails, client *Client) (string, error) {
	var (
		mu      sync.Mutex
		data    string
		dataErr error
	)

	logger.WithField("URL", loginDetails.URL).Info("opening browser")

	signinRe, err := signinRegex()
	if err != nil {
		return "", err
	}

	// record stores the request body. Atmos fork: a captured SAMLResponse is never replaced by a later
	// request that carries none (for example a GET to the same URL, whose body is empty), which upstream
	// overwrote and then waited for another request until the timeout.
	record := func(postData string, postErr error) {
		mu.Lock()
		defer mu.Unlock()
		if dataErr == nil && hasSAMLResponse(data) && (postErr != nil || !hasSAMLResponse(postData)) {
			return
		}
		data, dataErr = postData, postErr
	}

	page.OnRequest(func(request playwright.Request) {
		if signinRe.MatchString(request.URL()) {
			postData, postErr := request.PostData()
			record(postData, postErr)
		}
	})
	if _, err := page.Goto(loginDetails.URL); err != nil {
		return "", err
	}

	if client.BrowserAutoFill {
		if fullPage, ok := page.(playwright.Page); ok {
			if err := autoFill(fullPage, loginDetails); err != nil {
				logger.Error("error when auto filling", err)
			}
		}
	}

	logger.Info("waiting ...")
	capturedByCallback := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return data != ""
	}
	if !capturedByCallback() {
		// Atmos fork: the sign-in request can reach the OnRequest callback after the check
		// above but before ExpectRequest starts listening. ExpectRequest then never sees it
		// and upstream waited for the full timeout and failed. The callback passed to
		// ExpectRequest runs once the waiter listens and ends the wait when the request was
		// already captured, and a failed wait still uses a request the callback captured.
		r, err := page.ExpectRequest(signinRe, func() error {
			if capturedByCallback() {
				return errSAMLRequestCaptured
			}
			return nil
		}, client.expectRequestTimeout())
		switch {
		case (err != nil || r == nil) && capturedByCallback():
			// Use the request captured by the OnRequest callback.
		case err != nil:
			return "", fmt.Errorf("waiting for the AWS sign-in request: %w", err)
		case r == nil:
			return "", ErrNoSAMLResponse
		default:
			postData, postErr := r.PostData()
			record(postData, postErr)
		}
	}

	mu.Lock()
	captured, capturedErr := data, dataErr
	mu.Unlock()
	if capturedErr != nil {
		return "", fmt.Errorf("reading the AWS sign-in request body: %w", capturedErr)
	}

	values, err := url.ParseQuery(captured)
	if err != nil {
		return "", err
	}

	samlResponse := values.Get("SAMLResponse")
	if samlResponse == "" {
		return "", ErrNoSAMLResponse
	}

	return samlResponse, nil
}

// hasSAMLResponse reports whether the request body carries a non-empty SAMLResponse form field.
func hasSAMLResponse(body string) bool {
	values, err := url.ParseQuery(body)
	return err == nil && values.Get("SAMLResponse") != ""
}

var autoFill = func(page playwright.Page, loginDetails *creds.LoginDetails) error {
	passwordField := page.Locator("input[type='password']")
	err := passwordField.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		return err
	}

	err = passwordField.Fill(loginDetails.Password)
	if err != nil {
		return err
	}

	keyboard := page.Keyboard()

	// move to username field which is above password field
	err = keyboard.Press("Shift+Tab")
	if err != nil {
		return err
	}

	err = keyboard.InsertText(loginDetails.Username)
	if err != nil {
		return err
	}

	// Find the submit button or input of the form that the password field is in
	submitLocator := page.Locator("form", playwright.PageLocatorOptions{
		Has: passwordField,
	}).Locator("[type='submit']")
	count, err := submitLocator.Count()
	if err != nil {
		return err
	}

	// when submit locator exists, Click it
	if count > 0 {
		return submitLocator.Click()
	} else { // Use javascript to submit the form when no submit input or button is found
		_, err := page.Evaluate(`document.querySelector('input[type="password"]').form.submit()`, nil)
		return err
	}
}

func signinRegex() (*regexp.Regexp, error) {
	// https://docs.aws.amazon.com/general/latest/gr/signin-service.html
	// https://docs.amazonaws.cn/en_us/aws/latest/userguide/endpoints-Ningxia.html
	// https://docs.amazonaws.cn/en_us/aws/latest/userguide/endpoints-Beijing.html
	return regexp.Compile(`https:\/\/((.*\.)?signin\.(aws\.amazon\.com|amazonaws-us-gov\.com|amazonaws\.cn))\/saml`)
}

func (cl *Client) Validate(loginDetails *creds.LoginDetails) error {
	if loginDetails.URL == "" {
		return errors.New("empty URL")
	}

	return nil
}

func (cl *Client) expectRequestTimeout() playwright.PageExpectRequestOptions {
	timeout := float64(cl.Timeout)
	if timeout < 30000 {
		timeout = DEFAULT_TIMEOUT
	}
	return playwright.PageExpectRequestOptions{Timeout: &timeout}
}
