package installer

import (
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	toolchainlock "github.com/cloudposse/atmos/pkg/toolchain/lockfile"
	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

// TestLockChecksumAlgorithmFallback covers explicit, inferred, and upstream algorithms.
func TestLockChecksumAlgorithmFallback(t *testing.T) {
	for _, tt := range []struct {
		name      string
		checksum  string
		algorithm string
		fallback  string
		want      string
	}{
		{"explicit algorithm wins", strings.Repeat("a", sha256.Size*2), "sha1", "sha512", "sha1"},
		{"infer sha512", strings.Repeat("a", sha512.Size*2), "", "sha256", "sha512"},
		{"infer sha256", strings.Repeat("a", sha256.Size*2), "", "sha512", "sha256"},
		{"preserve fallback", strings.Repeat("a", 40), "", "sha1", "sha1"},
		{"preserve empty fallback", "unknown", "", "", ""},
		{"missing checksum", "", "sha512", "sha256", "sha256"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			i := installerWithLockedChecksum(t, tt.checksum, tt.algorithm)
			algorithm, err := i.lockChecksumAlgorithm("owner/tool", "1.0.0", tt.fallback)
			require.NoError(t, err)
			require.Equal(t, tt.want, algorithm)
		})
	}
}

// TestPrepareLockChecksumWithoutAlgorithm ensures legacy entries still detect tampering
// without rewriting the recorded artifact metadata.
func TestPrepareLockChecksumWithoutAlgorithm(t *testing.T) {
	asset := []byte("original artifact")
	checksum := fmt.Sprintf("%x", sha512.Sum512(asset))
	for _, tampered := range []bool{false, true} {
		t.Run(fmt.Sprintf("tampered=%t", tampered), func(t *testing.T) {
			i := installerWithLockedChecksum(t, checksum, "")
			before, err := os.ReadFile(i.lockFilePath)
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "artifact")
			contents := asset
			if tampered {
				contents = []byte("changed artifact")
			}
			require.NoError(t, os.WriteFile(path, contents, 0o600))
			tool := &registry.Tool{RepoOwner: "owner", RepoName: "tool"}
			result := &verification.Result{}
			require.NoError(t, i.prepareLockChecksum(tool, "1.0.0", path, result))
			require.Equal(t, "sha512", result.ChecksumAlgorithm)
			err = i.checkLockFileChecksumMismatch(tool, "1.0.0", result)
			if tampered {
				require.ErrorIs(t, err, ErrLockfileChecksumMismatch)
			} else {
				require.NoError(t, err)
				require.Equal(t, checksum, result.Checksum)
			}
			after, err := os.ReadFile(i.lockFilePath)
			require.NoError(t, err)
			require.Equal(t, before, after, "checksum validation must preserve the lock entry")
		})
	}
}

// installerWithLockedChecksum creates an installer with one current-platform artifact entry.
func installerWithLockedChecksum(t *testing.T, checksum, algorithm string) *Installer {
	t.Helper()
	i := &Installer{useLockFile: true, verifyAgainstLock: true, lockFilePath: filepath.Join(t.TempDir(), "toolchain.lock.yaml")}
	lf := newInstallerLockFile()
	entry := getOrCreateInstallerToolVersion(lf, "owner/tool", "1.0.0")
	entry.Platforms[runtime.GOOS+"_"+runtime.GOARCH] = &toolchainlock.PlatformEntry{
		URL: "https://example.com/artifact", Checksum: checksum, ChecksumAlgorithm: algorithm,
	}
	require.NoError(t, saveInstallerLockFile(i.lockFilePath, lf))
	return i
}
