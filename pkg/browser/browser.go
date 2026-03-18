package browser

import (
	"fmt"
	"os/exec"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/spf13/viper"
)

// Opener opens URLs in a browser.
type Opener interface {
	// Open opens the given URL in a browser.
	Open(url string) error
}

// CommandRunner executes external commands. Abstracted for testing.
type CommandRunner interface {
	// Run starts the command and does not wait for it to complete.
	Run(name string, args ...string) error
}

// execRunner is the default CommandRunner that uses os/exec.
type execRunner struct{}

func (r *execRunner) Run(name string, args ...string) error {
	defer perf.Track(nil, "browser.execRunner.Run")()

	return exec.Command(name, args...).Start()
}

// Option configures browser opening behavior.
type Option func(*config)

type config struct {
	isolated   bool
	sessionDir string
	runner     CommandRunner
}

// WithIsolatedSession enables isolated browser sessions using the given directory
// as Chrome's user data directory. The caller is responsible for creating the
// directory and choosing an appropriate path (e.g., based on realm, identity, or
// any other session key).
func WithIsolatedSession(sessionDir string) Option {
	defer perf.Track(nil, "browser.WithIsolatedSession")()

	return func(c *config) {
		c.isolated = true
		c.sessionDir = sessionDir
	}
}

// WithCommandRunner sets a custom command runner (for testing).
func WithCommandRunner(runner CommandRunner) Option {
	defer perf.Track(nil, "browser.WithCommandRunner")()

	return func(c *config) {
		c.runner = runner
	}
}

// New returns an Opener configured with the given options.
// When WithIsolatedSession is provided and Chrome is available, returns an isolated opener.
// Otherwise, returns a default system-browser opener.
func New(opts ...Option) Opener {
	defer perf.Track(nil, "browser.New")()

	cfg := &config{
		runner: &execRunner{},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.isolated {
		chrome, err := DetectChrome()
		if err != nil {
			log.Warn("Isolated browser sessions require Chrome/Chromium. Falling back to default browser.", "error", err)
			return &defaultOpener{runner: cfg.runner}
		}
		return &isolatedOpener{
			chrome:     chrome,
			sessionDir: cfg.sessionDir,
			runner:     cfg.runner,
		}
	}

	return &defaultOpener{runner: cfg.runner}
}

// defaultOpener opens URLs in the system default browser.
type defaultOpener struct {
	runner CommandRunner
}

func (o *defaultOpener) Open(url string) error {
	defer perf.Track(nil, "browser.defaultOpener.Open")()

	if viper.GetString("go.test") == "1" {
		log.Debug("Skipping browser launch in test environment")
		return nil
	}

	switch runtime.GOOS {
	case "linux":
		return o.runner.Run("xdg-open", url)
	case "windows":
		return o.runner.Run("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		return o.runner.Run("open", url)
	default:
		return fmt.Errorf("%w: %s", errUtils.ErrUnsupportedPlatform, runtime.GOOS)
	}
}
