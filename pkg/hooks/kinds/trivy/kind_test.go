package trivy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKindIsRegistered(t *testing.T) {
	kind, ok := hooks.GetKind(kindName)
	require.True(t, ok)

	assert.Equal(t, kindName, kind.Name)
	assert.Equal(t, "trivy", kind.Command)
	assert.Equal(t, []string{
		"config",
		"--format", "sarif",
		"--output", "$ATMOS_OUTPUT_FILE",
		"--quiet",
		"$ATMOS_COMPONENT_PATH",
	}, kind.DefaultArgs)
	assert.Equal(t, hooks.OnFailureWarn, kind.OnFailure)
	assert.NotNil(t, kind.ResultHandler)
	_, ok = kind.Engine.(*hooks.CommandEngine)
	assert.True(t, ok)
}

func TestResultHandlerReadsTrivySARIFFileFromOutputFile(t *testing.T) {
	kind, ok := hooks.GetKind(kindName)
	require.True(t, ok)

	outputFile := filepath.Join(t.TempDir(), "trivy.sarif")
	require.NoError(t, os.WriteFile(outputFile, []byte(`{
		"runs": [{
			"tool": {"driver": {"name": "trivy"}},
			"results": [{
				"ruleId": "AVD-AWS-0089",
				"level": "error",
				"message": {"text": "enable logging"},
				"properties": {"security-severity": "8.5"}
			}]
		}]
	}`), 0o600))

	summary, err := kind.ResultHandler(&hooks.ExecContext{OutputFile: outputFile})
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, kindName, summary.Kind)
	assert.Equal(t, hooks.StatusWarning, summary.Status)
	assert.Equal(t, "1 HIGH", summary.Title)
	assert.Equal(t, 1, summary.Counts["high"])
}
