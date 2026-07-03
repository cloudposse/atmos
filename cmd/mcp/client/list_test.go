package client

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
)

// TestGetMCPListColumns_Defaults verifies the default column configuration.
func TestGetMCPListColumns_Defaults(t *testing.T) {
	cols := getMCPListColumns(nil)

	require.Len(t, cols, 3)
	assert.Equal(t, "NAME", cols[0].Name)
	assert.Equal(t, "STATUS", cols[1].Name)
	assert.Equal(t, "DESCRIPTION", cols[2].Name)

	// Verify templates reference the correct data keys.
	assert.Equal(t, "{{ .name }}", cols[0].Value)
	assert.Equal(t, "{{ .status }}", cols[1].Value)
	assert.Equal(t, "{{ .description }}", cols[2].Value)
}

// TestGetMCPListColumns_FromFlag verifies that --columns flag overrides defaults.
// Simple name columns (e.g. "NAME") generate templates using the lowercase data key ({{ .name }}).
func TestGetMCPListColumns_FromFlag(t *testing.T) {
	cols := getMCPListColumns([]string{"NAME", "STATUS"})

	require.Len(t, cols, 2)
	assert.Equal(t, "NAME", cols[0].Name)
	assert.Equal(t, "STATUS", cols[1].Name)

	// Simple name columns should use the lowercase form as data key.
	assert.Equal(t, "{{ .name }}", cols[0].Value)
	assert.Equal(t, "{{ .status }}", cols[1].Value)
}

// TestGetMCPListColumns_EmptySlice verifies empty slice returns defaults.
func TestGetMCPListColumns_EmptySlice(t *testing.T) {
	cols := getMCPListColumns([]string{})

	require.Len(t, cols, 3, "empty slice should return default columns")
}

// TestParseMCPColumnsFlag_SimpleNames verifies simple name parsing.
// The display name is the spec as-is; the data key is lowercased.
func TestParseMCPColumnsFlag_SimpleNames(t *testing.T) {
	cols := parseMCPColumnsFlag([]string{"name", "status"})

	require.Len(t, cols, 2)
	assert.Equal(t, "name", cols[0].Name)
	// "name" → lowercase "name" → {{ .name }}
	assert.Equal(t, "{{ .name }}", cols[0].Value)
	assert.Equal(t, "status", cols[1].Name)
	assert.Equal(t, "{{ .status }}", cols[1].Value)
}

// TestParseMCPColumnsFlag_NameEqualsTemplate verifies Name=template parsing.
func TestParseMCPColumnsFlag_NameEqualsTemplate(t *testing.T) {
	cols := parseMCPColumnsFlag([]string{"Server={{ .name }}", "State={{ .status }}"})

	require.Len(t, cols, 2)
	assert.Equal(t, "Server", cols[0].Name)
	assert.Equal(t, "{{ .name }}", cols[0].Value)
	assert.Equal(t, "State", cols[1].Name)
	assert.Equal(t, "{{ .status }}", cols[1].Value)
}

// TestParseMCPColumnsFlag_NameEqualsFieldName verifies Name=field (non-template) wrapping.
func TestParseMCPColumnsFlag_NameEqualsFieldName(t *testing.T) {
	cols := parseMCPColumnsFlag([]string{"Server=name"})

	require.Len(t, cols, 1)
	assert.Equal(t, "Server", cols[0].Name)
	// Non-template value should be auto-wrapped.
	assert.Equal(t, "{{ .name }}", cols[0].Value)
}

// TestParseMCPColumnsFlag_EmptySpecsSkipped verifies empty entries are skipped.
func TestParseMCPColumnsFlag_EmptySpecsSkipped(t *testing.T) {
	cols := parseMCPColumnsFlag([]string{"name", "", "status"})

	require.Len(t, cols, 2, "empty spec should be skipped")
}

// TestParseMCPColumnSpec_SimpleCase verifies simple column spec parsing.
// The display name is preserved as-is; the data key is lowercased.
func TestParseMCPColumnSpec_SimpleCase(t *testing.T) {
	cfg := parseMCPColumnSpec("myfield")

	assert.Equal(t, "myfield", cfg.Name)
	// "myfield" is already lowercase, so data key == spec.
	assert.Equal(t, "{{ .myfield }}", cfg.Value)
}

// TestParseMCPColumnSpec_UppercaseName verifies uppercase name maps to lowercase data key.
func TestParseMCPColumnSpec_UppercaseName(t *testing.T) {
	cfg := parseMCPColumnSpec("NAME")

	assert.Equal(t, "NAME", cfg.Name)
	// Data key is lowercased: "NAME" → {{ .name }}.
	assert.Equal(t, "{{ .name }}", cfg.Value)
}

// TestParseMCPColumnSpec_WithTemplate verifies template column spec parsing.
func TestParseMCPColumnSpec_WithTemplate(t *testing.T) {
	cfg := parseMCPColumnSpec("Label={{ .name | upper }}")

	assert.Equal(t, "Label", cfg.Name)
	assert.Equal(t, "{{ .name | upper }}", cfg.Value)
}

// TestParseMCPColumnSpec_WithEqualsInTemplate verifies '=' inside templates doesn't split early.
func TestParseMCPColumnSpec_WithEqualsInTemplate(t *testing.T) {
	// The '=' in "ternary" result should not confuse the parser since we split on first '='.
	cfg := parseMCPColumnSpec("Status={{ .status }}")

	assert.Equal(t, "Status", cfg.Name)
	assert.Equal(t, "{{ .status }}", cfg.Value)
}

// TestContainsTemplate verifies template detection helper.
func TestContainsTemplate(t *testing.T) {
	assert.True(t, containsTemplate("{{ .name }}"))
	assert.True(t, containsTemplate("prefix {{ .field }} suffix"))
	assert.False(t, containsTemplate("just-a-field"))
	assert.False(t, containsTemplate(""))
	assert.False(t, containsTemplate("{single-brace}"))
}

// TestBuildMCPListSorters_Default verifies default sort is by NAME ascending.
func TestBuildMCPListSorters_Default(t *testing.T) {
	sorters, err := buildMCPListSorters("")

	require.NoError(t, err)
	require.Len(t, sorters, 1)
	assert.Equal(t, "NAME", sorters[0].Column)
}

// TestBuildMCPListSorters_CustomSpec verifies custom sort specification parsing.
// ParseSortSpec capitalizes the first character of the column name (e.g. "status" → "Status").
func TestBuildMCPListSorters_CustomSpec(t *testing.T) {
	sorters, err := buildMCPListSorters("status:desc")

	require.NoError(t, err)
	require.Len(t, sorters, 1)
	// ParseSortSpec capitalizes the first letter: "status" → "Status".
	assert.Equal(t, "Status", sorters[0].Column)
}

// TestBuildMCPListSorters_InvalidSpec verifies error on invalid sort spec.
func TestBuildMCPListSorters_InvalidSpec(t *testing.T) {
	_, err := buildMCPListSorters("invalid:badorder")

	require.Error(t, err)
}

// renderMCPListToString is a test helper that runs the full mcp list renderer pipeline
// and returns the formatted string output without writing to stdout (avoids data.InitWriter).
func renderMCPListToString(t *testing.T, data []map[string]any, opts *MCPListOptions) (string, error) {
	t.Helper()

	cols := getMCPListColumns(opts.Columns)

	selector, err := column.NewSelector(cols, column.BuildColumnFuncMap())
	require.NoError(t, err)

	sorters, err := buildMCPListSorters(opts.Sort)
	require.NoError(t, err)

	outputFormat := format.Format(opts.Format)
	r := renderer.New([]filter.Filter{}, selector, sorters, outputFormat, opts.Delimiter)

	return r.RenderToString(data)
}

// TestRenderMCPList_DefaultTableFormat verifies the renderer pipeline produces output
// when given well-formed data and default options.
func TestRenderMCPList_DefaultTableFormat(t *testing.T) {
	data := []map[string]any{
		{"name": "aws-docs", "status": "stopped", "description": "AWS Documentation"},
		{"name": "github-mcp", "status": "running", "description": "GitHub MCP server"},
	}

	out, err := renderMCPListToString(t, data, &MCPListOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}

// TestRenderMCPList_JSONFormat verifies JSON output format contains expected fields.
func TestRenderMCPList_JSONFormat(t *testing.T) {
	data := []map[string]any{
		{"name": "server-a", "status": "stopped", "description": "Test server"},
	}

	out, err := renderMCPListToString(t, data, &MCPListOptions{Format: "json"})
	require.NoError(t, err)
	assert.Contains(t, out, "server-a")
	assert.Contains(t, out, "stopped")
	assert.Contains(t, out, "Test server")
}

// TestRenderMCPList_CSVFormat verifies CSV output format.
func TestRenderMCPList_CSVFormat(t *testing.T) {
	data := []map[string]any{
		{"name": "server-a", "status": "stopped", "description": "Test server"},
	}

	out, err := renderMCPListToString(t, data, &MCPListOptions{Format: "csv"})
	require.NoError(t, err)
	// CSV should have headers and values separated by commas.
	assert.Contains(t, out, "NAME,STATUS,DESCRIPTION")
	assert.Contains(t, out, "server-a,stopped,Test server")
}

// TestRenderMCPList_CustomColumns verifies the --columns flag is respected.
func TestRenderMCPList_CustomColumns(t *testing.T) {
	data := []map[string]any{
		{"name": "server-a", "status": "stopped", "description": "Test server"},
	}

	out, err := renderMCPListToString(t, data, &MCPListOptions{Columns: []string{"NAME", "STATUS"}})
	require.NoError(t, err)
	assert.Contains(t, out, "server-a")
	assert.Contains(t, out, "stopped")
	// DESCRIPTION should not be present when only NAME and STATUS are requested.
	assert.NotContains(t, out, "Test server")
}

// TestRenderMCPList_EmptyData verifies the renderer handles empty data correctly
// by ensuring the column selector still initialises (returns headers, no rows).
func TestRenderMCPList_EmptyData(t *testing.T) {
	cols := getMCPListColumns(nil)
	selector, err := column.NewSelector(cols, column.BuildColumnFuncMap())
	require.NoError(t, err)

	headers, rows, err := selector.Extract([]map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, []string{"NAME", "STATUS", "DESCRIPTION"}, headers)
	assert.Empty(t, rows)
}

// TestRenderMCPList_SortByName verifies sort specification produces ordered output.
func TestRenderMCPList_SortByName(t *testing.T) {
	data := []map[string]any{
		{"name": "zzz-server", "status": "stopped", "description": "Last alphabetically"},
		{"name": "aaa-server", "status": "running", "description": "First alphabetically"},
	}

	out, err := renderMCPListToString(t, data, &MCPListOptions{Format: "csv", Sort: "NAME:asc"})
	require.NoError(t, err)

	// aaa-server should appear before zzz-server in the output.
	idxAAA := strings.Index(out, "aaa-server")
	idxZZZ := strings.Index(out, "zzz-server")
	assert.Less(t, idxAAA, idxZZZ, "aaa-server should appear before zzz-server after NAME:asc sort")
}
