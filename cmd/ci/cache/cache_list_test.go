package cache

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cachepkg "github.com/cloudposse/atmos/pkg/ci/cache"
)

func sampleEntries() []cachepkg.Entry {
	return []cachepkg.Entry{
		{Key: "atmos-a", Size: 10, CreatedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC), ID: "1"},
		{Key: "atmos-b", Size: 20, ID: "2"},
	}
}

func TestFormatCreatedAt(t *testing.T) {
	assert.Equal(t, "", formatCreatedAt(time.Time{}))
	got := formatCreatedAt(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))
	assert.Equal(t, "2024-01-02 03:04:05", got)
}

func TestBuildCacheTableRows(t *testing.T) {
	rows := buildCacheTableRows(sampleEntries())
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"atmos-a", "10", "2024-01-02 03:04:05"}, rows[0])
	assert.Equal(t, []string{"atmos-b", "20", ""}, rows[1])
}

func TestBuildCacheFullRows(t *testing.T) {
	rows := buildCacheFullRows(sampleEntries())
	require.Len(t, rows, 2)
	assert.Equal(t, "atmos-a", rows[0]["key"])
	assert.Equal(t, int64(10), rows[0]["size"])
	assert.Equal(t, "1", rows[0]["id"])
	assert.Equal(t, "atmos-b", rows[1]["key"])
	assert.Equal(t, "", rows[1]["created"])
}

func TestBuildCacheDataMap(t *testing.T) {
	rows := buildCacheFullRows(sampleEntries())
	m := buildCacheDataMap(rows)
	require.Len(t, m, 2)
	assert.Contains(t, m, "atmos-a")
	assert.Contains(t, m, "atmos-b")

	// Rows without a key fall back to an index-based key.
	fallback := buildCacheDataMap([]map[string]interface{}{{"size": int64(1)}})
	assert.Contains(t, fallback, "entry_0")
}

func TestBuildCacheCSVTSV(t *testing.T) {
	out := buildCacheCSVTSV(sampleEntries(), ",")
	lines := strings.Split(strings.TrimRight(out, "\r\n"), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, "Key,Size,Created", strings.TrimRight(lines[0], "\r"))
	assert.Contains(t, lines[1], "atmos-a,10,2024-01-02 03:04:05")
	assert.Contains(t, lines[2], "atmos-b,20,")
}

func TestFormatCacheOutput(t *testing.T) {
	entries := sampleEntries()

	t.Run("json", func(t *testing.T) {
		out, err := formatCacheOutput(entries, "json", "", 0)
		require.NoError(t, err)
		assert.Contains(t, out, "atmos-a")
		assert.Contains(t, out, "atmos-b")
	})

	t.Run("yaml", func(t *testing.T) {
		out, err := formatCacheOutput(entries, "yaml", "", 0)
		require.NoError(t, err)
		assert.Contains(t, out, "atmos-a")
	})

	t.Run("csv", func(t *testing.T) {
		out, err := formatCacheOutput(entries, "csv", "", 0)
		require.NoError(t, err)
		assert.Contains(t, out, "Key,Size,Created")
		assert.Contains(t, out, "atmos-a,10")
	})

	t.Run("tsv", func(t *testing.T) {
		out, err := formatCacheOutput(entries, "tsv", "", 0)
		require.NoError(t, err)
		assert.Contains(t, out, "atmos-a\t10")
	})

	t.Run("table non-tty falls back to tsv", func(t *testing.T) {
		// In the test environment stdout is not a TTY, so table → plain TSV.
		out, err := formatCacheOutput(entries, "table", "", 0)
		require.NoError(t, err)
		assert.Contains(t, out, "atmos-a")
		assert.Contains(t, out, "atmos-b")
	})
}
