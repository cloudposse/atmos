package browser

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Constants for Chrome executable name and subdirectories.
const (
	chromeExe  = "chrome.exe"
	chromePath = "Google/Chrome/Application"
)

// ChromeInfo holds information about a detected Chrome/Chromium installation.
type ChromeInfo struct {
	// Path is the full path to the Chrome executable.
	Path string
	// UseMacOSOpen indicates whether to use the macOS `open -na` pattern.
	UseMacOSOpen bool
	// AppName is the application name for macOS `open -na` (e.g., "Google Chrome").
	AppName string
}

// DetectChrome finds a Chrome or Chromium installation on the system.
// Returns ErrChromeNotFound if no installation is found.
func DetectChrome() (*ChromeInfo, error) {
	defer perf.Track(nil, "browser.DetectChrome")()

	switch runtime.GOOS {
	case "darwin":
		return detectChromeDarwin()
	case "linux":
		return detectChromeLinux()
	case "windows":
		return detectChromeWindows()
	default:
		return nil, fmt.Errorf("%w: unsupported platform %s", errUtils.ErrChromeNotFound, runtime.GOOS)
	}
}

// detectChromeDarwin finds Chrome on macOS.
func detectChromeDarwin() (*ChromeInfo, error) {
	// Check for Chrome/Chromium apps in /Applications.
	apps := []struct {
		name string
		path string
	}{
		{"Google Chrome", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
		{"Chromium", "/Applications/Chromium.app/Contents/MacOS/Chromium"},
		{"Google Chrome Canary", "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary"},
	}

	for _, app := range apps {
		if _, err := os.Stat(app.path); err == nil {
			log.Debug("Found Chrome on macOS", "app", app.name, "path", app.path)
			return &ChromeInfo{
				Path:         app.path,
				UseMacOSOpen: true,
				AppName:      app.name,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w: no Chrome or Chromium found in /Applications", errUtils.ErrChromeNotFound)
}

// detectChromeLinux finds Chrome on Linux via PATH.
func detectChromeLinux() (*ChromeInfo, error) {
	candidates := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium-browser",
		"chromium",
	}

	for _, name := range candidates {
		path, err := exec.LookPath(name)
		if err == nil {
			log.Debug("Found Chrome on Linux", "name", name, "path", path)
			return &ChromeInfo{
				Path: path,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w: no Chrome or Chromium found in PATH", errUtils.ErrChromeNotFound)
}

// detectChromeWindows finds Chrome on Windows.
func detectChromeWindows() (*ChromeInfo, error) {
	// Check common installation paths on Windows.
	var candidates []string

	// Program Files paths.
	programFilesVals := []string{
		"C:",
		"Program Files",
		chromePath,
		chromeExe,
	}
	candidates = append(candidates, filepath.Join(programFilesVals...))

	programFilesX86Vals := []string{
		"C:",
		"Program Files (x86)",
		chromePath,
		chromeExe,
	}
	candidates = append(candidates, filepath.Join(programFilesX86Vals...))

	// User LocalAppData path (for per-user Chrome installations).
	if usr, err := user.Current(); err == nil {
		localAppDataVals := []string{
			usr.HomeDir,
			"AppData",
			"Local",
			chromePath,
			chromeExe,
		}
		candidates = append(candidates, filepath.Join(localAppDataVals...))
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			log.Debug("Found Chrome on Windows", "path", path)
			return &ChromeInfo{
				Path: path,
			}, nil
		}
	}

	// Try PATH as fallback.
	path, err := exec.LookPath(chromeExe)
	if err == nil {
		return &ChromeInfo{
			Path: path,
		}, nil
	}

	return nil, fmt.Errorf("%w: no Chrome found in standard locations or PATH", errUtils.ErrChromeNotFound)
}
