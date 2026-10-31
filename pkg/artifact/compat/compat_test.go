package compat

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/artifact/local"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/oci/ocitest"
)

// Frozen compatibility constants. They were recorded from the code that existed before the
// artifact work and must not be edited to make a test pass: a failure here means a legacy CI
// planfile archive or a plugin release manifest changed identity.
const (
	// The legacy tar SHA-256 of the bundled planfile tar and the sha256 field of its sidecar.
	legacyTarSHA256 = "cfb77a4fd8064a9e743ec84d6ed64c6101a5684fc1b7d1fc18a2f6789051118d"

	// The plugin release index digest.
	pluginIndexDigest = "sha256:af34a37555cc97c7f1043fdb2ad3404a062d97d1ce07d96c5a594f6b5ac83b59"

	// The per-platform manifest digests of the plugin release.
	pluginManifestDigestLinuxAMD64  = "sha256:e249a05906c791f6881749476b7123acbfd0a7f37dfa94a397045e22b5c819ee"
	pluginManifestDigestDarwinARM64 = "sha256:6a988f06a391ca23572eb78363f6241d40127d05b7bc71facdebf0a8855b2380"

	// The digest of plugin.yaml recorded in the release annotations.
	pluginDefinitionDigest = "sha256:ca012d44c6a9ea139972472614caa656eb40d86e298ddcde501c216eb953b090"
)

// pluginBlobDigests lists every blob in the plugin release layout (index, two manifests, two
// configs, two bundle layers), sorted.
var pluginBlobDigests = []string{
	"192cdb60e7ddc392b8048fa5b026bd033ac034a0c79d60b0233b30b25cd3c485",
	"2e33831c0fd207c7001a3ad37e4055f47d3cddc851002a5ecee4d8c503156d79",
	"6a988f06a391ca23572eb78363f6241d40127d05b7bc71facdebf0a8855b2380",
	"9ecd9e0a855de88e01dc9e76dc7c17213a7c356b9778e19cfe0c52585b2d823f",
	"af34a37555cc97c7f1043fdb2ad3404a062d97d1ce07d96c5a594f6b5ac83b59",
	"b0c7d7c2a4ded93805cb35f506478b33d99b8a1dc65de9fc6683e4f26d51af7e",
	"e249a05906c791f6881749476b7123acbfd0a7f37dfa94a397045e22b5c819ee",
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func legacyArchivePath(root string) string {
	return filepath.Join(root, filepath.FromSlash(legacyKey))
}

func TestLegacyCIArchiveDigestIsStable(t *testing.T) {
	committed, err := os.ReadFile(legacyArchivePath(legacyFixtureDir()))
	require.NoError(t, err)
	require.Positive(t, len(committed), "fixture must not be empty")
	assert.Equal(t, legacyTarSHA256, sha256Hex(committed), "committed archive bytes changed")

	t.Run("current code reproduces the committed fixture byte for byte", func(t *testing.T) {
		rebuilt := t.TempDir()
		buildLegacyStore(t, rebuilt)
		got := treeBytes(t, rebuilt)
		want := treeBytes(t, legacyFixtureDir())
		require.Len(t, want, 2, "fixture holds the archive and its metadata sidecar")
		require.Equal(t, len(want), len(got))
		for name, content := range want {
			assert.Equal(t, content, got[name], "%s differs from what the current CreateTarArchive/BundledStore/local backend produce", name)
		}
	})

	t.Run("sidecar records the archive checksum", func(t *testing.T) {
		raw, err := os.ReadFile(legacyArchivePath(legacyFixtureDir()) + ".metadata.json")
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"sha256": "`+legacyTarSHA256+`"`)
	})

	t.Run("legacy encoding is mode 0644 with no timestamps in file order", func(t *testing.T) {
		raw, err := os.ReadFile(legacyArchivePath(legacyFixtureDir()))
		require.NoError(t, err)
		reader := tar.NewReader(strings.NewReader(string(raw)))
		var names []string
		for {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			names = append(names, header.Name)
			assert.Equal(t, int64(0o644), header.Mode, header.Name)
			assert.True(t, header.ModTime.IsZero() || header.ModTime.Unix() == 0, "%s has a modification time", header.Name)
		}
		assert.Equal(t, []string{"plan.tfplan", ".terraform.lock.hcl"}, names)
	})
}

func TestLegacyCIFixtureStillReadsThroughCurrentStore(t *testing.T) {
	copyRoot := t.TempDir()
	for name, content := range treeBytes(t, legacyFixtureDir()) {
		target := filepath.Join(copyRoot, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, content, 0o644))
	}
	backend, err := local.NewStore(artifact.StoreOptions{Type: "local", Options: map[string]any{"path": copyRoot}})
	require.NoError(t, err)
	store := artifact.NewBundledStore(backend)

	files, metadata, err := store.Download(t.Context(), legacyKey)
	require.NoError(t, err)
	require.Len(t, files, 2)
	bodies := map[string]string{}
	for _, file := range files {
		body, readErr := io.ReadAll(file.Data)
		require.NoError(t, readErr)
		require.NoError(t, file.Data.Close())
		bodies[file.Name] = string(body)
	}
	assert.Equal(t, map[string]string{"plan.tfplan": legacyPlanBody, ".terraform.lock.hcl": legacyLockBody}, bodies)
	assert.Equal(t, legacyMetadata().Stack, metadata.Stack)
	assert.Equal(t, legacyMetadata().SHA, metadata.SHA)
	assert.Equal(t, 42, metadata.PRNumber)
	assert.Equal(t, legacyTarSHA256, metadata.SHA256)
	assert.Equal(t, map[string]string{"env": "dev", "zone": "a"}, metadata.Custom)
	require.NoError(t, metadata.Validate(), "the legacy Metadata.Validate contract is unchanged")

	t.Run("a changed archive fails the integrity check", func(t *testing.T) {
		archive := legacyArchivePath(copyRoot)
		content, err := os.ReadFile(archive)
		require.NoError(t, err)
		content[0] ^= 0xff
		require.NoError(t, os.WriteFile(archive, content, 0o644))
		_, _, err = store.Download(t.Context(), legacyKey)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
}

func TestPluginReleaseFixtureDigestsAreStable(t *testing.T) {
	root := pluginFixtureDir()

	t.Run("layout holds exactly the recorded content-addressed blobs", func(t *testing.T) {
		tree := treeBytes(t, root)
		var blobs []string
		for name, content := range tree {
			digestHex, found := strings.CutPrefix(name, "blobs/sha256/")
			if !found {
				continue
			}
			blobs = append(blobs, digestHex)
			assert.Equal(t, digestHex, sha256Hex(content), "blob %s does not hash to its name", digestHex)
		}
		slices.Sort(blobs)
		assert.Equal(t, pluginBlobDigests, blobs)
		require.Contains(t, tree, "index.json")
		require.Contains(t, tree, "oci-layout")
	})

	t.Run("release inspects to the recorded manifest digests", func(t *testing.T) {
		index, err := atmosoci.LoadIndex(root, pluginIndexDigest)
		require.NoError(t, err)
		release, err := pluginoci.Inspect(index, pluginIndexDigest)
		require.NoError(t, err)
		assert.Equal(t, pluginIndexDigest, release.Digest)
		assert.Equal(t, "example/fixture", release.Name)
		assert.Equal(t, "1.2.3", release.Version)
		require.Len(t, release.Platforms, 2)
		assert.Equal(t, pluginManifestDigestLinuxAMD64, release.Platforms["linux/amd64"].Digest.String())
		assert.Equal(t, pluginManifestDigestDarwinARM64, release.Platforms["darwin/arm64"].Digest.String())
		assert.Equal(t, pluginDefinitionDigest, release.Platforms["linux/amd64"].Annotations["dev.atmos.plugin.manifest.digest"])
	})

	t.Run("platform payloads extract to the original executables", func(t *testing.T) {
		index, err := atmosoci.LoadIndex(root, pluginIndexDigest)
		require.NoError(t, err)
		release, err := pluginoci.Inspect(index, pluginIndexDigest)
		require.NoError(t, err)
		for platform, want := range map[string][]byte{
			"linux/amd64":  ocitest.Binary("linux", "amd64"),
			"darwin/arm64": ocitest.Binary("darwin", "arm64"),
		} {
			destination := t.TempDir()
			require.NoError(t, release.Extract(platform, destination), platform)
			got, err := os.ReadFile(filepath.Join(destination, "bin", "plugin"))
			require.NoError(t, err)
			assert.Equal(t, want, got, platform)
		}
	})
}

func TestPluginReleaseRebuildMatchesFixture(t *testing.T) {
	rebuilt := filepath.Join(t.TempDir(), "layout")
	packPlugin(t, "1.2.3", rebuilt)

	got := treeBytes(t, rebuilt)
	want := treeBytes(t, pluginFixtureDir())
	require.Positive(t, len(want))
	require.Equal(t, len(want), len(got), "layout file set changed")
	for name, content := range want {
		assert.Equal(t, content, got[name], "%s differs from what the current plugin packer produces", name)
	}
}

// TestPluginDigestChangeIsDetected proves the constants above would fail if the packer's output
// changed: a one-field change in the packed release moves the index digest.
func TestPluginDigestChangeIsDetected(t *testing.T) {
	changed := filepath.Join(t.TempDir(), "layout")
	packPlugin(t, "1.2.4", changed)

	index, err := atmosoci.LoadIndex(changed, "")
	require.NoError(t, err)
	digest, err := index.Digest()
	require.NoError(t, err)
	assert.NotEqual(t, pluginIndexDigest, digest.String())
}
