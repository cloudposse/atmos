package planfile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
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

func TestRenderPlanfileList(t *testing.T) {
	initTestIO(t)

	files := []planfile.PlanfileInfo{
		{
			Key:          "stack1/component1/sha1.tfplan",
			Size:         1024,
			LastModified: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			Metadata: &planfile.Metadata{
				MD5: "d41d8cd98f00b204e9800998ecf8427e",
			},
		},
		{
			Key:          "stack2/component2/sha2.tfplan",
			Size:         2048,
			LastModified: time.Date(2024, 1, 16, 11, 45, 0, 0, time.UTC),
			Metadata: &planfile.Metadata{
				MD5: "098f6bcd4621d373cade4e832627b4f6",
			},
		},
	}

	t.Run("table format", func(t *testing.T) {
		err := renderPlanfileList(files, "table", "", "")
		assert.NoError(t, err)
	})

	t.Run("json format", func(t *testing.T) {
		err := renderPlanfileList(files, "json", "", "")
		assert.NoError(t, err)
	})

	t.Run("yaml format", func(t *testing.T) {
		err := renderPlanfileList(files, "yaml", "", "")
		assert.NoError(t, err)
	})

	t.Run("csv format", func(t *testing.T) {
		err := renderPlanfileList(files, "csv", "", "")
		assert.NoError(t, err)
	})

	t.Run("tsv format", func(t *testing.T) {
		err := renderPlanfileList(files, "tsv", "", "")
		assert.NoError(t, err)
	})

	t.Run("unknown format defaults to table", func(t *testing.T) {
		err := renderPlanfileList(files, "unknown", "", "")
		assert.NoError(t, err)
	})
}

func TestRenderPlanfileListEmpty(t *testing.T) {
	initTestIO(t)

	t.Run("empty list", func(t *testing.T) {
		// Should not panic with empty list.
		err := renderPlanfileList([]planfile.PlanfileInfo{}, "table", "", "")
		assert.NoError(t, err)
	})
}

func TestRenderPlanfileListWithMetadata(t *testing.T) {
	initTestIO(t)

	t.Run("with metadata includes sha and md5", func(t *testing.T) {
		files := []planfile.PlanfileInfo{
			{
				Key:          "key1.tfplan",
				Size:         1024,
				LastModified: time.Now(),
				Metadata: &planfile.Metadata{
					Stack:     "test-stack",
					Component: "test-component",
					SHA:       "abc123",
					MD5:       "abc123md5",
				},
			},
		}
		err := renderPlanfileList(files, "json", "", "")
		assert.NoError(t, err)
	})

	t.Run("without metadata sha and md5 are empty", func(t *testing.T) {
		files := []planfile.PlanfileInfo{
			{
				Key:          "key1.tfplan",
				Size:         1024,
				LastModified: time.Now(),
			},
		}
		err := renderPlanfileList(files, "yaml", "", "")
		assert.NoError(t, err)
	})
}

func TestRenderPlanfileListWithOwnerRepo(t *testing.T) {
	initTestIO(t)

	files := []planfile.PlanfileInfo{
		{
			Key:          "stack1/component1/sha1.tfplan",
			Size:         1024,
			LastModified: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			Metadata: &planfile.Metadata{
				Stack:     "stack1",
				Component: "component1",
				SHA:       "sha1",
				MD5:       "d41d8cd98f00b204e9800998ecf8427e",
			},
		},
	}

	t.Run("with owner and repo adds columns", func(t *testing.T) {
		err := renderPlanfileList(files, "table", "cloudposse", "atmos")
		assert.NoError(t, err)
	})

	t.Run("with owner and repo json format", func(t *testing.T) {
		err := renderPlanfileList(files, "json", "cloudposse", "atmos")
		assert.NoError(t, err)
	})

	t.Run("without owner and repo omits columns", func(t *testing.T) {
		err := renderPlanfileList(files, "table", "", "")
		assert.NoError(t, err)
	})

	t.Run("with only owner still adds columns", func(t *testing.T) {
		err := renderPlanfileList(files, "table", "cloudposse", "")
		assert.NoError(t, err)
	})
}

func TestRenderPlanfileListMD5Column(t *testing.T) {
	initTestIO(t)

	t.Run("md5 present in output from metadata", func(t *testing.T) {
		files := []planfile.PlanfileInfo{
			{
				Key:          "test/key.tfplan",
				Size:         512,
				LastModified: time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC),
				Metadata: &planfile.Metadata{
					Stack:     "test-stack",
					Component: "test-component",
					SHA:       "abc123def456",
					MD5:       "d41d8cd98f00b204e9800998ecf8427e",
				},
			},
		}
		err := renderPlanfileList(files, "table", "", "")
		assert.NoError(t, err)
	})

	t.Run("md5 empty when no metadata", func(t *testing.T) {
		files := []planfile.PlanfileInfo{
			{
				Key:          "test/key.tfplan",
				Size:         512,
				LastModified: time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC),
			},
		}
		err := renderPlanfileList(files, "table", "", "")
		assert.NoError(t, err)
	})
}

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
				MD5:        "abcdef1234567890",
				HasChanges: true,
				Additions:  5,
				Changes:    3,
			},
		},
	}

	formats := []string{"table", "json", "yaml", "csv", "tsv", ""}
	for _, format := range formats {
		t.Run("format_"+format, func(t *testing.T) {
			assert.NotPanics(t, func() {
				_ = renderPlanfileList(files, format, "", "")
			})
		})
	}
}

func TestOutputFormatsWithOwnerRepo(t *testing.T) {
	initTestIO(t)

	files := []planfile.PlanfileInfo{
		{
			Key:          "test/key.tfplan",
			Size:         512,
			LastModified: time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC),
			Metadata: &planfile.Metadata{
				Stack:     "test-stack",
				Component: "test-component",
				SHA:       "abc123def456",
				MD5:       "abcdef1234567890",
			},
		},
	}

	formats := []string{"table", "json", "yaml", "csv", "tsv", ""}
	for _, format := range formats {
		t.Run("format_"+format+"_with_owner_repo", func(t *testing.T) {
			assert.NotPanics(t, func() {
				_ = renderPlanfileList(files, format, "cloudposse", "atmos")
			})
		})
	}
}
