package discovery

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/registryauth"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// These tests characterize the plugin discovery cache's on-disk contract independently of the
// implementation: the directory, the file name derivation, the scope recipe, the serialized
// field order, the file modes, the default TTL, and the readability of a hand-written snapshot.
// They were written before the cache mechanics moved to the shared package so that the move can
// be proved byte-compatible. They deliberately re-derive every value from the documented recipe
// rather than calling the cache code.

// characterizedScope re-derives the scope hash: sha256 over the JSON of the auth configuration,
// the realm value, and the host bindings, never any credential value.
func characterizedScope(t *testing.T, f *fixture) string {
	t.Helper()
	info, err := realm.GetRealm(f.service.Config.Auth.Realm, f.service.Config.CliConfigPath)
	require.NoError(t, err)
	bindings, err := registryauth.PluginBindings(f.service.Config)
	require.NoError(t, err)
	raw, err := json.Marshal([]any{f.service.Config.Auth, info.Value, bindings})
	require.NoError(t, err)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// characterizedFile re-derives the snapshot path of a source.
func characterizedFile(t *testing.T, f *fixture, kind, reference string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(characterizedScope(t, f) + "\x00" + kind + "\x00" + reference))
	return filepath.Join(f.service.Config.Plugins.CacheDir, "discovery", hex.EncodeToString(sum[:])+".json")
}

func TestCacheCharacterizationFileNameAndSerialization(t *testing.T) {
	f := newFixture(t)
	release := f.publish(t, "1.0.0", "Original")
	before := time.Now().UTC().Add(-time.Second)
	_, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)

	path := characterizedFile(t, f, "repository", "oci://"+f.repository.Name())
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "the snapshot must live at <cache>/discovery/<sha256(scope NUL kind NUL source)>.json")

	// Top-level keys keep their historical order.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), token)
	var keys []string
	for decoder.More() {
		key, keyErr := decoder.Token()
		require.NoError(t, keyErr)
		keys = append(keys, key.(string))
		var skipped json.RawMessage
		require.NoError(t, decoder.Decode(&skipped))
	}
	require.Equal(t, []string{"version", "source", "kind", "fetched_at", "entries", "releases"}, keys)

	var value snapshot
	require.NoError(t, json.Unmarshal(raw, &value))
	require.Equal(t, 1, value.Version)
	require.Equal(t, "oci://"+f.repository.Name(), value.Source)
	require.Equal(t, "repository", value.Kind)
	require.False(t, value.FetchedAt.Before(before), "fetched_at records the refresh time")
	require.Equal(t, time.UTC, value.FetchedAt.Location())
	require.Len(t, value.Entries, 1)
	require.Equal(t, release.Digest, value.Entries[0].Versions[0].Digest)
	require.Contains(t, value.Releases, release.Digest)

	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		dir, statErr := os.Stat(filepath.Dir(path))
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o700), dir.Mode().Perm())
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.json"))
	require.NoError(t, err)
	require.Equal(t, []string{path}, files, "one snapshot per source and no stray temporary files")
}

func TestCacheCharacterizationScopeSeparatesRealmsAndKeepsCredentialsOut(t *testing.T) {
	f := newFixture(t)
	f.publish(t, "1.0.0", "Original")
	_, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	first := characterizedFile(t, f, "repository", "oci://"+f.repository.Name())
	require.FileExists(t, first)

	f.service.Config.Auth.Realm = "other-realm"
	other := characterizedFile(t, f, "repository", "oci://"+f.repository.Name())
	require.NotEqual(t, first, other, "a different auth realm changes the cache file")
	_, err = f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.FileExists(t, other)

	for _, path := range []string{first, other} {
		raw, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.NotContains(t, string(raw), "other-realm", "the snapshot never embeds the scope inputs")
	}
}

func TestCacheCharacterizationDefaultDirectoryTTLAndLegacySnapshot(t *testing.T) {
	f := newFixture(t)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("ATMOS_XDG_CACHE_HOME", "")
	f.service.Config.Plugins.CacheDir = ""
	release := f.publish(t, "1.0.0", "Original")
	_, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)

	directory := xdg.LookupXDGCacheDir("plugins/discovery")
	scope := characterizedScope(t, f)
	sum := sha256.Sum256([]byte(scope + "\x00repository\x00oci://" + f.repository.Name()))
	path := filepath.Join(directory, hex.EncodeToString(sum[:])+".json")
	require.FileExists(t, path, "the default location is the XDG cache plugins/discovery directory")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var written snapshot
	require.NoError(t, json.Unmarshal(raw, &written))
	// Rewrite the snapshot as a legacy writer would have: only the historical keys.
	legacy := map[string]any{
		"version": 1, "source": written.Source, "kind": written.Kind,
		"fetched_at": time.Now().UTC().Add(-14 * time.Minute).Format(time.RFC3339Nano),
		"entries":    written.Entries, "releases": written.Releases,
	}
	encoded, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0o600))

	// Fourteen minutes old is inside the fifteen minute default: no registry request.
	calls := f.requests.Load()
	results, err := f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Equal(t, calls, f.requests.Load(), "a fresh legacy snapshot is served without contacting the registry")
	require.Equal(t, release.Digest, results[0].Versions[0].Digest)

	// Sixteen minutes old is stale: the registry is contacted again and the file is replaced.
	legacy["fetched_at"] = time.Now().UTC().Add(-16 * time.Minute).Format(time.RFC3339Nano)
	encoded, err = json.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, encoded, 0o600))
	_, err = f.service.Search(t.Context(), Request{})
	require.NoError(t, err)
	require.Greater(t, f.requests.Load(), calls, "a stale snapshot is refreshed")
	refreshed, err := os.ReadFile(path)
	require.NoError(t, err)
	var after snapshot
	require.NoError(t, json.Unmarshal(refreshed, &after))
	require.WithinDuration(t, time.Now(), after.FetchedAt, time.Minute)
}

func TestCacheCharacterizationCatalogSnapshotKeepsDigest(t *testing.T) {
	f := newFixture(t)
	release := f.publish(t, "1.0.0", "Original")
	reference := "oci://" + f.server.Listener.Addr().String() + "/product/catalog:stable"
	file := catalog.New("product")
	require.NoError(t, catalog.Add(file, "oci://"+f.repository.Name(), release, f.service.Descriptors))
	require.NoError(t, catalog.Publish(t.Context(), f.service.Registry, reference, file, ""))
	f.service.Config.Plugins.Registries["product"] = schema.PluginRegistry{Catalog: reference}
	_, err := f.service.Search(t.Context(), Request{Registry: "product"})
	require.NoError(t, err)

	raw, err := os.ReadFile(characterizedFile(t, f, "catalog", reference))
	require.NoError(t, err)
	var value snapshot
	require.NoError(t, json.Unmarshal(raw, &value))
	require.Equal(t, "catalog", value.Kind)
	require.Equal(t, reference, value.Source)
	require.NotEmpty(t, value.Digest, "a catalog snapshot records the catalog manifest digest")
	require.Empty(t, value.Releases)
}
