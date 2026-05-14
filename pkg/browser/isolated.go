package browser

import (
	"fmt"
	"runtime"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// isolatedOpener opens URLs in an isolated Chrome browser context.
type isolatedOpener struct {
	chrome     *ChromeInfo
	sessionDir string
	runner     CommandRunner
}

// Open opens the URL in an isolated Chrome browser session.
func (o *isolatedOpener) Open(url string) error {
	defer perf.Track(nil, "browser.isolatedOpener.Open")()

	log.Debug("Opening isolated browser session",
		"session_dir", o.sessionDir,
		"chrome", o.chrome.Path,
	)

	userDataDirFlag := fmt.Sprintf("--user-data-dir=%s", o.sessionDir)

	if runtime.GOOS == "darwin" && o.chrome.UseMacOSOpen {
		// macOS: use `open -na "Google Chrome" --args --user-data-dir=<dir> <url>`.
		return o.runner.Run("open", "-na", o.chrome.AppName, "--args", userDataDirFlag, url)
	}

	// Linux/Windows: invoke Chrome directly.
	return o.runner.Run(o.chrome.Path, userDataDirFlag, url)
}
