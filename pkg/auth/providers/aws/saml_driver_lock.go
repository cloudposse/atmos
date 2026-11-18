package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// driverSeedLockName is the lock file, inside the driver directory, that serializes driver seeding
	// across Atmos processes. It lives outside package/, so a rollback of the package never removes it.
	driverSeedLockName = ".atmos-driver-seed.lock"

	// driverSeedLockTimeout bounds the wait for another process's seeding. It covers the four downloads
	// of a full seed (npm metadata, the playwright-core tarball, the Node.js archive and its checksums),
	// each bounded by preseedDownloadTimeout.
	driverSeedLockTimeout = 4*preseedDownloadTimeout + time.Minute
)

// withDriverSeedLock runs fn while holding an exclusive cross-process lock on the driver directory.
// Without it, a process whose seeding failed could roll back a package that another process had
// finished seeding in the meantime, and two processes could write the same package files at once.
func withDriverSeedLock(driverDir string, fn func() error) error {
	if err := os.MkdirAll(driverDir, preseedFileMode); err != nil {
		return fmt.Errorf("%w: could not create driver directory: %w", errUtils.ErrPlaywrightDriverSeed, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), driverSeedLockTimeout)
	defer cancel()

	err := filelock.New(filepath.Join(driverDir, driverSeedLockName)).WithExclusive(ctx, fn)
	if errors.Is(err, filelock.ErrAcquire) {
		return fmt.Errorf("%w: could not lock the driver directory: %w", errUtils.ErrPlaywrightDriverSeed, err)
	}
	return err
}

// removeSeededPackage removes the driver package written by a failed seeding attempt. It runs only
// when package/cli.js did not exist before the attempt, so nothing usable is removed. It runs under
// the driver seed lock, so it cannot remove a package that another process seeded.
func removeSeededPackage(driverDir string) {
	if err := os.RemoveAll(filepath.Join(driverDir, playwrightPackageDir)); err != nil {
		log.Debug("Could not remove the partially seeded Playwright driver", "dir", driverDir, "error", err)
	}
}

// playwrightDriverSeeded reports whether every part of the driver that ensurePlaywrightDriver seeds is
// already present: package/cli.js, unless PLAYWRIGHT_CLI_PATH selects an external CLI, and the Node.js
// binary, unless PLAYWRIGHT_NODEJS_PATH selects a preinstalled Node.js.
func playwrightDriverSeeded(driverDir string) bool {
	cliReady := externalPlaywrightCLI() || fileExists(playwrightDriverCLI(driverDir))
	nodeReady := externalPlaywrightNode() || fileExists(filepath.Join(driverDir, playwrightNodeName()))
	return cliReady && nodeReady
}

// externalPlaywrightNode reports whether PLAYWRIGHT_NODEJS_PATH selects a preinstalled Node.js runtime.
func externalPlaywrightNode() bool {
	return os.Getenv("PLAYWRIGHT_NODEJS_PATH") != "" //nolint:forbidigo // Third-party (playwright-go) env var, read the same way the library reads it.
}

// playwrightDriverCLI returns the path of the driver CLI (package/cli.js) inside driverDir.
func playwrightDriverCLI(driverDir string) string {
	return filepath.Join(driverDir, playwrightPackageDir, "cli.js")
}

// playwrightNodeName returns the file name of the Node.js binary seeded into the driver directory.
func playwrightNodeName() string {
	if runtime.GOOS == windowsOS {
		return "node.exe"
	}
	return "node"
}

// fileExists reports whether path can be stat'ed.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
