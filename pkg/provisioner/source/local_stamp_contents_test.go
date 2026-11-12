package source

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func replaceStampContents(t *testing.T, path string) {
	t.Helper()
	before, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("# two"), before.Mode()))
	require.NoError(t, os.Chtimes(path, before.ModTime(), before.ModTime()))
	after, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, before.Size(), after.Size())
	require.Equal(t, before.Mode(), after.Mode())
	require.Equal(t, before.ModTime(), after.ModTime())
}

func TestLocalSourceStampEqualMetadataContents(t *testing.T) {
	for _, kind := range []string{"regular", "file link", "directory link"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "main.tf")
			if kind != "regular" {
				path = filepath.Join(root, ".terraform", "main.tf")
			}
			writeStampFile(t, path, "# one")
			switch kind {
			case "file link":
				require.NoError(t, os.Symlink(path, filepath.Join(root, "main.tf")))
			case "directory link":
				require.NoError(t, os.Symlink(filepath.Dir(path), filepath.Join(root, "module")))
			}
			spec := &schema.VendorComponentSource{Uri: root}
			before := localSourceStamp(spec)
			require.NotEmpty(t, before)
			require.Equal(t, before, localSourceStamp(spec))
			replaceStampContents(t, path)
			after := localSourceStamp(spec)
			require.NotEmpty(t, after)
			assert.NotEqual(t, before, after)
			assert.Equal(t, after, localSourceStamp(spec))
			changed, reason := localSourceChanged(&workdir.WorkdirMetadata{SourceStamp: before}, spec)
			assert.True(t, changed)
			assert.Contains(t, reason, "Local source changed")
		})
	}
}

func TestLocalSourceStampExcludedContents(t *testing.T) {
	for _, kind := range []string{"excluded file", "include filter", "runtime directory", "external link"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "main.tf")
			spec := &schema.VendorComponentSource{Uri: root}
			switch kind {
			case "excluded file":
				spec.ExcludedPaths = []string{"main.tf"}
			case "include filter":
				spec.IncludedPaths = []string{"**/*.txt"}
			case "runtime directory":
				path = filepath.Join(root, ".terraform", "main.tf")
			case "external link":
				path = filepath.Join(t.TempDir(), "main.tf")
				require.NoError(t, os.Symlink(path, filepath.Join(root, "main.tf")))
			}
			writeStampFile(t, path, "# one")
			before := localSourceStamp(spec)
			require.NotEmpty(t, before)
			replaceStampContents(t, path)
			assert.Equal(t, before, localSourceStamp(spec))
		})
	}
}

func TestStampFileContentsReadErrors(t *testing.T) {
	// A directory opens successfully but cannot be read as file contents.
	assert.Error(t, stampFileContents(io.Discard, t.TempDir()))
	assert.Error(t, stampFileContents(io.Discard, filepath.Join(t.TempDir(), "missing")))
}
