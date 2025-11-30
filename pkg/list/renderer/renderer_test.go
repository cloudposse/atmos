package renderer

import (
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestNew(t *testing.T) {
	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
	}
	selector, err := column.NewSelector(configs, template.FuncMap{})
	require.NoError(t, err)

	r := New(
		[]filter.Filter{},
		selector,
		[]*sort.Sorter{},
		format.FormatJSON,
		"",
	)

	assert.NotNil(t, r)
	assert.NotNil(t, r.output)
	assert.Equal(t, format.FormatJSON, r.format)
}

func TestRenderer_Render_Complete(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc", "stack": "prod", "enabled": true},
		{"component": "eks", "stack": "dev", "enabled": false},
		{"component": "rds", "stack": "prod", "enabled": true},
		{"component": "s3", "stack": "staging", "enabled": true},
	}

	// Column configuration.
	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Stack", Value: "{{ .stack }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Filter: only prod stack.
	filters := []filter.Filter{
		filter.NewColumnFilter("stack", "prod"),
	}

	// Sort: by component ascending.
	sorters := []*sort.Sorter{
		sort.NewSorter("Component", sort.Ascending),
	}

	r := New(filters, selector, sorters, format.FormatJSON, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_NoFilters(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc", "stack": "prod"},
		{"component": "eks", "stack": "dev"},
	}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	r := New(nil, selector, nil, format.FormatJSON, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_NoSorters(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc"},
		{"component": "eks"},
	}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	r := New(nil, selector, nil, format.FormatYAML, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_MultipleFilters(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc", "stack": "prod", "enabled": true},
		{"component": "eks", "stack": "dev", "enabled": false},
		{"component": "rds", "stack": "prod", "enabled": true},
		{"component": "s3", "stack": "prod", "enabled": false},
	}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Enabled", Value: "{{ .enabled }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	trueVal := true

	// Filter: prod stack AND enabled=true.
	filters := []filter.Filter{
		filter.NewColumnFilter("stack", "prod"),
		filter.NewBoolFilter("enabled", &trueVal),
	}

	r := New(filters, selector, nil, format.FormatCSV, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_MultiSorter(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc", "stack": "prod"},
		{"component": "eks", "stack": "dev"},
		{"component": "vpc", "stack": "dev"},
		{"component": "eks", "stack": "prod"},
	}

	configs := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Component", Value: "{{ .component }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Sort by stack ascending, then component ascending.
	sorters := []*sort.Sorter{
		sort.NewSorter("Stack", sort.Ascending),
		sort.NewSorter("Component", sort.Ascending),
	}

	r := New(nil, selector, sorters, format.FormatTSV, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_EmptyData(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	r := New(nil, selector, nil, format.FormatJSON, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_TableFormat(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc", "stack": "prod"},
		{"component": "eks", "stack": "dev"},
	}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Stack", Value: "{{ .stack }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	r := New(nil, selector, nil, format.FormatTable, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_InvalidColumnTemplate(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc"},
	}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Bad", Value: "{{ .nonexistent.nested.field }}"}, // Will produce <no value>
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	r := New(nil, selector, nil, format.FormatJSON, "")

	// Should still succeed - template returns "<no value>" for missing fields.
	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_FilterReturnsNoResults(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc", "stack": "prod"},
		{"component": "eks", "stack": "dev"},
	}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Filter that matches nothing.
	filters := []filter.Filter{
		filter.NewColumnFilter("stack", "nonexistent"),
	}

	r := New(filters, selector, nil, format.FormatJSON, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_GlobFilter(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc", "stack": "plat-ue2-prod"},
		{"component": "eks", "stack": "plat-ue2-dev"},
		{"component": "rds", "stack": "plat-uw2-prod"},
	}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Stack", Value: "{{ .stack }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Filter: plat-*-prod pattern.
	globFilter, err := filter.NewGlobFilter("stack", "plat-*-prod")
	require.NoError(t, err)

	filters := []filter.Filter{globFilter}

	r := New(filters, selector, nil, format.FormatJSON, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_CompleteWorkflow(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	// Simulate realistic component list data.
	testData := []map[string]any{
		{"component": "vpc", "stack": "plat-ue2-prod", "type": "real", "enabled": true},
		{"component": "eks", "stack": "plat-ue2-dev", "type": "real", "enabled": false},
		{"component": "rds", "stack": "plat-ue2-prod", "type": "real", "enabled": true},
		{"component": "s3", "stack": "plat-uw2-prod", "type": "abstract", "enabled": true},
		{"component": "lambda", "stack": "plat-ue2-staging", "type": "real", "enabled": false},
	}

	// Column configuration with templates.
	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Type", Value: "{{ .type }}"},
		{Name: "Enabled", Value: "{{ .enabled }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	trueVal := true

	// Filters: real components, prod stacks, enabled only.
	globFilter, err := filter.NewGlobFilter("stack", "*-prod")
	require.NoError(t, err)

	filters := []filter.Filter{
		filter.NewColumnFilter("type", "real"),
		globFilter,
		filter.NewBoolFilter("enabled", &trueVal),
	}

	// Sort: by component ascending.
	sorters := []*sort.Sorter{
		sort.NewSorter("Component", sort.Ascending),
	}

	r := New(filters, selector, sorters, format.FormatJSON, "")

	err = r.Render(testData)
	assert.NoError(t, err)
}

func TestRenderer_Render_SortError(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"component": "vpc"},
	}

	configs := []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Sort by non-existent column.
	sorters := []*sort.Sorter{
		sort.NewSorter("NonExistent", sort.Ascending),
	}

	r := New(nil, selector, sorters, format.FormatJSON, "")

	err = r.Render(testData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sorting failed")
}

func TestRenderer_Render_AllFormats(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"name": "test1", "value": "1"},
		{"name": "test2", "value": "2"},
	}

	configs := []column.Config{
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "Value", Value: "{{ .value }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	formats := []format.Format{
		format.FormatJSON,
		format.FormatYAML,
		format.FormatCSV,
		format.FormatTSV,
		format.FormatTable,
	}

	for _, f := range formats {
		t.Run(string(f), func(t *testing.T) {
			r := New(nil, selector, nil, f, "")
			err := r.Render(testData)
			assert.NoError(t, err)
		})
	}
}

func TestRenderer_Render_UnsupportedFormat(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"name": "test"},
	}

	configs := []column.Config{
		{Name: "Name", Value: "{{ .name }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Use an unsupported format.
	r := New(nil, selector, nil, format.Format("unsupported"), "")

	err = r.Render(testData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "formatting failed")
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestRenderer_Render_FilterReturnsInvalidType(t *testing.T) {
	// Initialize I/O context for output tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	testData := []map[string]any{
		{"name": "test"},
	}

	configs := []column.Config{
		{Name: "Name", Value: "{{ .name }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// Use a filter that returns invalid type.
	badFilter := &mockBadFilter{}

	r := New([]filter.Filter{badFilter}, selector, nil, format.FormatJSON, "")

	err = r.Render(testData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "filter returned invalid type")
}
