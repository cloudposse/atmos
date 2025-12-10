package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestRenderer_EmptyFormat tests that empty format defaults to table.
func TestRenderer_EmptyFormat(t *testing.T) {
	// Initialize I/O context.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	data := []map[string]any{
		{"stack": "plat-ue2-dev", "component": "vpc"},
		{"stack": "plat-ue2-prod", "component": "vpc"},
	}

	columns := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Component", Value: "{{ .component }}"},
	}

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Create renderer with empty format string.
	r := New(nil, selector, nil, "", "")

	// Should not error - should default to table format.
	err = r.Render(data)
	assert.NoError(t, err, "Empty format should default to table")
}

// TestRenderer_AllFormats tests all supported formats.
func TestRenderer_AllFormats(t *testing.T) {
	// Initialize I/O context.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"stack": "plat-ue2-dev", "component": "vpc"},
		{"stack": "plat-ue2-prod", "component": "eks"},
	}

	columns := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Component", Value: "{{ .component }}"},
	}

	testCases := []struct {
		name   string
		format format.Format
	}{
		{"table format", format.FormatTable},
		{"json format", format.FormatJSON},
		{"yaml format", format.FormatYAML},
		{"csv format", format.FormatCSV},
		{"tsv format", format.FormatTSV},
		{"empty format (defaults to table)", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
			require.NoError(t, err)

			r := New(nil, selector, nil, tc.format, "")
			err = r.Render(testData)
			assert.NoError(t, err)
		})
	}
}

// TestRenderer_WithSorting tests renderer with sorting.
func TestRenderer_WithSorting(t *testing.T) {
	// Initialize I/O context.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"stack": "plat-ue2-prod", "component": "vpc"},
		{"stack": "plat-ue2-dev", "component": "vpc"},
		{"stack": "plat-uw2-dev", "component": "eks"},
	}

	columns := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Component", Value: "{{ .component }}"},
	}

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Sort by stack ascending.
	sorters := []*sort.Sorter{
		sort.NewSorter("Stack", sort.Ascending),
	}

	r := New(nil, selector, sorters, format.FormatJSON, "")
	err = r.Render(testData)
	assert.NoError(t, err)
}

// TestRenderer_EmptyData tests renderer with empty data.
func TestRenderer_EmptyData(t *testing.T) {
	// Initialize I/O context.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{}

	columns := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
	}

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	require.NoError(t, err)

	r := New(nil, selector, nil, format.FormatTable, "")
	err = r.Render(testData)
	assert.NoError(t, err)
}

// TestRenderer_InvalidFormat tests unsupported format handling.
func TestRenderer_InvalidFormat(t *testing.T) {
	// Initialize I/O context.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"stack": "plat-ue2-dev"},
	}

	columns := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
	}

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Use invalid format.
	r := New(nil, selector, nil, format.Format("invalid"), "")
	err = r.Render(testData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}
