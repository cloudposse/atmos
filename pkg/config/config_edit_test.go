package config

import (
	"os"
	"path/filepath"
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

	got, ok := firstExistingConfig(dir)
	require.True(t, ok)
	assert.Equal(t, filepath.Join(dir, AtmosConfigFileName), got)
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
