package installer

import (
	"errors"
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

func TestInstallFromTool_VerifiesChecksumBeforeExtraction(t *testing.T) {
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

	installer := &Installer{
		cacheDir:           t.TempDir(),
		binDir:             t.TempDir(),
		useLockFile:        true,
		lockFilePath:       filepath.Join(t.TempDir(), "toolchain.lock.yaml"),
		verificationPolicy: verification.Policy{Checksums: verification.PolicyWhenAvailable, Signatures: verification.PolicyDisabled},
	}
	binaryPath, err := installer.installFromTool(&registry.Tool{
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
	assert.FileExists(t, binaryPath)
	lockData, err := os.ReadFile(installer.lockFilePath)
	require.NoError(t, err)
	assert.Contains(t, string(lockData), "checksum_algorithm: sha256")
	assert.Contains(t, string(lockData), sum)
}

func TestInstallFromTool_DoesNotUpdateLockFileWhenExtractionFails(t *testing.T) {
	assetName := "tool.zip"
	asset := []byte("not a zip archive")
	sum := "a90877b716582c036052f8b70ed0e9a60464ad21daa00ed4c77d2f478ca16239"
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

	installer := &Installer{
		cacheDir:           t.TempDir(),
		binDir:             t.TempDir(),
		useLockFile:        true,
		lockFilePath:       filepath.Join(t.TempDir(), "toolchain.lock.yaml"),
		verificationPolicy: verification.Policy{Checksums: verification.PolicyWhenAvailable, Signatures: verification.PolicyDisabled},
	}
	_, err := installer.installFromTool(&registry.Tool{
		Type:      "http",
		RepoOwner: "owner",
		RepoName:  "tool",
		Asset:     ts.URL + "/" + assetName,
		Format:    "zip",
		Checksum: registry.ChecksumConfig{
			Type:      "http",
			URL:       ts.URL + "/checksums.txt",
			Algorithm: "sha256",
		},
	}, "1.0.0")

	require.ErrorIs(t, err, ErrFileOperation)
	assert.NoFileExists(t, installer.lockFilePath)
}

func TestInstallFromTool_RemovesTamperedCachedAsset(t *testing.T) {
	assetName := EnsureWindowsExeExtension("tool")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/checksums.txt":
			_, _ = w.Write([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	cacheDir := t.TempDir()
	cachedPath := filepath.Join(cacheDir, assetName)
	require.NoError(t, os.WriteFile(cachedPath, []byte("tampered"), 0o644))
	require.NoError(t, os.WriteFile(cacheSourceURLPath(cachedPath), []byte(ts.URL+"/"+assetName+"\n"), 0o644))

	installer := &Installer{
		cacheDir:           cacheDir,
		binDir:             t.TempDir(),
		verificationPolicy: verification.Policy{Checksums: verification.PolicyWhenAvailable, Signatures: verification.PolicyDisabled},
	}
	_, err := installer.installFromTool(&registry.Tool{
		Type:      "http",
		RepoOwner: "owner",
		RepoName:  "tool",
		Asset:     ts.URL + "/" + assetName,
		Checksum: registry.ChecksumConfig{
			Type:       "http",
			URL:        ts.URL + "/checksums.txt",
			FileFormat: "raw",
			Algorithm:  "sha256",
		},
	}, "1.0.0")

	require.Error(t, err)
	assert.True(t, errors.Is(err, verification.ErrChecksumMismatch) || errors.Is(err, verification.ErrChecksumNotFound))
	assert.NoFileExists(t, cachedPath)
}
