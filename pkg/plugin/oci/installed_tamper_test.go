package oci

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// extractedRelease returns a verified release and a freshly extracted payload directory.
func extractedRelease(t *testing.T) (*Release, string) {
	t.Helper()
	directory, descriptors := validationFixture(t)
	index, err := BuildIndex(directory, descriptors)
	require.NoError(t, err)
	release, err := Inspect(index, "")
	require.NoError(t, err)
	destination := t.TempDir()
	require.NoError(t, release.Extract("linux/amd64", destination))
	require.NoError(t, release.VerifyInstalled("linux/amd64", destination), "an intact payload must verify")
	return release, destination
}

func TestVerifyInstalledDetectsTamperingAsCacheCorruption(t *testing.T) {
	executable := filepath.Join("bin", "plugin")
	cases := []struct {
		name    string
		windows bool
		tamper  func(t *testing.T, payload string)
	}{
		{"appended byte", true, func(t *testing.T, payload string) {
			file, err := os.OpenFile(filepath.Join(payload, executable), os.O_APPEND|os.O_WRONLY, 0)
			require.NoError(t, err)
			_, err = file.Write([]byte{0})
			require.NoError(t, err)
			require.NoError(t, file.Close())
		}},
		{"extra file", true, func(t *testing.T, payload string) {
			require.NoError(t, os.WriteFile(filepath.Join(payload, "bin", "extra"), []byte("planted"), 0o644))
		}},
		{"extra top-level file", true, func(t *testing.T, payload string) {
			require.NoError(t, os.WriteFile(filepath.Join(payload, "planted.so"), []byte("planted"), 0o644))
		}},
		{"extra directory", true, func(t *testing.T, payload string) {
			require.NoError(t, os.Mkdir(filepath.Join(payload, "planted"), 0o755))
		}},
		{"chmod 777 executable", false, func(t *testing.T, payload string) {
			require.NoError(t, os.Chmod(filepath.Join(payload, executable), 0o777))
		}},
		{"removed executable bit", false, func(t *testing.T, payload string) {
			require.NoError(t, os.Chmod(filepath.Join(payload, executable), 0o644))
		}},
		{"executable bit added to metadata", false, func(t *testing.T, payload string) {
			require.NoError(t, os.Chmod(filepath.Join(payload, "plugin.yaml"), 0o755))
		}},
		{"deleted file", true, func(t *testing.T, payload string) {
			require.NoError(t, os.Remove(filepath.Join(payload, "plugin.yaml")))
		}},
		{"deleted payload directory", true, func(t *testing.T, payload string) {
			require.NoError(t, os.RemoveAll(payload))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.windows && runtime.GOOS == "windows" {
				t.Skip("POSIX permission bits are not preserved on Windows.")
			}
			release, payload := extractedRelease(t)
			tc.tamper(t, payload)
			err := release.VerifyInstalled("linux/amd64", payload)
			require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
			require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
		})
	}
}

func TestVerifyInstalledDoesNotClassifyUnknownPlatformAsCorruption(t *testing.T) {
	release, payload := extractedRelease(t)
	err := release.VerifyInstalled("missing", payload)
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	require.NotErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
}
