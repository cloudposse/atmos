package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestModule creates a minimal, dependency-free module fixture in dir
// so `go list -m` has something real to inspect, without depending on this
// repository's own (much larger) module graph.
func writeTestModule(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/noticegentest\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644))
}

func TestGoListModuleVersionResolvesSelf(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	version, err := goListModuleVersion(dir, defaultLicenseEnv())("example.com/noticegentest")

	require.NoError(t, err)
	assert.Equal(t, "", version, "the main module itself reports an empty version from `go list -m`")
}

func TestGoListModuleVersionPropagatesError(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	_, err := goListModuleVersion(dir, defaultLicenseEnv())("example.com/does-not-exist")

	require.Error(t, err)
}

func TestGoListAllGoogleCloudGoModulesFiltersToRelevantModules(t *testing.T) {
	dir := t.TempDir()
	writeTestModule(t, dir)

	modules, err := goListAllGoogleCloudGoModules(dir, defaultLicenseEnv())()

	require.NoError(t, err)
	assert.Empty(t, modules, "a module with no cloud.google.com/go dependency must yield no modules")
}
