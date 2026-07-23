package renderer

import (
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestRenderer_RenderToString_PathsFormat(t *testing.T) {
	testData := []map[string]any{
		{"file": "atmos.d/integrations.yaml", "path": "integrations.github.enabled", "type": "bool", "value": "true"},
		{"file": "atmos.yaml", "path": "logs.level", "type": "string", "value": "info"},
		{"file": "atmos.yaml", "path": "components.terraform.base_path", "type": "string", "value": "components/terraform"},
	}

	configs := []column.Config{
		{Name: "file", Value: "{{ .file }}"},
		{Name: "path", Value: "{{ .path }}"},
		{Name: "type", Value: "{{ .type }}"},
		{Name: "value", Value: "{{ .value }}"},
	}
	selector, err := column.NewSelector(configs, column.BuildColumnFuncMap())
	require.NoError(t, err)

	r := New(
		nil,
		selector,
		[]*sort.Sorter{
			sort.NewSorter("file", sort.Ascending),
			sort.NewSorter("path", sort.Ascending),
		},
		format.FormatPaths,
		"",
	)

	output, err := r.RenderToString(testData)
	require.NoError(t, err)
	require.Equal(t, `atmos.d/integrations.yaml
  integrations.github.enabled

atmos.yaml
  components.terraform.base_path
  logs.level
`, output)
}

func TestFormatStyledPathsIncludesTypeAndValue(t *testing.T) {
	output := formatStyledPaths(
		[]string{"file", "path", "type", "value"},
		[][]string{
			{"atmos.yaml", "logs.level", "string", "info"},
			{"atmos.yaml", "settings", "object", "{2 keys}"},
			{"atmos.yaml", "commands[0].steps[0]", "string", "echo one\necho two\n"},
		},
	)

	require.Contains(t, output, "atmos.yaml")
	require.Contains(t, output, "logs.level")
	require.Contains(t, output, "info")
	require.Contains(t, output, "settings")
	require.Contains(t, output, "{2 keys}")
	require.Contains(t, output, "echo one ... (2 lines)")

	lines := strings.Split(output, "\n")
	logLine := lines[1]
	settingsLine := lines[2]
	commandLine := lines[3]
	require.Equal(t, strings.Index(logLine, "string"), strings.Index(settingsLine, "object"))
	require.Equal(t, strings.Index(logLine, "string"), strings.Index(commandLine, "string"))
	require.Equal(t, strings.Index(logLine, "info"), strings.Index(settingsLine, "{2 keys}"))
	require.Equal(t, strings.Index(logLine, "info"), strings.Index(commandLine, "echo one"))
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

func TestRenderer_RenderToString_NilSelector(t *testing.T) {
	r := New(nil, nil, nil, format.FormatJSON, "")

	_, err := r.RenderToString([]map[string]any{{"component": "vpc"}})
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	require.Contains(t, err.Error(), "nil column selector")
}

func TestFormatPlainPaths_MissingColumns(t *testing.T) {
	// Neither "file" nor "path" columns present: fileIndex/pathIndex guard returns "".
	output := formatPlainPaths(
		[]string{"name", "value"},
		[][]string{{"foo", "bar"}},
	)
	require.Empty(t, output)
}

func TestFormatPlainPaths_MixedValidAndShortRows(t *testing.T) {
	// One row is missing the "path" column value entirely (shorter than the header
	// count), the other rows are well-formed. The short row must be skipped while
	// valid rows are still rendered, exercising both sides of the per-row bounds check.
	output := formatPlainPaths(
		[]string{"file", "path"},
		[][]string{
			{"atmos.yaml"}, // Too short: pathIndex (1) >= len(row) (1) -> skipped.
			{"atmos.yaml", "logs.level"},
			{"atmos.d.yaml", "settings.enabled"},
		},
	)
	require.Equal(t, "atmos.yaml\n  logs.level\n\natmos.d.yaml\n  settings.enabled\n", output)
}

func TestFormatPlainPaths_AllRowsShort(t *testing.T) {
	// Every row fails the bounds check, so no lines are ever appended and the
	// len(lines) == 0 branch returns "".
	output := formatPlainPaths(
		[]string{"file", "path"},
		[][]string{{"atmos.yaml"}, {}},
	)
	require.Empty(t, output)
}

func TestAppendStyledPathLine_MixedValidAndShortRows(t *testing.T) {
	styles := styledPathStyles()
	opts := styledPathLineOptions{
		indexes: pathColumnIndexes{file: 0, path: 1, typ: 2, value: 3},
		widths:  pathColumnWidths{path: 10, typ: 6},
		styles:  &styles,
	}

	var lines []string
	currentFile := ""

	// Row shorter than required indexes: bounds check must reject it (ok == false),
	// leaving lines/currentFile untouched.
	nextLines, nextFile, ok := appendStyledPathLine(lines, currentFile, []string{"atmos.yaml"}, opts)
	require.False(t, ok)
	require.Equal(t, lines, nextLines)
	require.Equal(t, currentFile, nextFile)

	// A well-formed row is appended and becomes the new "current file": a file
	// header line is emitted first, followed by the content line.
	lines, currentFile, ok = appendStyledPathLine(nextLines, nextFile, []string{"atmos.yaml", "logs.level", "string", "info"}, opts)
	require.True(t, ok)
	require.Equal(t, "atmos.yaml", currentFile)
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "atmos.yaml")
	require.Contains(t, lines[1], "logs.level")

	// A second row for the same file does not repeat the file header.
	lines, currentFile, ok = appendStyledPathLine(lines, currentFile, []string{"atmos.yaml", "settings", "object", ""}, opts)
	require.True(t, ok)
	require.Equal(t, "atmos.yaml", currentFile)
	require.Len(t, lines, 3)

	// A row for a new file inserts a blank separator line before the new file header.
	lines, currentFile, ok = appendStyledPathLine(lines, currentFile, []string{"other.yaml", "path.a", "string", "x"}, opts)
	require.True(t, ok)
	require.Equal(t, "other.yaml", currentFile)
	require.Len(t, lines, 6)
	require.Empty(t, lines[3])
	require.Contains(t, lines[4], "other.yaml")
}
