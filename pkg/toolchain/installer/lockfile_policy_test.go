package installer

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	toolchainlock "github.com/cloudposse/atmos/pkg/toolchain/lockfile"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

func TestLockUpdatePreservesExistingEntry(t *testing.T) {
	i := &Installer{useLockFile: true, verifyAgainstLock: true, lockFilePath: filepath.Join(t.TempDir(), "toolchain.lock.yaml")}
	tool := &registry.Tool{RepoOwner: "owner", RepoName: "tool"}
	result := &verification.Result{Checksum: "original", ChecksumAlgorithm: "sha256"}
	require.NoError(t, i.updateLockFile(tool, "1.0.0", "https://example.com/original", result))
	before, err := os.ReadFile(i.lockFilePath)
	require.NoError(t, err)
	require.NoError(t, i.updateLockFile(tool, "1.0.0", "https://example.com/mirror", result))
	after, err := os.ReadFile(i.lockFilePath)
	require.NoError(t, err)
	require.Equal(t, before, after, "reinstalling must not rewrite an existing lock entry")

	// Recheck inside the write lock: another installer may have recorded an entry
	// after the pre-extraction check completed.
	err = i.updateLockFile(tool, "1.0.0", "https://example.com/mirror", &verification.Result{Checksum: "changed"})
	require.ErrorIs(t, err, ErrLockfileChecksumMismatch)
}

func TestFrozenInstall(t *testing.T) {
	asset := []byte("a test executable")
	sum := fmt.Sprintf("%x", sha256.Sum256(asset))
	for _, scenario := range []string{"complete", "missing file", "missing tool", "missing version", "missing platform", "missing checksum", "missing URL", "mismatch", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			var requests int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				_, _ = w.Write(asset)
			}))
			defer server.Close()
			tool := &registry.Tool{Type: "http", RepoOwner: "owner", RepoName: "tool", Asset: server.URL + "/tool", Format: "raw"}
			i := &Installer{cacheDir: t.TempDir(), binDir: t.TempDir(), useLockFile: true, verifyAgainstLock: true, lockFilePath: filepath.Join(t.TempDir(), "toolchain.lock.yaml")}
			lf := newInstallerLockFile()
			entry := getOrCreateInstallerToolVersion(lf, "owner/tool", "1.0.0")
			platform := runtime.GOOS + "_" + runtime.GOARCH
			entry.Platforms[platform] = &toolchainlock.PlatformEntry{URL: tool.Asset, Checksum: sum, ChecksumAlgorithm: "sha256"}
			switch scenario {
			case "missing tool":
				delete(lf.Tools, "owner/tool")
			case "missing version":
				delete(lf.Tools["owner/tool"].Versions, "1.0.0")
			case "missing platform":
				delete(entry.Platforms, platform)
			case "missing checksum":
				entry.Platforms[platform].Checksum = ""
			case "missing URL":
				entry.Platforms[platform].URL = ""
			case "mismatch":
				entry.Platforms[platform].Checksum = "unexpected"
			}
			if scenario != "missing file" {
				require.NoError(t, saveInstallerLockFile(i.lockFilePath, lf))
			}
			if scenario == "malformed" {
				require.NoError(t, os.WriteFile(i.lockFilePath, []byte("tools: ["), 0o644))
			}
			before, _ := os.ReadFile(i.lockFilePath)
			i.frozenLockFile = true
			binary, err := i.installFromTool(tool, "1.0.0")
			switch scenario {
			case "complete":
				require.NoError(t, err)
				require.FileExists(t, binary)
			case "mismatch":
				require.ErrorIs(t, err, ErrLockfileChecksumMismatch)
			default:
				require.ErrorIs(t, err, errUtils.ErrFrozenLockfile)
				require.Zero(t, requests, "incomplete locks must fail before downloading")
			}
			after, _ := os.ReadFile(i.lockFilePath)
			require.Equal(t, string(before), string(after))
			require.NoFileExists(t, i.lockFilePath+".lock", "frozen reads must not create a lock sidecar")
			if scenario != "complete" {
				files, err := os.ReadDir(i.binDir)
				require.NoError(t, err)
				require.Empty(t, files, "failed validation must precede extraction")
			}
		})
	}
}

func TestFrozenCachedBinaryRequiresLockEntry(t *testing.T) {
	i := &Installer{frozenLockFile: true, binDir: t.TempDir(), lockFilePath: filepath.Join(t.TempDir(), "missing.yaml")}
	binary := i.GetBinaryPath("owner", "tool", "1.0.0", "")
	require.NoError(t, os.MkdirAll(filepath.Dir(binary), 0o755))
	require.NoError(t, os.WriteFile(binary, []byte("cached"), 0o755))
	_, err := i.FindBinaryPath("owner", "tool", "1.0.0")
	require.ErrorIs(t, err, errUtils.ErrFrozenLockfile)
}

func TestFrozenCannotBeOverriddenByLockRefresh(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	i := New(WithAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{FrozenLockFile: true}}), WithForceLockFile())
	require.True(t, i.verifyAgainstLock)
	require.ErrorIs(t, i.LockTool(&registry.Tool{}, "1.0.0"), errUtils.ErrFrozenLockfile)
}
