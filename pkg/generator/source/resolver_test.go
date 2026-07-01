package source

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

const sampleScaffold = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: sample
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

// writeSampleTemplate creates a minimal on-disk scaffold template and returns its directory.
func writeSampleTemplate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(sampleScaffold), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o600))
	return dir
}

func hasSampleFile(files []templates.File) bool {
	for _, f := range files {
		if f.Path == "file.txt" {
			return true
		}
	}
	return false
}

func TestResolve_LocalPath(t *testing.T) {
	dir := writeSampleTemplate(t)

	cfg, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "sample", dir, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()
	require.NotNil(t, cfg)
	assert.True(t, hasSampleFile(cfg.Files), "local template files must be loaded")
}

func TestResolve_LocalPathDefaultTimeout(t *testing.T) {
	dir := writeSampleTemplate(t)

	cfg, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "sample", dir, 0)
	require.NoError(t, err)
	defer cleanup()
	require.NotNil(t, cfg)
	assert.True(t, hasSampleFile(cfg.Files))
}

func TestResolve_FileURI(t *testing.T) {
	dir := writeSampleTemplate(t)

	cfg, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "sample", "file://"+dir, time.Minute)
	require.NoError(t, err)
	defer cleanup()
	require.NotNil(t, cfg)
	assert.True(t, hasSampleFile(cfg.Files))
}

func TestResolve_BadLocalPathReturnsLoadError(t *testing.T) {
	_, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "missing", filepath.Join(t.TempDir(), "missing"), time.Minute)
	require.Error(t, err)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestResolve_OCIUnsupported(t *testing.T) {
	_, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "x", "oci://ghcr.io/cloudposse/x:latest", time.Minute)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldSourceUnsupported)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestResolve_RemoteFetchFailureCleansUp(t *testing.T) {
	_, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "x", "git::file:///definitely/not/a/repo", time.Millisecond)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldFetchSource)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestHydrate_NoopForFullConfig(t *testing.T) {
	stub := &templates.Configuration{Name: "x", Files: []templates.File{{Path: "a"}}}
	cleanup, err := Hydrate(stub, "")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	cleanup()
	assert.Len(t, stub.Files, 1, "full configs are returned unchanged")
}

func TestHydrate_NoopForEmptySource(t *testing.T) {
	stub := &templates.Configuration{Name: "x"}
	cleanup, err := Hydrate(stub, "")
	require.NoError(t, err)
	cleanup()
	assert.Empty(t, stub.Files)
}

func TestHydrate_LocalStub(t *testing.T) {
	dir := writeSampleTemplate(t)

	stub := &templates.Configuration{Name: "sample", Source: dir}
	cleanup, err := Hydrate(stub, "")
	require.NoError(t, err)
	defer cleanup()
	assert.True(t, hasSampleFile(stub.Files), "local stub must be hydrated from its source")
}

func TestHydrate_LocalStubError(t *testing.T) {
	stub := &templates.Configuration{Name: "missing", Source: filepath.Join(t.TempDir(), "missing")}

	cleanup, err := Hydrate(stub, "")
	require.Error(t, err)
	require.NotNil(t, cleanup)
	cleanup()
}
