package aws

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mxschmitt/playwright-go"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// chromiumBrowserName is the Chromium entry name in Playwright's browsers.json.
	chromiumBrowserName = "chromium"
	// firefoxBrowserName is the Firefox entry name in Playwright's browsers.json.
	firefoxBrowserName = "firefox"
	// webkitBrowserName is the WebKit entry name in Playwright's browsers.json.
	webkitBrowserName = "webkit"
	// playwrightBrowserManifest is the browser manifest shipped in the playwright-core package.
	playwrightBrowserManifest = "browsers.json"
	// playwrightInstallMarker is the file Playwright writes into a browser directory once the
	// browser is fully extracted. A directory without it is a partial download.
	playwrightInstallMarker = "INSTALLATION_COMPLETE"
	// playwrightLocalBrowsers is the in-package browser directory used when PLAYWRIGHT_BROWSERS_PATH=0.
	playwrightLocalBrowsers = ".local-browsers"
)

// requiredBrowserDir returns the registry directory name (for example chromium-1248) of the
// browser revision the bundled Playwright driver launches for browserName on this host.
// It is read from the driver's package/browsers.json, which exists once the driver is installed or pre-seeded.
var requiredBrowserDir = func(browserName string) (string, error) {
	opts := &playwright.RunOptions{}
	if _, err := playwright.NewDriver(opts); err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrPlaywrightBrowserManifest, err)
	}
	return readBrowserDir(playwrightManifestPath(opts.DriverDirectory), browserName, playwrightHostPlatform())
}

// playwrightManifestPath returns the browsers.json path of the Playwright driver.
// playwright-core keeps browsers.json next to cli.js, so an external PLAYWRIGHT_CLI_PATH
// (for example a distro-packaged driver) also moves the manifest.
func playwrightManifestPath(driverDir string) string {
	if cliPath := os.Getenv("PLAYWRIGHT_CLI_PATH"); cliPath != "" { //nolint:forbidigo // Third-party (playwright-go) env var, read the same way the library reads it.
		return filepath.Join(filepath.Dir(cliPath), playwrightBrowserManifest)
	}
	return filepath.Join(driverDir, "package", playwrightBrowserManifest)
}

// readBrowserDir reads the revision of browserName from a Playwright browsers.json manifest and
// returns the registry directory name the driver installs that revision into.
//
// It mirrors playwright-core's readDescriptors: a revisionOverrides entry for hostPlatform wins over
// the default revision and is installed into <name>_<hostPlatform>_special-<revision>; otherwise the
// directory is <name>-<revision>. Dashes in the prefix are replaced with underscores.
func readBrowserDir(manifestPath, browserName, hostPlatform string) (string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrPlaywrightBrowserManifest, err)
	}

	var manifest struct {
		Browsers []struct {
			Name              string            `json:"name"`
			Revision          string            `json:"revision"`
			RevisionOverrides map[string]string `json:"revisionOverrides"`
		} `json:"browsers"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrPlaywrightBrowserManifest, manifestPath, err)
	}

	for _, browser := range manifest.Browsers {
		if browser.Name != browserName {
			continue
		}
		if override := browser.RevisionOverrides[hostPlatform]; hostPlatform != "" && override != "" {
			prefix := browser.Name + "_" + hostPlatform + "_special"
			return strings.ReplaceAll(prefix, "-", "_") + "-" + override, nil
		}
		if browser.Revision != "" {
			return strings.ReplaceAll(browser.Name, "-", "_") + "-" + browser.Revision, nil
		}
	}

	return "", fmt.Errorf("%w: %s has no %s revision", errUtils.ErrPlaywrightBrowserManifest, manifestPath, browserName)
}

// playwrightRegistryDir returns the directory the Playwright driver installs browsers into
// and launches them from. It mirrors playwright-core's registryDirectory logic:
// PLAYWRIGHT_BROWSERS_PATH wins ("0" means the driver package's .local-browsers directory);
// otherwise <cache>/ms-playwright, where <cache> is $XDG_CACHE_HOME or ~/.cache on Linux,
// ~/Library/Caches on macOS, and %LOCALAPPDATA% or ~/AppData/Local on Windows.
func playwrightRegistryDir() (string, error) {
	switch browsersPath := playwrightBrowsersPathEnv(); browsersPath {
	case "":
	case "0":
		opts := &playwright.RunOptions{}
		if _, err := playwright.NewDriver(opts); err != nil {
			return "", err
		}
		// The playwright-core package root is the directory holding cli.js and browsers.json.
		return filepath.Join(filepath.Dir(playwrightManifestPath(opts.DriverDirectory)), playwrightLocalBrowsers), nil
	default:
		// Playwright resolves a relative path against INIT_CWD (set by npm) before the working directory.
		if filepath.IsAbs(browsersPath) {
			return filepath.Clean(browsersPath), nil
		}
		if initCwd := os.Getenv("INIT_CWD"); initCwd != "" { //nolint:forbidigo // Third-party (npm/Playwright) env var, read the same way the driver reads it.
			return filepath.Abs(filepath.Join(initCwd, browsersPath))
		}
		return filepath.Abs(browsersPath)
	}

	cacheDir, err := playwrightDefaultCacheDir(runtime.GOOS)
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, playwrightCacheDir), nil
}

// playwrightBrowsersPathEnv returns the configured PLAYWRIGHT_BROWSERS_PATH using Playwright's
// precedence: the variable itself, then its npm_config_ and npm_package_config_ lowercase forms.
func playwrightBrowsersPathEnv() string {
	for _, name := range []string{
		"PLAYWRIGHT_BROWSERS_PATH",
		"npm_config_playwright_browsers_path",
		"npm_package_config_playwright_browsers_path",
	} {
		if value := os.Getenv(name); value != "" { //nolint:forbidigo // Third-party (Playwright) env vars, read the same way the driver reads them.
			return value
		}
	}
	return ""
}

// playwrightDefaultCacheDir returns the cache base directory Playwright uses on goos.
func playwrightDefaultCacheDir(goos string) (string, error) {
	var envDir string
	switch goos {
	case "linux":
		envDir = os.Getenv("XDG_CACHE_HOME") //nolint:forbidigo // Read the same way the Playwright driver reads it.
	case windowsOS:
		envDir = os.Getenv("LOCALAPPDATA") //nolint:forbidigo // Read the same way the Playwright driver reads it.
	case "darwin":
	default:
		return "", fmt.Errorf("%w: Playwright does not support %s", errUtils.ErrUnsupportedPlatform, goos)
	}
	if envDir != "" {
		return envDir, nil
	}

	homeDir, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	switch goos {
	case "linux":
		return filepath.Join(homeDir, ".cache"), nil
	case windowsOS:
		return filepath.Join(homeDir, "AppData", "Local"), nil
	default:
		return filepath.Join(homeDir, "Library", "Caches"), nil
	}
}

// playwrightManagedBrowser maps a configured browser_type to the Playwright browser that the
// browser driver launches. Only Chromium, Firefox and WebKit are installed by Playwright; the other
// valid types are channels of a browser installed on the system (Chrome, Edge), and an unknown type
// is rejected by the driver, so neither is downloaded.
// The browser driver launches Chromium headed (Atmos never sets Headless), which uses the full
// Chromium build, not the headless shell.
func playwrightManagedBrowser(browserType string) (string, bool) {
	switch strings.ToLower(browserType) {
	case "", chromiumBrowserName:
		return chromiumBrowserName, true
	case firefoxBrowserName:
		return firefoxBrowserName, true
	case webkitBrowserName:
		return webkitBrowserName, true
	default:
		return "", false
	}
}

// requiredBrowserInstalled reports whether the revision of browserName required by the bundled
// Playwright driver is fully installed where the driver will look for it.
// Playwright installs each browser as <registry>/<name>-<revision> (see readBrowserDir) and writes
// an INSTALLATION_COMPLETE marker into it after extraction.
func (p *samlProvider) requiredBrowserInstalled(browserName string) bool {
	dirName, err := requiredBrowserDir(browserName)
	if err != nil {
		log.Debug("Cannot determine the required browser revision", "browser", browserName, "error", err)
		return false
	}

	registryDir, err := playwrightRegistryDir()
	if err != nil {
		log.Debug("Cannot determine the Playwright browser directory", "error", err)
		return false
	}

	browserDir := filepath.Join(registryDir, dirName)
	if _, err := os.Stat(filepath.Join(browserDir, playwrightInstallMarker)); err != nil {
		log.Debug("Required browser revision is not installed", "browser", browserName, "path", browserDir)
		return false
	}

	log.Debug("Found the required browser revision", "browser", browserName, "path", browserDir)
	return true
}
