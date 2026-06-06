package cache

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashFiles_Deterministic(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0o644))

	h1 := hashFiles(dir, []string{"*.txt"})
	h2 := hashFiles(dir, []string{"*.txt"})
	assert.Equal(t, h1, h2, "hash must be stable for the same inputs")
	assert.NotEqual(t, "no-files", h1)

	// Changing content changes the hash.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644))
	assert.NotEqual(t, h1, hashFiles(dir, []string{"*.txt"}))
}

func TestHashFiles_NoMatches(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, "no-files", hashFiles(dir, []string{"*.nope"}))
	assert.Equal(t, "no-files", hashFiles(dir, nil))
}

func TestRenderKey_Template(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lock.yaml"), []byte("v1"), 0o644))

	key, err := renderKey(`atmos-{{.OS}}-{{.Arch}}-{{ hashFiles "lock.yaml" }}`, dir)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(key, "atmos-"+runtime.GOOS+"-"+runtime.GOARCH+"-"))
	assert.NotContains(t, key, "no-files")
}

func TestRenderKey_InvalidTemplate(t *testing.T) {
	_, err := renderKey("{{ .Nope }}", t.TempDir())
	require.Error(t, err)
}

func TestDefaultKey_StableAndContentSensitive(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "toolchain.lock.yaml")
	require.NoError(t, os.WriteFile(lock, []byte("tools: {}"), 0o644))

	k1 := defaultKey(lock)
	assert.Equal(t, k1, defaultKey(lock))
	assert.True(t, strings.HasPrefix(k1, defaultKeyPrefix))

	require.NoError(t, os.WriteFile(lock, []byte("tools: {terraform: 1}"), 0o644))
	assert.NotEqual(t, k1, defaultKey(lock))
}

func TestDefaultKey_MissingLockfile(t *testing.T) {
	k := defaultKey(filepath.Join(t.TempDir(), "absent.yaml"))
	assert.True(t, strings.HasSuffix(k, "no-lock"))
}

func TestDefaultRestoreKey_IsPrefixOfDefaultKey(t *testing.T) {
	prefix := defaultRestoreKey()
	key := defaultKey(filepath.Join(t.TempDir(), "absent.yaml"))
	assert.True(t, strings.HasPrefix(key, prefix))
}
