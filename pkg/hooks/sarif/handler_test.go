// Tests live in an external _test package so they can blank-import the
// concrete kind packages (checkov, trivy, kics) without creating an import
// cycle with the kind packages themselves, which depend on sarif.
package sarif_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/checkov" // self-register checkov kind
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/kics"    // self-register kics kind
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/trivy"   // self-register trivy kind
	"github.com/cloudposse/atmos/pkg/hooks/sarif"
)

const sampleSARIF = `{
  "runs": [{
    "tool": {"driver": {"name": "checkov"}},
    "results": [
      {
        "ruleId": "CKV_AWS_19",
        "level": "warning",
        "message": {"text": "S3 bucket encryption"},
        "properties": {"severity": "HIGH"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "main.tf"}, "region": {"startLine": 12}}}]
      },
      {
        "ruleId": "CKV_AWS_18",
        "level": "warning",
        "message": {"text": "Bucket access logging"},
        "properties": {"severity": "LOW"}
      }
    ]
  }]
}`

func TestHandler_ParsesFindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(sampleSARIF), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "checkov",
		OutputPath: func(_ *hooks.ExecContext) string { return path },
	})

	s, err := handler(&hooks.ExecContext{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "checkov", s.Kind)
	assert.Equal(t, hooks.StatusWarning, s.Status)
	assert.Contains(t, s.Title, "1 HIGH")
	assert.Contains(t, s.Title, "1 LOW")
	assert.Equal(t, 1, s.Counts["high"])
	assert.Equal(t, 1, s.Counts["low"])
	assert.Contains(t, s.Body, "CKV_AWS_19")
	assert.Contains(t, s.Body, "main.tf:12")
}

func TestHandler_NoFindingsFile(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.sarif")
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "trivy",
		OutputPath: func(_ *hooks.ExecContext) string { return missingPath },
	})
	s, err := handler(&hooks.ExecContext{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "trivy", s.Kind)
	assert.Equal(t, hooks.StatusSuccess, s.Status)
	assert.Equal(t, "no findings", s.Title)
	assert.Contains(t, s.Body, "no findings")
}

func TestHandler_NilOutputPath(t *testing.T) {
	handler := sarif.NewResultHandler(sarif.HandlerOptions{Kind: "trivy"})
	s, err := handler(&hooks.ExecContext{})
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestHandler_EmptySARIF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(`{"runs":[{"tool":{"driver":{"name":"kics"}},"results":[]}]}`), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "kics",
		OutputPath: func(_ *hooks.ExecContext) string { return path },
	})
	s, err := handler(&hooks.ExecContext{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, hooks.StatusSuccess, s.Status)
	assert.Equal(t, "no findings", s.Title)
}

func TestHandler_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte("{not-json"), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "checkov",
		OutputPath: func(_ *hooks.ExecContext) string { return path },
	})
	_, err := handler(&hooks.ExecContext{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrParseFile), "expected ErrParseFile, got %v", err)
}

func TestHandler_ReadErrorUsesStaticError(t *testing.T) {
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "checkov",
		OutputPath: func(_ *hooks.ExecContext) string { return t.TempDir() },
	})
	_, err := handler(&hooks.ExecContext{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrReadFile), "expected ErrReadFile, got %v", err)
}

func TestHandler_HighestSeverityCriticalElevatesStatus(t *testing.T) {
	body := `{
		"runs": [{
			"tool": {"driver": {"name": "checkov"}},
			"results": [
				{"ruleId": "C1", "level": "error", "properties": {"severity": "CRITICAL"}, "message": {"text": "x"}}
			]
		}]
	}`
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "checkov",
		OutputPath: func(_ *hooks.ExecContext) string { return path },
	})
	s, err := handler(&hooks.ExecContext{})
	require.NoError(t, err)
	assert.Equal(t, hooks.StatusWarning, s.Status, "critical findings warn the run (fail is opt-in via on_failure)")
	assert.Contains(t, s.Title, "CRIT")
}

func TestKindsAreRegistered(t *testing.T) {
	for _, name := range []string{"checkov", "trivy", "kics"} {
		k, ok := hooks.GetKind(name)
		require.True(t, ok, "%s kind must self-register via init()", name)
		assert.Equal(t, name, k.Command, "default command should be the kind name itself for %s", name)
		assert.Equal(t, hooks.OnFailureWarn, k.OnFailure)
		assert.NotNil(t, k.ResultHandler)
	}
}
