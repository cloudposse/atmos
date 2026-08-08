package installer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

// TestLockTool_WritesLockEntryWithoutInstalling verifies LockTool writes a real,
// checksum-populated toolchain.lock.yaml entry -- the same guarantee
// TestInstallFromTool_VerifiesChecksumBeforeExtraction covers for a real install -- while
// never placing a binary in binDir, since locking must not require (or perform) an
// install.
func TestLockTool_WritesLockEntryWithoutInstalling(t *testing.T) {
	assetName := EnsureWindowsExeExtension("tool")
	asset := []byte("#!/bin/sh\n")
	sum := "a8076d3d28d21e02012b20eaf7dbf75409a6277134439025f282e368e3305abf"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + assetName:
			_, _ = w.Write(asset)
		case "/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum, assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	binDir := t.TempDir()
	installer := &Installer{
		cacheDir:           t.TempDir(),
		binDir:             binDir,
		useLockFile:        true,
		lockFilePath:       filepath.Join(t.TempDir(), "toolchain.lock.yaml"),
		verificationPolicy: verification.Policy{Checksums: verification.PolicyWhenAvailable, Signatures: verification.PolicyDisabled},
	}

	err := installer.LockTool(&registry.Tool{
		Type:      "http",
		RepoOwner: "owner",
		RepoName:  "tool",
		Asset:     ts.URL + "/" + assetName,
		Checksum: registry.ChecksumConfig{
			Type:      "http",
			URL:       ts.URL + "/checksums.txt",
			Algorithm: "sha256",
		},
	}, "1.0.0")
	require.NoError(t, err)

	lockData, err := os.ReadFile(installer.lockFilePath)
	require.NoError(t, err)
	assert.Contains(t, string(lockData), "checksum_algorithm: sha256")
	assert.Contains(t, string(lockData), sum)

	// The whole point of LockTool: no binary placed under binDir, so an already-installed
	// tool's real binary (or the absence of one) is left untouched.
	entries, err := os.ReadDir(binDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "LockTool must not extract/install a binary into binDir")
}

// TestLockTool_LockingMultipleVersionsOfSameToolRetainsBoth reproduces a field-test finding:
// toolchain.lock.yaml's schema keys tool entries by "owner/repo" alone, with a single Version
// field -- so a .tool-versions line pinning two versions of the same tool (a legitimate,
// documented asdf-style pattern, e.g. examples/toolchain/.tool-versions' "yq 4.45.1 4.50.1")
// reports "locked" for both versions, but the second LockTool call silently overwrites the
// first version's checksum/URL/platform data entirely. This must be fixed (e.g. by keying lock
// entries per-version) so both versions' verified checksums survive.
func TestLockTool_LockingMultipleVersionsOfSameToolRetainsBoth(t *testing.T) {
	asset1Name := EnsureWindowsExeExtension("tool-v1")
	asset1 := []byte("#!/bin/sh\necho v1\n")
	sum1 := "667f61a069b75f1f57fc561623da00e013e144221e8ea54de2803da9dd8aa952"

	asset2Name := EnsureWindowsExeExtension("tool-v2")
	asset2 := []byte("#!/bin/sh\necho v2\n")
	sum2 := "268005d39f7e216d686f4e63ed1e4b4a1b6a06552b5ca4d1a17e1a176e3ce249"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + asset1Name:
			_, _ = w.Write(asset1)
		case "/" + asset2Name:
			_, _ = w.Write(asset2)
		case "/checksums-v1.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum1, asset1Name)
		case "/checksums-v2.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum2, asset2Name)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	installer := &Installer{
		cacheDir:           t.TempDir(),
		binDir:             t.TempDir(),
		useLockFile:        true,
		lockFilePath:       filepath.Join(t.TempDir(), "toolchain.lock.yaml"),
		verificationPolicy: verification.Policy{Checksums: verification.PolicyWhenAvailable, Signatures: verification.PolicyDisabled},
	}

	err := installer.LockTool(&registry.Tool{
		Type: "http", RepoOwner: "owner", RepoName: "tool",
		Asset:    ts.URL + "/" + asset1Name,
		Checksum: registry.ChecksumConfig{Type: "http", URL: ts.URL + "/checksums-v1.txt", Algorithm: "sha256"},
	}, "1.0.0")
	require.NoError(t, err)

	err = installer.LockTool(&registry.Tool{
		Type: "http", RepoOwner: "owner", RepoName: "tool",
		Asset:    ts.URL + "/" + asset2Name,
		Checksum: registry.ChecksumConfig{Type: "http", URL: ts.URL + "/checksums-v2.txt", Algorithm: "sha256"},
	}, "2.0.0")
	require.NoError(t, err)

	lockData, err := os.ReadFile(installer.lockFilePath)
	require.NoError(t, err)
	assert.Contains(t, string(lockData), sum1, "locking version 2.0.0 must not silently discard version 1.0.0's checksum")
	assert.Contains(t, string(lockData), sum2, "version 2.0.0's checksum must also be recorded")
}

// TestLockTool_NoOpsWhenLockFileDisabled verifies LockTool doesn't write a lock file (or
// error) when useLockFile is false -- callers that want to force writing regardless (like
// `atmos toolchain lock`) must use WithForceLockFile.
func TestLockTool_NoOpsWhenLockFileDisabled(t *testing.T) {
	assetName := EnsureWindowsExeExtension("tool")
	asset := []byte("#!/bin/sh\n")
	sum := "a8076d3d28d21e02012b20eaf7dbf75409a6277134439025f282e368e3305abf"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + assetName:
			_, _ = w.Write(asset)
		case "/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum, assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	lockFilePath := filepath.Join(t.TempDir(), "toolchain.lock.yaml")
	installer := &Installer{
		cacheDir:           t.TempDir(),
		binDir:             t.TempDir(),
		useLockFile:        false,
		lockFilePath:       lockFilePath,
		verificationPolicy: verification.Policy{Checksums: verification.PolicyWhenAvailable, Signatures: verification.PolicyDisabled},
	}

	err := installer.LockTool(&registry.Tool{
		Type:      "http",
		RepoOwner: "owner",
		RepoName:  "tool",
		Asset:     ts.URL + "/" + assetName,
		Checksum: registry.ChecksumConfig{
			Type:      "http",
			URL:       ts.URL + "/checksums.txt",
			Algorithm: "sha256",
		},
	}, "1.0.0")
	require.NoError(t, err)
	assert.NoFileExists(t, lockFilePath)
}
