package cache

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/ui"
)

// useCacheRoot points the cache subcommands at root for the duration of the test.
func useCacheRoot(t *testing.T, root string) {
	t.Helper()
	orig := resolveCacheRoot
	t.Cleanup(func() { resolveCacheRoot = orig })
	resolveCacheRoot = func(*cobra.Command) (string, error) { return root, nil }
}

// initDataWriter wires the data package so the json/yaml output paths can render.
func initDataWriter(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
}

// seedCacheObject writes a cached object plus its metadata sidecar under root.
func seedCacheObject(t *testing.T, root, key string, size int, kind string) {
	t.Helper()
	objPath := filepath.Join(root, filepath.FromSlash(key))
	require.NoError(t, os.MkdirAll(filepath.Dir(objPath), 0o755))
	require.NoError(t, os.WriteFile(objPath, make([]byte, size), 0o644))
	sc := map[string]any{
		"sha256":     "deadbeef",
		"created_at": time.Now().Format(time.RFC3339),
		"custom": map[string]any{
			"kind":       kind,
			"fetched_at": time.Now().Format(time.RFC3339),
		},
	}
	b, err := json.MarshalIndent(sc, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(objPath+".metadata.json", b, 0o644))
}

func tfcacheSummaryForTest(root string) tfcache.Summary {
	return tfcache.Summary{
		Root:        root,
		ObjectCount: 2,
		Providers:   1,
		Modules:     1,
		TotalSize:   4096,
		Largest:     &tfcache.Entry{Key: "providers/registry.terraform.io/hashicorp/aws/index.json", Size: 2048},
	}
}

func captureCacheStderr(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stderr
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = old })

	fn()

	require.NoError(t, writer.Close())
	os.Stderr = old

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, reader)
	require.NoError(t, err)
	return buffer.String()
}

func TestListCmd(t *testing.T) {
	initDataWriter(t)
	root := t.TempDir()
	seedCacheObject(t, root, "providers/registry.terraform.io/hashicorp/aws/index.json", 100, "metadata")
	seedCacheObject(t, root, "modules/registry.terraform.io/cloudposse/vpc/aws/versions.json", 200, "metadata")
	useCacheRoot(t, root)

	for _, format := range []string{"table", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			cacheCmd.SetArgs([]string{"list", "--format", format})
			require.NoError(t, cacheCmd.Execute())
		})
	}
}

func TestListCmd_Empty(t *testing.T) {
	useCacheRoot(t, t.TempDir())
	cacheCmd.SetArgs([]string{"list", "--format", "table"})
	require.NoError(t, cacheCmd.Execute())
}

func TestStatsCmd(t *testing.T) {
	initDataWriter(t)
	root := t.TempDir()
	seedCacheObject(t, root, "providers/registry.terraform.io/hashicorp/aws/index.json", 100, "metadata")
	seedCacheObject(t, root, "providers/registry.terraform.io/hashicorp/aws/aws_5.95.0_linux_amd64.zip", 5000, "artifact")
	useCacheRoot(t, root)

	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			cacheCmd.SetArgs([]string{"stats", "--format", format})
			require.NoError(t, cacheCmd.Execute())
		})
	}
}

func TestPrintStatsUsesFormattedTable(t *testing.T) {
	initDataWriter(t)
	output := captureCacheStderr(t, func() {
		printStats(tfcacheSummaryForTest(t.TempDir()))
	})

	assert.Contains(t, output, "METRIC")
	assert.Contains(t, output, "VALUE")
	assert.Contains(t, output, "Registry cache root")
	assert.Contains(t, output, "Objects")
	assert.NotContains(t, output, "Registry cache root:")
}

func TestPruneCmd(t *testing.T) {
	root := t.TempDir()
	// A metadata object old enough to be pruned by the default window.
	seedCacheObject(t, root, "providers/registry.terraform.io/hashicorp/aws/index.json", 100, "metadata")
	useCacheRoot(t, root)

	t.Run("dry run", func(t *testing.T) {
		cacheCmd.SetArgs([]string{"prune", "--older-than", "0s", "--dry-run"})
		require.NoError(t, cacheCmd.Execute())
		// Dry run leaves the object in place.
		_, err := os.Stat(filepath.Join(root, "providers", "registry.terraform.io", "hashicorp", "aws", "index.json"))
		require.NoError(t, err)
	})

	t.Run("invalid duration", func(t *testing.T) {
		cacheCmd.SetArgs([]string{"prune", "--older-than", "not-a-duration"})
		require.Error(t, cacheCmd.Execute())
	})
}

func TestDeleteCmd(t *testing.T) {
	root := t.TempDir()
	key := "providers/registry.terraform.io/hashicorp/aws/index.json"
	seedCacheObject(t, root, key, 100, "metadata")
	useCacheRoot(t, root)

	cacheCmd.SetArgs([]string{"delete", key})
	require.NoError(t, cacheCmd.Execute())

	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(key)))
	assert.True(t, os.IsNotExist(err), "the object is removed")
}

func TestDeleteCmd_Args(t *testing.T) {
	assert.Error(t, deleteCmd.Args(deleteCmd, []string{}), "delete requires exactly one key")
	assert.NoError(t, deleteCmd.Args(deleteCmd, []string{"some/key"}))
	assert.Error(t, deleteCmd.Args(deleteCmd, []string{"a", "b"}))
}
