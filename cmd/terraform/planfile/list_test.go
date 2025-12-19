package planfile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/planfile"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// initTestIO initializes the I/O context for tests that use data or ui packages.
func initTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
}

func TestFormatListOutput(t *testing.T) {
	initTestIO(t)

	files := []planfile.PlanfileInfo{
		{
			Key:          "stack1/component1/sha1.tfplan",
			Size:         1024,
			LastModified: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			Key:          "stack2/component2/sha2.tfplan",
			Size:         2048,
			LastModified: time.Date(2024, 1, 16, 11, 45, 0, 0, time.UTC),
		},
	}

	t.Run("table format", func(t *testing.T) {
		err := formatListOutput(files, "table")
		assert.NoError(t, err)
	})

	t.Run("json format", func(t *testing.T) {
		err := formatListOutput(files, "json")
		assert.NoError(t, err)
	})

	t.Run("yaml format", func(t *testing.T) {
		err := formatListOutput(files, "yaml")
		assert.NoError(t, err)
	})

	t.Run("unknown format defaults to table", func(t *testing.T) {
		err := formatListOutput(files, "unknown")
		assert.NoError(t, err)
	})
}

func TestFormatListTable(t *testing.T) {
	initTestIO(t)

	t.Run("empty list", func(t *testing.T) {
		// Should not panic with empty list.
		assert.NotPanics(t, func() {
			formatListTable([]planfile.PlanfileInfo{})
		})
	})

	t.Run("with files", func(t *testing.T) {
		files := []planfile.PlanfileInfo{
			{
				Key:          "key1.tfplan",
				Size:         1024,
				LastModified: time.Now(),
			},
		}
		assert.NotPanics(t, func() {
			formatListTable(files)
		})
	})
}

func TestFormatListJSON(t *testing.T) {
	initTestIO(t)

	t.Run("empty list", func(t *testing.T) {
		err := formatListJSON([]planfile.PlanfileInfo{})
		assert.NoError(t, err)
	})

	t.Run("with files", func(t *testing.T) {
		files := []planfile.PlanfileInfo{
			{
				Key:          "key1.tfplan",
				Size:         1024,
				LastModified: time.Now(),
				Metadata: &planfile.Metadata{
					Stack:     "test-stack",
					Component: "test-component",
				},
			},
		}
		err := formatListJSON(files)
		assert.NoError(t, err)
	})
}

func TestFormatListYAML(t *testing.T) {
	initTestIO(t)

	t.Run("empty list", func(t *testing.T) {
		// Should not panic with empty list.
		assert.NotPanics(t, func() {
			formatListYAML([]planfile.PlanfileInfo{})
		})
	})

	t.Run("with files without metadata", func(t *testing.T) {
		files := []planfile.PlanfileInfo{
			{
				Key:          "key1.tfplan",
				Size:         1024,
				LastModified: time.Now(),
			},
		}
		assert.NotPanics(t, func() {
			formatListYAML(files)
		})
	})

	t.Run("with files with metadata", func(t *testing.T) {
		files := []planfile.PlanfileInfo{
			{
				Key:          "key1.tfplan",
				Size:         1024,
				LastModified: time.Now(),
				Metadata: &planfile.Metadata{
					Stack:     "test-stack",
					Component: "test-component",
					SHA:       "abc123",
				},
			},
		}
		assert.NotPanics(t, func() {
			formatListYAML(files)
		})
	})
}

// Note: TestTableHeaderWidth and TestPlanfileInfoSorting were removed as tautological.
// - TestTableHeaderWidth asserted a constant equals a hardcoded value
// - TestPlanfileInfoSorting only verified slice length, not actual sorting
// Per coding guidelines: "Test behavior, not implementation; avoid tautological tests."

// Note: Testing runList requires extensive mocking of:
// - Atmos config loading
// - Store creation
// - Store.List() calls
//
// The helper functions above provide coverage for the formatting logic.
// Full integration tests would run against actual stores.

func TestOutputFormats(t *testing.T) {
	initTestIO(t)

	// Verify all output format options work without panicking.
	files := []planfile.PlanfileInfo{
		{
			Key:          "test/key.tfplan",
			Size:         512,
			LastModified: time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC),
			Metadata: &planfile.Metadata{
				Stack:      "test-stack",
				Component:  "test-component",
				SHA:        "abc123def456",
				HasChanges: true,
				Additions:  5,
				Changes:    3,
			},
		},
	}

	formats := []string{"table", "json", "yaml", ""}
	for _, format := range formats {
		t.Run("format_"+format, func(t *testing.T) {
			assert.NotPanics(t, func() {
				_ = formatListOutput(files, format)
			})
		})
	}
}
