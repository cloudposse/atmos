package client

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
)

// TestToolsCmd_Registration is the basic shape guard for the command.
func TestToolsCmd_Registration(t *testing.T) {
	assert.Equal(t, "tools <name>", toolsCmd.Use)
	assert.NotEmpty(t, toolsCmd.Short)
	assert.NotEmpty(t, toolsCmd.Long)
	assert.NotNil(t, toolsCmd.RunE)
}

// TestToolsCmd_StandardListFlagsRegistered guards issue #7 in
// docs/fixes/2026-05-15-mcp-review-fixes.md: the `tools` command must
// expose the same renderer-pipeline flag surface as `mcp list`
// (--format, --columns, --sort, --delimiter) so users can pipe and
// reshape output uniformly across `mcp` subcommands.
func TestToolsCmd_StandardListFlagsRegistered(t *testing.T) {
	for _, name := range []string{"format", "columns", "sort", "delimiter"} {
		t.Run("flag "+name, func(t *testing.T) {
			f := toolsCmd.Flags().Lookup(name)
			require.NotNil(t, f, "flag --%s MUST be registered on tools command", name)
		})
	}
}

// renderMCPToolsToString mirrors renderMCPListToString in list_test.go —
// it drives the renderer pipeline but uses RenderToString to avoid
// data.InitWriter being required (which the production Render() expects).
// This is the same test-seam pattern the sibling command already uses.
func renderMCPToolsToString(t *testing.T, data []map[string]any, opts *MCPToolsOptions) (string, error) {
	t.Helper()

	cols := getMCPToolsColumns(opts.Columns)
	selector, err := column.NewSelector(cols, column.BuildColumnFuncMap())
	require.NoError(t, err)

	sorters, err := buildMCPToolsSorters(opts.Sort)
	require.NoError(t, err)

	outputFormat := format.Format(opts.Format)
	r := renderer.New([]filter.Filter{}, selector, sorters, outputFormat, opts.Delimiter)

	return r.RenderToString(data)
}

// TestRenderMCPTools_DefaultColumns covers the happy-path renderer output
// for the default `--format=table` shape, asserting both NAME and
// DESCRIPTION columns are present with the expected data.
func TestRenderMCPTools_DefaultColumns(t *testing.T) {
	data := []map[string]any{
		{
			"name":             "list_iam_roles",
			"description":      firstSentence("Lists all IAM roles."),
			"full_description": "Lists all IAM roles in the account.",
		},
		{
			"name":             "get_role_policy",
			"description":      firstSentence("Returns the policy attached to a role."),
			"full_description": "Returns the policy attached to a role.",
		},
	}

	out, err := renderMCPToolsToString(t, data, &MCPToolsOptions{Format: "table"})
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// RenderToString emits row data only (no header decoration — that's
	// applied by the production Render path). The data values still
	// reflect the configured columns (name + description), so checking
	// the data is sufficient to prove the column selector resolved both.
	assert.Contains(t, out, "list_iam_roles")
	assert.Contains(t, out, "get_role_policy")
	// First-sentence truncation must have been applied — both descriptions
	// here are single sentences ending in `.`, so they survive intact.
	assert.Contains(t, out, "Lists all IAM roles.")
	assert.Contains(t, out, "Returns the policy attached to a role.")
}

// TestRenderMCPTools_FormatJSON exercises the JSON output path through the
// renderer pipeline. Pre-fix this command had no --format flag at all and
// always rendered a hand-rolled table; this test pins the JSON contract
// (issue #7 in docs/fixes/2026-05-15-mcp-review-fixes.md).
func TestRenderMCPTools_FormatJSON(t *testing.T) {
	data := []map[string]any{
		{
			"name":             "list_iam_roles",
			"description":      "Lists all IAM roles.",
			"full_description": "Lists all IAM roles in the account.",
		},
		{
			"name":             "describe_role",
			"description":      "Describes a role.",
			"full_description": "Describes a role by name or ARN.",
		},
	}

	out, err := renderMCPToolsToString(t, data, &MCPToolsOptions{Format: "json"})
	require.NoError(t, err)

	// JSON output must be parseable and contain both tool names.
	require.True(t, json.Valid([]byte(out)),
		"JSON output MUST be valid; got:\n%s", out)
	assert.Contains(t, out, "list_iam_roles")
	assert.Contains(t, out, "describe_role")
}

// TestRenderMCPTools_SortByName checks that tools come out in
// alphabetical order even when the input data is unsorted. This is the
// effect of the default `NAME asc` sorter buildMCPToolsSorters returns
// when --sort is empty.
func TestRenderMCPTools_SortByName(t *testing.T) {
	// Input data is intentionally out of order.
	data := []map[string]any{
		{"name": "zzz_tool", "description": "Z tool."},
		{"name": "aaa_tool", "description": "A tool."},
		{"name": "mmm_tool", "description": "M tool."},
	}

	out, err := renderMCPToolsToString(t, data, &MCPToolsOptions{Format: "json"})
	require.NoError(t, err)

	// Parse and assert order.
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))
	require.Len(t, rows, 3)
	// The renderer applies the sorter on whatever column the user
	// requested via --sort. The default for `tools` is NAME ascending,
	// matching `list`. Note: the column headers in the JSON output are
	// the display names ("NAME"), not the data keys ("name").
	assert.Equal(t, "aaa_tool", rows[0]["NAME"])
	assert.Equal(t, "mmm_tool", rows[1]["NAME"])
	assert.Equal(t, "zzz_tool", rows[2]["NAME"])
}

// TestGetMCPToolsColumns_Defaults locks in the documented default columns
// (NAME + DESCRIPTION). If a future change re-orders or adds defaults, the
// docs in markdown/atmos_mcp_tools.md need updating in lockstep.
func TestGetMCPToolsColumns_Defaults(t *testing.T) {
	cols := getMCPToolsColumns(nil)
	require.Len(t, cols, 2)
	assert.Equal(t, "NAME", cols[0].Name)
	assert.Equal(t, "DESCRIPTION", cols[1].Name)
}

// TestGetMCPToolsColumns_CustomFromFlag exercises the --columns flag's
// simple-name + Name=template forms — same shape as `mcp list`.
func TestGetMCPToolsColumns_CustomFromFlag(t *testing.T) {
	cols := getMCPToolsColumns([]string{"NAME", "Full={{ .full_description }}"})
	require.Len(t, cols, 2)
	assert.Equal(t, "NAME", cols[0].Name)
	assert.Equal(t, "{{ .name }}", cols[0].Value)
	assert.Equal(t, "Full", cols[1].Name)
	assert.Equal(t, "{{ .full_description }}", cols[1].Value)
}
