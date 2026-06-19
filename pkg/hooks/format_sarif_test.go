package hooks_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/hooks"
	_ "github.com/cloudposse/atmos/pkg/hooks/sarif" // registers the "sarif" format handler via init().
	"github.com/cloudposse/atmos/pkg/schema"
)

// minimalSARIF is a one-finding SARIF document the test subprocess writes to
// $ATMOS_OUTPUT_FILE, standing in for a custom SARIF-emitting tool ("tfsec").
const minimalSARIF = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"tfsec"}},` +
	`"results":[{"ruleId":"AVD-AWS-0001","level":"error","message":{"text":"bucket not encrypted"},` +
	`"locations":[{"physicalLocation":{"artifactLocation":{"uri":"main.tf"},"region":{"startLine":6}}}]}]}]}`

// A generic `kind: command` hook with `format: sarif` must reuse the shared
// SARIF handler: its output is parsed into a Summary with findings + raw SARIF,
// and the report is labeled by the SARIF's own tool name (not "command").
// This covers the custom-hook extensibility path end-to-end through Run.
func TestCommandKind_FormatSARIF_SurfacesFindings(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	kind := &hooks.Kind{Name: "command", Engine: &hooks.CommandEngine{}}
	hook := &hooks.Hook{
		Kind:    "command",
		Command: exe,
		Args:    []string{"-test.run", "^$"},
		Format:  hooks.FormatSARIF,
		Env: map[string]string{
			"_ATMOS_TEST_WRITE_OUTPUT": "1",
			"_ATMOS_TEST_OUTPUT_BODY":  minimalSARIF,
		},
	}
	ctx := &hooks.ExecContext{
		Hook:        hook,
		Kind:        kind,
		AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: t.TempDir()},
		Info:        &schema.ConfigAndStacksInfo{Stack: "test", ComponentFromArg: "bucket"},
	}

	out, err := kind.Engine.Run(ctx)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.Summary)

	// Labeled by the tool's own SARIF driver name, not the literal kind.
	assert.Equal(t, "tfsec", out.Summary.Kind)

	// Structured findings surfaced for annotations.
	require.Len(t, out.Summary.Findings, 1)
	f := out.Summary.Findings[0]
	assert.Equal(t, "main.tf", f.Path)
	assert.Equal(t, 6, f.Line)
	assert.Equal(t, "high", f.Severity) // SARIF level "error" → high.
	assert.Equal(t, "AVD-AWS-0001", f.RuleID)

	// Raw SARIF surfaced for upload.
	assert.NotEmpty(t, out.Summary.SARIF)
}
