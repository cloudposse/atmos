package installer

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

// TestVerificationUsesDownloadedVersion checks checksum templates after a legacy
// bare pin resolves to a prefixed release. The installation/lock key stays pinned.
func TestVerificationUsesDownloadedVersion(t *testing.T) {
	for _, operation := range []string{"install", "lock"} {
		t.Run(operation, func(t *testing.T) {
			assetName := EnsureWindowsExeExtension("tool")
			asset := []byte("test binary")
			sum := fmt.Sprintf("%x", sha256.Sum256(asset))
			var checksumRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1.0.0/" + assetName:
					_, _ = w.Write(asset)
				case "/v1.0.0/checksums.txt":
					checksumRequests.Add(1)
					_, _ = fmt.Fprintf(w, "%s  %s\n", sum, assetName)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			installer := &Installer{
				cacheDir: t.TempDir(), binDir: t.TempDir(), useLockFile: true,
				lockFilePath:       filepath.Join(t.TempDir(), "toolchain.lock.yaml"),
				verificationPolicy: verification.Policy{Checksums: verification.PolicyWhenAvailable, Signatures: verification.PolicyDisabled},
			}
			tool := &registry.Tool{
				Type: "http", RepoOwner: "example", RepoName: "tool",
				Asset: server.URL + "/{{.Version}}/" + assetName,
				Checksum: registry.ChecksumConfig{
					Type: "http", URL: server.URL + "/{{.Version}}/checksums.txt", Algorithm: "sha256",
				},
			}
			if operation == "lock" {
				require.NoError(t, installer.LockTool(tool, "1.0.0"))
			} else {
				binary, err := installer.installFromTool(tool, "1.0.0")
				require.NoError(t, err)
				assert.FileExists(t, binary)
				assert.Contains(t, binary, filepath.Join("example", "tool", "1.0.0"))
			}
			assert.Positive(t, checksumRequests.Load(), "verification must fetch checksums for the resolved release tag")
			lockData, err := os.ReadFile(installer.lockFilePath)
			require.NoError(t, err)
			assert.Contains(t, string(lockData), sum)
		})
	}
}
