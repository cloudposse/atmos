package tflint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
