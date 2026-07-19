package tflint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/scanners"
	"github.com/cloudposse/atmos/pkg/scanners/sarif"
)

func TestKindIsRegistered(t *testing.T) {
	kind, ok := hooks.GetKind(kindName)
	require.True(t, ok)

	assert.Equal(t, kindName, kind.Name)
	assert.Equal(t, "tflint", kind.Command)
	assert.Equal(t, []string{
		"--chdir=$ATMOS_COMPONENT_PATH",
		"--format=sarif",
	}, kind.DefaultArgs)
	assert.Equal(t, hooks.OnFailureWarn, kind.OnFailure)
	assert.Nil(t, kind.ResultHandler)
	_, ok = kind.Engine.(tflintEngine)
	assert.True(t, ok)
}

func TestResultHandlerReadsTflintSARIFFromOutputFile(t *testing.T) {
	// tflint --format=sarif output captured (by the engine) into ATMOS_OUTPUT_FILE.
	outputFile := filepath.Join(t.TempDir(), "tflint.sarif")
	require.NoError(t, os.WriteFile(outputFile, []byte(`{
		"runs": [{
			"tool": {"driver": {"name": "tflint"}},
			"results": [{
				"ruleId": "terraform_unused_declarations",
				"level": "error",
				"message": {"text": "variable \"unused\" is declared but not used"},
				"locations": [{
					"physicalLocation": {
						"artifactLocation": {"uri": "main.tf"},
						"region": {"startLine": 3}
					}
				}]
			}]
		}]
	}`), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       kindName,
		OutputPath: sarif.DefaultOutputFile,
	})
	scannerSummary, err := handler(&scanners.Context{OutputFile: outputFile})
	require.NoError(t, err)
	summary := toHookSummary(scannerSummary)
	require.NotNil(t, summary)
	assert.Equal(t, kindName, summary.Kind)
	assert.Equal(t, hooks.StatusWarning, summary.Status)
	assert.Equal(t, "1 HIGH", summary.Title)
	assert.Equal(t, 1, summary.Counts["high"])

	// CI tie-in: Findings drive PR annotations, SARIF drives Code Scanning
	// upload. Assert both so the native-CI integration can't silently regress.
	require.Len(t, summary.Findings, 1)
	f := summary.Findings[0]
	assert.Equal(t, "terraform_unused_declarations", f.RuleID)
	assert.Equal(t, "high", f.Severity)
	assert.Equal(t, 3, f.Line)
	assert.Contains(t, f.Path, "main.tf")
	assert.NotEmpty(t, summary.SARIF, "raw SARIF must be preserved for Code Scanning upload")
}

func TestTflintEngineRunNilGuards(t *testing.T) {
	var engine tflintEngine

	out, err := engine.Run(nil)
	require.Nil(t, out)
	require.ErrorIs(t, err, errUtils.ErrNilParam)

	out, err = engine.Run(&hooks.ExecContext{})
	require.Nil(t, out)
	require.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestCopyScanStateNilGuards(t *testing.T) {
	// Neither side must panic when the other is nil.
	copyScanState(nil, &scanners.Context{OutputFile: "x"})
	copyScanState(&hooks.ExecContext{}, nil)
}

func TestCopyScanStateCopiesFields(t *testing.T) {
	ctx := &hooks.ExecContext{}
	scan := &scanners.Context{
		OutputFile:   "/tmp/out.sarif",
		OutputDir:    "/tmp",
		ExitCode:     2,
		CommandError: assert.AnError,
	}

	copyScanState(ctx, scan)

	assert.Equal(t, "/tmp/out.sarif", ctx.OutputFile)
	assert.Equal(t, "/tmp", ctx.OutputDir)
	assert.Equal(t, 2, ctx.ExitCode)
	require.ErrorIs(t, ctx.CommandError, assert.AnError)
}

func TestToHookOutputNil(t *testing.T) {
	require.Nil(t, toHookOutput(nil))
}

func TestToHookOutputConvertsArtifactAndSummary(t *testing.T) {
	in := &scanners.Output{
		Artifact: &scanners.Artifact{
			Name:     "tflint.sarif",
			Body:     []byte("sarif-body"),
			Format:   "sarif",
			Metadata: map[string]string{"kind": "tflint"},
		},
		Summary: &scanners.Summary{
			Kind:   "tflint",
			Status: scanners.StatusFailure,
			Title:  "1 HIGH",
			Counts: map[string]int{"high": 1},
			Body:   "body",
			SARIF:  []byte("raw"),
		},
	}

	out := toHookOutput(in)
	require.NotNil(t, out)
	require.NotNil(t, out.Artifact)
	require.NotNil(t, out.Summary)

	assert.Equal(t, "tflint.sarif", out.Artifact.Name)
	assert.Equal(t, []byte("sarif-body"), out.Artifact.Body)
	assert.Equal(t, "sarif", out.Artifact.Format)
	assert.Equal(t, map[string]string{"kind": "tflint"}, out.Artifact.Metadata)

	assert.Equal(t, "tflint", out.Summary.Kind)
	assert.Equal(t, hooks.StatusFailure, out.Summary.Status)
	assert.Equal(t, "1 HIGH", out.Summary.Title)
	assert.Equal(t, map[string]int{"high": 1}, out.Summary.Counts)
	assert.Equal(t, "body", out.Summary.Body)
	assert.Equal(t, []byte("raw"), out.Summary.SARIF)
}

func TestToHookArtifactNil(t *testing.T) {
	require.Nil(t, toHookArtifact(nil))
}

func TestToHookSummaryNil(t *testing.T) {
	require.Nil(t, toHookSummary(nil))
}

func TestToHookFindingsEmpty(t *testing.T) {
	require.Nil(t, toHookFindings(nil))
	require.Nil(t, toHookFindings([]scanners.Finding{}))
}

func TestToHookFindingsConvertsFirstAndLast(t *testing.T) {
	findings := []scanners.Finding{
		{Path: "a.tf", Line: 1, Severity: "high", RuleID: "rule-a", Message: "first"},
		{Path: "b.tf", Line: 2, Severity: "medium", RuleID: "rule-b", Message: "middle"},
		{Path: "c.tf", Line: 3, Severity: "low", RuleID: "rule-c", Message: "last"},
	}

	out := toHookFindings(findings)
	require.Len(t, out, 3)
	assert.Equal(t, hooks.Finding{Path: "a.tf", Line: 1, Severity: "high", RuleID: "rule-a", Message: "first"}, out[0])
	assert.Equal(t, hooks.Finding{Path: "c.tf", Line: 3, Severity: "low", RuleID: "rule-c", Message: "last"}, out[2])
}
