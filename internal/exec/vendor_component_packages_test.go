package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveFileURIPath_NotFileScheme proves resolveFileURIPath only handles "file://" URIs,
// leaving every other scheme (or a bare path) untouched.
func TestResolveFileURIPath_NotFileScheme(t *testing.T) {
	for _, uri := range []string{
		"github.com/cloudposse/terraform-null-label.git",
		"https://example.com/foo",
		"/already/absolute/path",
		"relative/path",
	} {
		_, ok := resolveFileURIPath(uri)
		assert.False(t, ok, "uri %q must not be treated as a file:// scheme", uri)
	}
}

// TestResolveFileURIPath_PreservesAbsoluteRoot proves a "file://" URI resolves to the absolute
// path it names, instead of being trimmed down to a path relative to the current directory.
// Regression: "file:///tmp/source" previously resolved to the relative "tmp/source" because the
// leading "/" was unconditionally stripped.
func TestResolveFileURIPath_PreservesAbsoluteRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX absolute-path case only applies on non-Windows platforms")
	}

	resolved, ok := resolveFileURIPath("file:///tmp/source")
	require.True(t, ok)
	assert.Equal(t, "/tmp/source", resolved)
}

// TestHandleLocalFileScheme_FileURI_RecomputesSourceIsLocalFile proves handleLocalFileScheme
// recognizes an existing file named by a "file://" URI (sourceIsLocalFile), not just an existing
// file named by a plain path resolved against componentPath.
func TestHandleLocalFileScheme_FileURI_RecomputesSourceIsLocalFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX absolute-path case only applies on non-Windows platforms")
	}

	tempDir := t.TempDir()
	sourceFile := filepath.Join(tempDir, "source")
	require.NoError(t, os.WriteFile(sourceFile, []byte("test"), 0o644))

	uri, useLocalFileSystem, sourceIsLocalFile := handleLocalFileScheme(t.TempDir(), "file://"+sourceFile)

	assert.Equal(t, sourceFile, uri)
	assert.True(t, useLocalFileSystem)
	assert.True(t, sourceIsLocalFile, "sourceIsLocalFile must be recomputed for the resolved file:// path")
}

// TestProcessComponentMixins_FileURI_NormalizesToAbsolutePath proves a mixin declared with a
// "file://" URI resolves to the absolute path it names (shared with handleLocalFileScheme via
// resolveFileURIPath), instead of a broken relative path.
func TestProcessComponentMixins_FileURI_NormalizesToAbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX absolute-path case only applies on non-Windows platforms")
	}

	// A path that does not exist locally, so processComponentMixins does not skip it via the
	// already-materialized dedup check.
	missingSource := filepath.Join(t.TempDir(), "does-not-exist")

	spec := &schema.VendorComponentSpec{
		Mixins: []schema.VendorComponentMixins{
			{Uri: "file://" + missingSource, Filename: "context.tf"},
		},
	}

	packages, err := processComponentMixins(nil, spec, t.TempDir())
	require.NoError(t, err)
	require.Len(t, packages, 1)
	assert.Equal(t, missingSource, packages[0].uri)
}
