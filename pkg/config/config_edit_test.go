package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEditableConfigFile_Override(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "custom.yaml")
	require.NoError(t, os.WriteFile(file, []byte("a: 1\n"), 0o644))

	got, err := ResolveEditableConfigFile(nil, file)
	require.NoError(t, err)
	assert.Equal(t, file, got)
}

func TestResolveEditableConfigFile_OverrideDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(file, []byte("a: 1\n"), 0o644))

	got, err := ResolveEditableConfigFile(nil, dir)
	require.NoError(t, err)
	assert.Equal(t, file, got)
}

func TestResolveEditableConfigFile_OverrideMissing(t *testing.T) {
	_, err := ResolveEditableConfigFile(nil, filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoEditableConfig)
}

func TestResolveEditableConfigFile_PrefersAtmosYamlOverDotfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, AtmosConfigFileName), []byte("a: 1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, DotAtmosConfigFileName), []byte("a: 2\n"), 0o644))

	got, ok, err := firstExistingConfig(dir)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, filepath.Join(dir, AtmosConfigFileName), got)
}

// TestResolveEditableConfigFile_NonMissingStatErrorPropagates verifies that a
// stat error other than "not found" surfaces instead of collapsing into
// ErrNoEditableConfig. Using a regular file as a directory component yields
// ENOTDIR on Unix, which os.IsNotExist does not match.
func TestResolveEditableConfigFile_NonMissingStatErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a file-as-directory probe as path-not-found, which os.IsNotExist treats as missing")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))

	override := filepath.Join(file, AtmosConfigFileName)
	_, err := ResolveEditableConfigFile(nil, override)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoEditableConfig, "ENOTDIR must not be reported as missing config")
}

func TestResolveEditableConfigFile_CurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(file, []byte("a: 1\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))

	got, err := ResolveEditableConfigFile(nil, "")
	require.NoError(t, err)
	// Resolve symlinks for macOS /var -> /private/var.
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(file)
	assert.Equal(t, wantResolved, gotResolved)
}
