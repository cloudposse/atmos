package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

// cacheCorruptions lists every way a cached payload can stop matching its verified archive.
var cacheCorruptions = []struct {
	name    string
	windows bool
	corrupt func(t *testing.T, payload string)
}{
	{"appended byte", true, func(t *testing.T, payload string) {
		file, err := os.OpenFile(filepath.Join(payload, "bin", "plugin"), os.O_APPEND|os.O_WRONLY, 0)
		require.NoError(t, err)
		_, err = file.Write([]byte{0})
		require.NoError(t, err)
		require.NoError(t, file.Close())
	}},
	{"extra file", true, func(t *testing.T, payload string) {
		require.NoError(t, os.WriteFile(filepath.Join(payload, "planted"), []byte("x"), 0o644))
	}},
	{"chmod 777", false, func(t *testing.T, payload string) {
		require.NoError(t, os.Chmod(filepath.Join(payload, "bin", "plugin"), 0o777))
	}},
	{"removed executable bit", false, func(t *testing.T, payload string) {
		require.NoError(t, os.Chmod(filepath.Join(payload, "bin", "plugin"), 0o644))
	}},
	{"deleted file", true, func(t *testing.T, payload string) {
		require.NoError(t, os.Remove(filepath.Join(payload, "plugin.yaml")))
	}},
}

func skipUnlessSupported(t *testing.T, windows bool) {
	t.Helper()
	if !windows && runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not preserved on Windows.")
	}
}

func quarantines(t *testing.T, payload string) []string {
	t.Helper()
	release := filepath.Dir(payload)
	matches, err := filepath.Glob(release + ".corrupt-*")
	require.NoError(t, err)
	return matches
}

func TestInstalledReportsCorruptionDistinctFromAbsence(t *testing.T) {
	for _, tc := range cacheCorruptions {
		t.Run(tc.name, func(t *testing.T) {
			skipUnlessSupported(t, tc.windows)
			service, release, _ := installFixture(t)
			payload, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
			require.NoError(t, err)
			entry := lockedEntry(release)
			_, err = service.Cache.Installed(entry, CurrentPlatform())
			require.NoError(t, err)
			tc.corrupt(t, payload)
			_, err = service.Cache.Installed(entry, CurrentPlatform())
			require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
			require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
			require.NotErrorIs(t, err, errUtils.ErrPluginNotInstalled)
		})
	}
	service, release, _ := installFixture(t)
	_, err := service.Cache.Installed(lockedEntry(release), CurrentPlatform())
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	require.NotErrorIs(t, err, errUtils.ErrPluginCacheCorrupt, "an absent release is not corrupt")
}

func TestInstalledTreatsDamagedLayoutAsCorruption(t *testing.T) {
	service, release, _ := installFixture(t)
	payload, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(filepath.Dir(payload), "oci", "blobs")))
	_, err = service.Cache.Installed(lockedEntry(release), CurrentPlatform())
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
}

func TestStoreQuarantinesAndRepairsCorruptRelease(t *testing.T) {
	for _, tc := range cacheCorruptions {
		t.Run(tc.name, func(t *testing.T) {
			skipUnlessSupported(t, tc.windows)
			service, release, _ := installFixture(t)
			payload, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
			require.NoError(t, err)
			tc.corrupt(t, payload)
			require.Empty(t, quarantines(t, payload))

			repaired, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
			require.NoError(t, err)
			require.Equal(t, payload, repaired)
			require.NoError(t, release.VerifyInstalled(CurrentPlatform(), repaired), "the repaired release verifies")
			_, err = service.Cache.Installed(lockedEntry(release), CurrentPlatform())
			require.NoError(t, err)
			moved := quarantines(t, payload)
			require.Len(t, moved, 1, "the corrupt release is kept for inspection")
			require.DirExists(t, moved[0])
		})
	}
}

func TestStoreNeverQuarantinesIntactRelease(t *testing.T) {
	service, release, _ := installFixture(t)
	payload, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	executable := filepath.Join(payload, "bin", "plugin")
	before, err := os.Stat(executable)
	require.NoError(t, err)
	for range 3 {
		again, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
		require.NoError(t, err)
		require.Equal(t, payload, again)
	}
	require.Empty(t, quarantines(t, payload), "an intact release is never quarantined")
	after, err := os.Stat(executable)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after), "an intact release is not refetched")
}

func TestStoreDoesNotRepairUnrelatedFailures(t *testing.T) {
	service, release, _ := installFixture(t)
	payload, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	service.Cache.Directory = filepath.Join(payload, "bin", "plugin")
	_, err = service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.Error(t, err)
	require.NotErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
}

// layoutRequest builds a locked request whose replacement bytes come from an offline layout.
func layoutRequest(t *testing.T, service *Service, directory string, release *pluginoci.Release) *Request {
	t.Helper()
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	locked := lockedEntry(release)
	locked.Source, locked.Constraint = "oci://ghcr.io/acme/plugin", "*"
	return &Request{Source: locked.Source, Constraint: locked.Constraint, Locked: locked, Layout: layout, Alias: "fixture"}
}

func corruptedService(t *testing.T) (*Service, *pluginoci.Release, string, string) {
	t.Helper()
	service, release, directory := installFixture(t)
	payload, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(payload, "planted"), []byte("x"), 0o644))
	return service, release, directory, payload
}

func TestPrepareLockedRepairsCorruptReleaseWhenNotFrozen(t *testing.T) {
	service, release, directory, payload := corruptedService(t)
	request := layoutRequest(t, service, directory, release)
	entry, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, release.Digest, entry.IndexDigest, "repair re-fetches by the locked digest")
	require.NoFileExists(t, filepath.Join(payload, "planted"))
	require.Len(t, quarantines(t, payload), 1)
	_, err = service.Cache.Installed(entry, CurrentPlatform())
	require.NoError(t, err)
}

func TestPrepareLockedFrozenFailsWithoutMutating(t *testing.T) {
	service, release, directory, payload := corruptedService(t)
	request := layoutRequest(t, service, directory, release)
	request.Frozen = true
	_, err := service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "fixture", "the hint names the alias")
	require.Contains(t, hints, filepath.Dir(payload), "the hint names the cache path")
	require.Contains(t, hints, "atmos plugin install", "the hint names the repair command")
	require.FileExists(t, filepath.Join(payload, "planted"), "frozen mode never mutates the cache")
	require.Empty(t, quarantines(t, payload))
}

func TestPrepareLockedOfflineWithoutLayoutFailsWithoutMutating(t *testing.T) {
	service, release, directory, payload := corruptedService(t)
	request := layoutRequest(t, service, directory, release)
	request.Layout = ""
	service.Config.Plugins.Policy.Offline = true
	_, err := service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "atmos plugin install")
	require.FileExists(t, filepath.Join(payload, "planted"))
	require.Empty(t, quarantines(t, payload))
}

func TestPrepareLockedIntactReleaseIsNeverRefetched(t *testing.T) {
	service, release, directory := installFixture(t)
	payload, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	request := layoutRequest(t, service, directory, release)
	request.Layout = "" // A refetch would fail: no layout, no registry.
	service.Config.Plugins.Policy.Offline = true
	_, err = service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, quarantines(t, payload))
}

func lockedEntry(release *pluginoci.Release) *pluginlock.Entry {
	entry := &pluginlock.Entry{Plugin: release.Name, Version: release.Version, IndexDigest: release.Digest, Runtime: release.Definition.Spec.Runtime, Platforms: map[string]string{}}
	for platform := range release.Platforms {
		descriptor := release.Platforms[platform]
		entry.Platforms[platform] = descriptor.Digest.String()
	}
	return entry
}

func TestStoreDoesNotPromoteRejectedOrCanceledValidation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "validator rejects payload", true: "canceled during validation"}[canceled], func(t *testing.T) {
			service, release, _ := installFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			descriptors := extension.New()
			validated := false
			require.NoError(t, descriptors.Register(extension.Descriptor{
				Capability: "fixture", Path: []string{"fixtures"}, Version: 1,
				Validate: func(binding extension.Binding) error {
					validated = true
					require.FileExists(t, filepath.Join(binding.Directory, "bin", "plugin"))
					if canceled {
						cancel()
						return nil
					}
					return os.ErrPermission
				},
			}))
			payload, err := service.Cache.Store(ctx, release, CurrentPlatform(), descriptors)
			if canceled {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, os.ErrPermission)
			}
			require.True(t, validated)
			require.Empty(t, payload)
			directory, err := service.Cache.location(release.Digest, CurrentPlatform())
			require.NoError(t, err)
			require.NoDirExists(t, directory, "unvalidated payloads must not be published")
			staging, err := filepath.Glob(filepath.Join(filepath.Dir(directory), ".install-*"))
			require.NoError(t, err)
			require.Empty(t, staging, "failed staging must not leave extracted executable bytes")
			_, err = service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
			require.NoError(t, err, "a rejected attempt must not poison a later install")
		})
	}
}

func TestInstalledRejectsValidOCILayoutWithoutPluginMetadata(t *testing.T) {
	cache, err := NewCache(t.TempDir())
	require.NoError(t, err)
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: empty.Image})
	digest, err := index.Digest()
	require.NoError(t, err)
	entry := &pluginlock.Entry{IndexDigest: digest.String()}
	directory, err := cache.location(entry.IndexDigest, CurrentPlatform())
	require.NoError(t, err)
	require.NoError(t, atmosoci.SaveIndex(filepath.Join(directory, "oci"), index))
	_, err = cache.Installed(entry, CurrentPlatform())
	require.ErrorIs(t, err, errUtils.ErrPluginCacheCorrupt)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.NotErrorIs(t, err, errUtils.ErrPluginNotInstalled)
	require.DirExists(t, directory, "an inspection failure must preserve the suspect bytes")
}

func TestStoreRepairsMissingOCIBlobsWithoutTrustingCachedPayload(t *testing.T) {
	service, release, _ := installFixture(t)
	payload, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(filepath.Dir(payload), "oci", "blobs")))
	repaired, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
	require.Equal(t, payload, repaired)
	require.Len(t, quarantines(t, payload), 1)
	_, err = service.Cache.Installed(lockedEntry(release), CurrentPlatform())
	require.NoError(t, err, "repair must restore verified OCI evidence, not just retain payload files")
}

func TestStoreRejectsPayloadChangedAfterMetadataInspection(t *testing.T) {
	service, _, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	index, err := atmosoci.LoadIndex(layout, "")
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, "")
	require.NoError(t, err)
	image, err := index.Image(release.Platforms[CurrentPlatform()].Digest)
	require.NoError(t, err)
	metadata, err := image.Manifest()
	require.NoError(t, err)
	digest := metadata.Layers[0].Digest
	blob := filepath.Join(layout, "blobs", digest.Algorithm, digest.Hex)
	original, err := os.ReadFile(blob)
	require.NoError(t, err)
	changed := append([]byte(nil), original...)
	changed[len(changed)-1] ^= 1
	require.NoError(t, os.WriteFile(blob, changed, 0o600))
	_, err = service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	cached, err := service.Cache.location(release.Digest, CurrentPlatform())
	require.NoError(t, err)
	require.NoDirExists(t, cached)
	staging, err := filepath.Glob(filepath.Join(filepath.Dir(cached), ".install-*"))
	require.NoError(t, err)
	require.Empty(t, staging, "failed integrity verification must remove staged executable bytes")
	require.NoError(t, os.WriteFile(blob, original, 0o600))
	_, err = service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
	require.NoError(t, err)
}
