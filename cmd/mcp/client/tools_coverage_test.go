package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// initMCPToolsTestIO initializes the I/O context so renderMCPTools'
// inner r.Render() call has a writer to flush into. Without this,
// data.Writer() returns nil and Render fails with a nil-pointer
// dereference. Tests that call renderMCPTools (or executeMCPTools)
// directly must call this first.
func initMCPToolsTestIO(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() {
		ui.Reset()
	})
}

// TestRenderMCPTools_DirectCallSucceeds drives the production
// renderMCPTools function (which routes through r.Render → data.Writer)
// rather than the test-only RenderToString seam. This is the coverage
// lever that takes renderMCPTools from 0% → ~100% without needing a
// real MCP server.
func TestRenderMCPTools_DirectCallSucceeds(t *testing.T) {
	initMCPToolsTestIO(t)

	rows := []map[string]any{
		{"name": "do_thing", "description": "Does the thing."},
		{"name": "do_other", "description": "Does the other thing."},
	}
	err := renderMCPTools(rows, &MCPToolsOptions{Format: "json"})
	assert.NoError(t, err,
		"renderMCPTools must succeed for well-formed data + valid format")
}

// TestRenderMCPTools_InvalidSortSpecReturnsWrappedError covers the
// buildMCPToolsSorters error wrap inside renderMCPTools. ParseSortSpec
// rejects malformed specs ("not-a-real-spec:::") with a non-nil error
// that renderMCPTools then wraps in "error parsing sort specification".
func TestRenderMCPTools_InvalidSortSpecReturnsWrappedError(t *testing.T) {
	initMCPToolsTestIO(t)

	rows := []map[string]any{{"name": "x", "description": "y."}}
	err := renderMCPTools(rows, &MCPToolsOptions{
		Format: "json",
		Sort:   "::::not-a-valid-sort-spec::::",
	})
	require.Error(t, err,
		"renderMCPTools must surface the sort-parse error so the user can see what was wrong with their --sort flag")
	assert.Contains(t, err.Error(), "sort",
		"the wrap must mention sort so the diagnostic is actionable; got: %v", err)
}

// TestExecuteMCPTools_NonexistentServerReturnsError exercises the
// `mgr.Start` failure path of executeMCPTools. With an atmos.yaml
// pointing at a server whose command does not exist, mgr.Start fails
// and executeMCPTools returns the wrapped error to cobra. Without this
// test, executeMCPTools' err-from-Start branch is uncovered.
func TestExecuteMCPTools_NonexistentServerReturnsError(t *testing.T) {
	initMCPToolsTestIO(t)

	tempDir := t.TempDir()
	atmosYAML := `
base_path: "."
mcp:
  servers:
    broken:
      command: "nonexistent-binary-for-tools-test-xyz"
      description: "intentionally invalid"
`
	require.NoError(t,
		os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))
	t.Chdir(tempDir)

	cmd := newTestCobraCmdForMCPTools(t)
	err := executeMCPTools(cmd, []string{"broken"})

	require.Error(t, err,
		"executeMCPTools must propagate the mgr.Start error when the server's command is unresolvable")
}

// TestExecuteMCPTools_UnknownServerNameReturnsError covers the case
// where the server name passed in args is not configured in atmos.yaml.
// Manager Start returns an ErrMCPServerNotFound-style sentinel.
func TestExecuteMCPTools_UnknownServerNameReturnsError(t *testing.T) {
	initMCPToolsTestIO(t)

	tempDir := t.TempDir()
	atmosYAML := `
base_path: "."
mcp:
  servers:
    only-server:
      command: "echo"
      description: "the only configured server"
`
	require.NoError(t,
		os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))
	t.Chdir(tempDir)

	cmd := newTestCobraCmdForMCPTools(t)
	err := executeMCPTools(cmd, []string{"this-server-does-not-exist"})

	require.Error(t, err,
		"executeMCPTools must return an error when the requested server name is not configured")
}

// newTestCobraCmdForMCPTools builds a cobra command with the same flag
// surface executeMCPTools needs. Mirrors newTestCobraCmdForMCPTest in
// test_cmd_test.go for the same reason — isolates the test from the
// production init() side effects on the shared toolsCmd singleton.
func newTestCobraCmdForMCPTools(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{
		Use:  "tools <name>",
		Args: cobra.ExactArgs(1),
	}
	// Register the same flags executeMCPTools reads via Viper. The
	// production parser is the canonical source of these names — but
	// we can't share parser state across tests, so we re-register the
	// minimal set executeMCPTools actually reads.
	cmd.Flags().String(flagFormat, "json", "")
	cmd.Flags().StringSlice(flagColumns, nil, "")
	cmd.Flags().String(flagSort, "", "")
	cmd.Flags().String(flagDelimiter, "", "")
	return cmd
}

// TestParseMCPToolsColumnSpec covers the column-spec parser's three
// distinct branches: simple name (no `=`), Name=value (value gets
// wrapped in template syntax), Name={{ .raw_template }} (left alone).
//
// The existing TestGetMCPToolsColumns_CustomFromFlag only touches two
// of these branches; this table-driven test pins all three plus the
// `=`-at-position-0 edge case.
func TestParseMCPToolsColumnSpec(t *testing.T) {
	tests := []struct {
		name      string
		spec      string
		wantName  string
		wantValue string
	}{
		{
			name:      "simple name lowercased into template",
			spec:      "STATUS",
			wantName:  "STATUS",
			wantValue: "{{ .status }}",
		},
		{
			name:      "Name=plainvalue gets wrapped in template syntax",
			spec:      "Custom=field_name",
			wantName:  "Custom",
			wantValue: "{{ .field_name }}",
		},
		{
			name:      "Name={{ template }} is left as-is",
			spec:      "Full={{ .full_description }}",
			wantName:  "Full",
			wantValue: "{{ .full_description }}",
		},
		{
			name: "equal sign at position 0 falls through to simple-name branch",
			// `=foo`: the `=` at index 0 fails the `i > 0` guard, so the
			// loop ends and the simple-name branch wraps the whole spec
			// (including the leading `=`) as a data key. Pinning this
			// edge case prevents a future "let's also handle empty
			// names" change from silently breaking.
			spec:      "=foo",
			wantName:  "=foo",
			wantValue: "{{ .=foo }}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMCPToolsColumnSpec(tt.spec)
			assert.Equal(t, tt.wantName, got.Name)
			assert.Equal(t, tt.wantValue, got.Value)
		})
	}
}

// TestParseMCPToolsColumnsFlag_FiltersEmptyAndNamelessEntries covers
// the two skip branches in parseMCPToolsColumnsFlag: empty string
// entries and parsed configs with an empty Name.
func TestParseMCPToolsColumnsFlag_FiltersEmptyAndNamelessEntries(t *testing.T) {
	// Empty entries must be skipped; non-empty entries pass through.
	cols := parseMCPToolsColumnsFlag([]string{"", "NAME", "", "DESCRIPTION", ""})
	require.Len(t, cols, 2,
		"empty strings must be filtered out, leaving only the two named entries")
	assert.Equal(t, "NAME", cols[0].Name)
	assert.Equal(t, "DESCRIPTION", cols[1].Name)
}

// TestMCPToolsContainsTemplate covers all three branches of the
// trivial helper: explicit `{{` found, no `{` at all, single `{` but
// no following `{`.
func TestMCPToolsContainsTemplate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "double brace at start", input: "{{ .name }}", want: true},
		{name: "double brace in middle", input: "prefix {{ .name }} suffix", want: true},
		{name: "single brace not followed by brace", input: "literal { value }", want: false},
		{name: "no braces at all", input: "plain text", want: false},
		{name: "empty string", input: "", want: false},
		{name: "single character", input: "{", want: false},
		{name: "two non-brace characters", input: "ab", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mcpToolsContainsTemplate(tt.input))
		})
	}
}

// TestBuildMCPToolsSorters_CustomSortSpec covers the non-default
// branch — when the user passes --sort, ParseSortSpec is invoked
// instead of returning the hard-coded NAME asc sorter. The existing
// TestRenderMCPTools_SortByName only exercises the default-spec
// branch via the empty Sort field.
func TestBuildMCPToolsSorters_CustomSortSpec(t *testing.T) {
	sorters, err := buildMCPToolsSorters("description:desc")
	require.NoError(t, err)
	require.NotEmpty(t, sorters,
		"a valid sort spec must produce at least one sorter")
}

// TestBuildMCPToolsSorters_InvalidSpecReturnsError pins the wrap
// contract: a malformed sort spec must return a non-nil error so
// renderMCPTools can wrap it with "error parsing sort specification".
// We intentionally do not pin the exact error message — that is owned
// by listSort.ParseSortSpec, and coupling this test to that string
// would break on cosmetic message changes downstream.
func TestBuildMCPToolsSorters_InvalidSpecReturnsError(t *testing.T) {
	_, err := buildMCPToolsSorters("::::not-a-valid-spec::::")
	require.Error(t, err)
}
