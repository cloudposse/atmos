package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsCICacheCommand(t *testing.T) {
	ci := &cobra.Command{Use: "ci"}
	cacheGroup := &cobra.Command{Use: "cache"}
	restore := &cobra.Command{Use: "restore"}
	ci.AddCommand(cacheGroup)
	cacheGroup.AddCommand(restore)

	assert.True(t, isCICacheCommand(cacheGroup), "the cache group itself qualifies")
	assert.True(t, isCICacheCommand(restore), "a cache subcommand qualifies")

	// A `cache` command not parented by `ci` must not match.
	other := &cobra.Command{Use: "other"}
	lonelyCache := &cobra.Command{Use: "cache"}
	other.AddCommand(lonelyCache)
	assert.False(t, isCICacheCommand(lonelyCache))

	// An unrelated command does not match.
	unrelated := &cobra.Command{Use: "version"}
	assert.False(t, isCICacheCommand(unrelated))
}

func TestAutoRestore_NilConfig(t *testing.T) {
	// Must not panic and must be a no-op.
	AutoRestore(&cobra.Command{Use: "x"}, nil)
}

func TestAutoRestore_Disabled(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	ac.CI.Cache.Enabled = false
	AutoRestore(&cobra.Command{Use: "x"}, ac)
}

func TestAutoRestore_SkipsCICacheCommand(t *testing.T) {
	ci := &cobra.Command{Use: "ci"}
	cacheGroup := &cobra.Command{Use: "cache"}
	ci.AddCommand(cacheGroup)

	ac := &schema.AtmosConfiguration{}
	ac.CI.Cache.Enabled = true
	ac.CI.Cache.Auto = "both"
	AutoRestore(cacheGroup, ac)
}

func TestAutoRestore_AutoOffIsNoOp(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	ac := &schema.AtmosConfiguration{}
	ac.CI.Cache.Enabled = true
	ac.CI.Cache.Auto = "off"
	AutoRestore(&cobra.Command{Use: "version"}, ac)
}

func TestAutoRestore_NoCIProviderDetected(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	// Auto restore enabled, but no CI provider is detected in the test
	// environment, so the DetectCache error branch is exercised (no panic).
	ac := &schema.AtmosConfiguration{}
	ac.CI.Cache.Enabled = true
	ac.CI.Cache.Auto = "restore"
	AutoRestore(&cobra.Command{Use: "version"}, ac)
}

func TestRunPendingSave_NoPending(t *testing.T) {
	// Ensure nothing is registered, then run: must be a no-op.
	registerPendingSave(nil)
	RunPendingSave()
}

func TestRegisterAndRunPendingSave(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "toolchain", "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "toolchain", "bin", "tool"), []byte("bin"), 0o644))

	cfg := &cachepkg.Config{Enabled: true, Auto: "both", Root: root, Key: "k1", Compression: "gzip"}
	fake := newFakeBackend()
	mgr := cachepkg.NewManager(fake, cfg)

	registerPendingSave(mgr)
	RunPendingSave()

	_, ok := fake.blobs["k1"]
	assert.True(t, ok, "pending save should upload under the configured key")

	// Pending save is cleared after running.
	RunPendingSave()
	require.Len(t, fake.blobs, 1, "second run must be a no-op")
}

// newPendingSaveManager builds a real Manager over the cmd fakeBackend rooted at
// a temp dir holding one file, so RunPendingSave exercises a genuine Save.
func newPendingSaveManager(t *testing.T, fake *fakeBackend) *cachepkg.Manager {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "toolchain", "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "toolchain", "bin", "tool"), []byte("bin"), 0o644))
	cfg := &cachepkg.Config{Enabled: true, Auto: "both", Root: root, Key: "k1", Compression: "gzip"}
	return cachepkg.NewManager(fake, cfg)
}

func TestRunPendingSave_SaveError(t *testing.T) {
	fake := newFakeBackend()
	fake.saveErr = errUtils.ErrCacheSaveFailed
	registerPendingSave(newPendingSaveManager(t, fake))

	// A failing save is logged (warn branch) and must not panic.
	RunPendingSave()
	// Pending save is cleared even on failure.
	RunPendingSave()
}

func TestRunPendingSave_Skipped(t *testing.T) {
	fake := newFakeBackend()
	// "Already exists remotely" makes Save report Skipped.
	fake.saveErr = errUtils.ErrCacheAlreadyExists
	registerPendingSave(newPendingSaveManager(t, fake))

	RunPendingSave()
}

func TestAutoRestoreHelper_LogsOutcome(t *testing.T) {
	// Drive the internal autoRestore helper directly across hit and miss.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "toolchain", "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "toolchain", "bin", "tool"), []byte("bin"), 0o644))
	cfg := &cachepkg.Config{Enabled: true, Auto: "both", Root: root, Key: "k1", Compression: "gzip"}
	fake := newFakeBackend()
	mgr := cachepkg.NewManager(fake, cfg)

	// Miss: nothing seeded.
	autoRestore(mgr, cfg)

	// Hit: seed then restore.
	_, err := mgr.Save(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, "toolchain", "bin", "tool")))
	// Clear the per-root state marker so the restore is not skipped as idempotent.
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".atmos-cache")))
	autoRestore(mgr, cfg)
	_, statErr := os.Stat(filepath.Join(root, "toolchain", "bin", "tool"))
	require.NoError(t, statErr)
}
