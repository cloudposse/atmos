package kics

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
	assert.Equal(t, "kics", kind.Command)
	assert.Equal(t, []string{
		"scan",
		"-p", "$ATMOS_COMPONENT_PATH",
		"-o", "$ATMOS_OUTPUT_DIR",
		"--report-formats", "sarif",
		"--no-progress",
	}, kind.DefaultArgs)
	assert.Equal(t, hooks.OnFailureWarn, kind.OnFailure)
	assert.NotNil(t, kind.ResultHandler)
	_, ok = kind.Engine.(*hooks.CommandEngine)
	assert.True(t, ok)
}

func TestResultHandlerReadsKICSSARIFFileFromOutputDir(t *testing.T) {
	kind, ok := hooks.GetKind(kindName)
	require.True(t, ok)

	summary, err := kind.ResultHandler(&hooks.ExecContext{})
	require.NoError(t, err)
	assert.Nil(t, summary)

	outputDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outputDir, resultFileName), []byte(`{
		"runs": [{
			"tool": {"driver": {"name": "kics"}},
			"results": [{
				"ruleId": "KICS_001",
				"level": "warning",
				"message": {"text": "encrypt bucket"},
				"properties": {"severity": "HIGH"}
			}]
		}]
	}`), 0o600))

	summary, err = kind.ResultHandler(&hooks.ExecContext{OutputDir: outputDir})
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, kindName, summary.Kind)
	assert.Equal(t, hooks.StatusWarning, summary.Status)
	assert.Equal(t, "1 HIGH", summary.Title)
	assert.Equal(t, 1, summary.Counts["high"])
}
