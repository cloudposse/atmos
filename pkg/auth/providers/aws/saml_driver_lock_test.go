package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
)

// Regression: ensurePlaywrightDriver checked for package/cli.js, seeded it and rolled it back on failure
// without a lock, so a process whose Node.js download failed could remove a package that another process
// had finished seeding meanwhile. Seeding now waits for the driver seed lock and re-checks the driver.
func TestEnsurePlaywrightDriver_WaitsForConcurrentSeeder(t *testing.T) {
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	// Without the lock, this server makes the waiting process fetch the package, fail on Node.js and roll back.
	coreRequests := coreOnlyServer(t)
	cliPath := filepath.Join(driverDir, "package", "cli.js")

	result := make(chan error, 1)
	lock := filelock.New(filepath.Join(driverDir, driverSeedLockName))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err := lock.WithExclusive(ctx, func() error {
		// The other seeder holds the lock while the driver is still missing.
		go func() { result <- ensurePlaywrightDriver() }()
		assert.Never(t, func() bool { return coreRequests.Load() > 0 }, 300*time.Millisecond, 10*time.Millisecond,
			"seeding must wait for the lock held by another seeder")

		// The other seeder completes the driver before it releases the lock.
		require.NoError(t, os.MkdirAll(filepath.Dir(cliPath), 0o755))
		require.NoError(t, os.WriteFile(cliPath, []byte("seeded-by-other"), 0o644))
		return os.WriteFile(filepath.Join(driverDir, playwrightNodeName()), []byte("node"), 0o755)
	})
	require.NoError(t, err)

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("ensurePlaywrightDriver did not return after the lock was released")
	}
	assert.Zero(t, coreRequests.Load(), "the driver completed by another seeder is not fetched again")
	content, err := os.ReadFile(cliPath)
	require.NoError(t, err)
	assert.Equal(t, "seeded-by-other", string(content), "the completed package is neither replaced nor rolled back")
}

// Negative path: an already seeded driver is used without taking the lock, so a driver installed in a
// read-only directory keeps working.
func TestEnsurePlaywrightDriver_SeededDriverTakesNoLock(t *testing.T) {
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	// Unreachable hosts prove no network request is made.
	overridePreseedHosts(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	require.NoError(t, os.MkdirAll(filepath.Join(driverDir, "package"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(driverDir, "package", "cli.js"), []byte("cli"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(driverDir, playwrightNodeName()), []byte("node"), 0o755))

	require.NoError(t, ensurePlaywrightDriver())
	assert.NoFileExists(t, filepath.Join(driverDir, driverSeedLockName))
}

// The seed lock lives outside package/, so rolling back a failed seed keeps it in place.
func TestEnsurePlaywrightDriver_RollbackKeepsSeedLock(t *testing.T) {
	skipIfNodejsOrgArchiveUnsupported(t)
	driverDir := t.TempDir()
	t.Setenv("PLAYWRIGHT_DRIVER_PATH", driverDir)
	t.Setenv("PLAYWRIGHT_NODEJS_PATH", "")
	t.Setenv("PLAYWRIGHT_CLI_PATH", "")
	coreOnlyServer(t)

	require.ErrorIs(t, ensurePlaywrightDriver(), errUtils.ErrPlaywrightDriverSeed)
	assert.NoDirExists(t, filepath.Join(driverDir, "package"))
	assert.FileExists(t, filepath.Join(driverDir, driverSeedLockName))
}

// A driver directory that cannot be created is reported as a seed error, and fn never runs.
func TestWithDriverSeedLock_UncreatableDirectoryFails(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o644))

	ran := false
	err := withDriverSeedLock(filepath.Join(blocker, "driver"), func() error {
		ran = true
		return nil
	})
	require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
	assert.False(t, ran)
}

// Errors returned by fn pass through unchanged.
func TestWithDriverSeedLock_ReturnsFnError(t *testing.T) {
	err := withDriverSeedLock(t.TempDir(), func() error { return errUtils.ErrPlaywrightDriverIntegrity })
	require.ErrorIs(t, err, errUtils.ErrPlaywrightDriverIntegrity)
	assert.NotErrorIs(t, err, errUtils.ErrPlaywrightDriverSeed)
}
