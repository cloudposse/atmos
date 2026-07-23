package sarif_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/scanners"
	"github.com/cloudposse/atmos/pkg/scanners/sarif"
	"github.com/cloudposse/atmos/pkg/schema"
)

const sampleSARIF = `{
  "runs": [{
    "tool": {"driver": {"name": "tflint"}},
    "results": [
      {
        "ruleId": "terraform_deprecated_interpolation",
        "level": "warning",
        "message": {"text": "quoted references are deprecated"},
        "properties": {"severity": "HIGH"},
        "locations": [{"physicalLocation": {"artifactLocation": {"uri": "main.tf"}, "region": {"startLine": 12}}}]
      },
      {
        "ruleId": "terraform_unused_declarations",
        "level": "warning",
        "message": {"text": "variable declared but not used"},
        "properties": {"severity": "LOW"}
      }
    ]
  }]
}`

func TestHandler_ParsesFindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(sampleSARIF), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return path },
	})

	s, err := handler(&scanners.Context{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "tflint", s.Kind)
	assert.Equal(t, scanners.StatusWarning, s.Status)
	assert.Contains(t, s.Title, "1 HIGH")
	assert.Contains(t, s.Title, "1 LOW")
	assert.Equal(t, 1, s.Counts["high"])
	assert.Equal(t, 1, s.Counts["low"])
	assert.Contains(t, s.Body, "terraform_deprecated_interpolation")
	assert.Contains(t, s.Body, "main.tf:12")
}

func TestHandler_RichTerminalBodyIncludesSourceExcerpt(t *testing.T) {
	t.Setenv("GITHUB_WORKSPACE", "")
	// Force color off so the asserted literal substrings aren't split by ANSI escape
	// codes. Terminal color-profile detection is environment-dependent (e.g. some CI
	// runners' output is detected as color-capable even without a TTY), and richStyle
	// wraps each styled segment individually, so "[tflint] variables.tf:2:10" only
	// appears contiguously when color is disabled.
	t.Setenv("NO_COLOR", "1")
	base := t.TempDir()
	componentPath := filepath.Join(base, "components", "terraform", "waf")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "variables.tf"), []byte("variable \"example\" {\n  type = optional(string, \"default\")\n}\n"), 0o600))

	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(`{
		"runs": [{"tool": {"driver": {"name": "tflint"}}, "results": [{
			"ruleId": "invalid_type",
			"level": "error",
			"message": {"text": "optional attribute syntax is unsupported"},
			"locations": [{"physicalLocation": {
				"artifactLocation": {"uri": "variables.tf"},
				"region": {"startLine": 2, "startColumn": 10, "endLine": 2, "endColumn": 18}
			}}]
		}]}]
	}`), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:           "tflint",
		OutputPath:     func(_ *scanners.Context) string { return path },
		TerminalFormat: "rich",
	})
	summary, err := handler(&scanners.Context{
		AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: filepath.Join(base, "components", "terraform")},
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "waf", FinalComponent: "waf"},
	})
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Contains(t, summary.Body, "| Severity | Rule | Message | Location |", "shared Markdown output remains available")
	assert.Contains(t, summary.TerminalBody, "[tflint] variables.tf:2:10")
	assert.Contains(t, summary.TerminalBody, "2 |   type = optional(string, \"default\")")
	assert.Contains(t, summary.TerminalBody, "^")
}

func TestHandler_RichTerminalBodyKeepsUnreadableLocation(t *testing.T) {
	t.Setenv("GITHUB_WORKSPACE", "")
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(sampleSARIF), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:           "tflint",
		OutputPath:     func(_ *scanners.Context) string { return path },
		TerminalFormat: "rich",
	})
	summary, err := handler(&scanners.Context{})
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Contains(t, summary.TerminalBody, "main.tf:12")
	assert.Contains(t, summary.TerminalBody, "quoted references are deprecated")
	assert.NotContains(t, summary.TerminalBody, "12 |")
}

func TestHandler_NoFindingsFile(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.sarif")
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return missingPath },
	})
	s, err := handler(&scanners.Context{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "tflint", s.Kind)
	assert.Equal(t, scanners.StatusSuccess, s.Status)
	assert.Equal(t, "no findings", s.Title)
	assert.Contains(t, s.Body, "no findings")
}

func TestHandler_MissingReportAfterScannerFailure(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.sarif")
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return missingPath },
	})

	s, err := handler(&scanners.Context{CommandError: errors.New("exit status 1"), ExitCode: 1})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, scanners.StatusFailure, s.Status)
	assert.Equal(t, "scan failed", s.Title)
	assert.Contains(t, s.Body, "exit code 1")
}

// TestHandler_EmptyOutputFileAfterScannerFailure verifies a 0-byte capture file (the
// state runSubprocess leaves behind when a scanner exits non-zero before writing
// anything) is treated the same as a missing file: routed through missingReportSummary
// rather than silently parsed as zero findings.
func TestHandler_EmptyOutputFileAfterScannerFailure(t *testing.T) {
	emptyPath := filepath.Join(t.TempDir(), "empty.sarif")
	require.NoError(t, os.WriteFile(emptyPath, []byte{}, 0o600))
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return emptyPath },
	})

	s, err := handler(&scanners.Context{CommandError: errors.New("exit status 1"), ExitCode: 1})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, scanners.StatusFailure, s.Status)
	assert.Equal(t, "scan failed", s.Title)
	assert.Contains(t, s.Body, "exit code 1")
}

// TestHandler_EmptyOutputFileCleanRun locks in that a 0-byte capture file after a
// successful (exit 0) run still reports success/no-findings — the fix only changes
// routing for the non-zero-exit case.
func TestHandler_EmptyOutputFileCleanRun(t *testing.T) {
	emptyPath := filepath.Join(t.TempDir(), "empty.sarif")
	require.NoError(t, os.WriteFile(emptyPath, []byte{}, 0o600))
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return emptyPath },
	})

	s, err := handler(&scanners.Context{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, scanners.StatusSuccess, s.Status)
	assert.Equal(t, "no findings", s.Title)
}

func TestHandler_NilOutputPath(t *testing.T) {
	handler := sarif.NewResultHandler(sarif.HandlerOptions{Kind: "tflint"})
	s, err := handler(&scanners.Context{})
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestHandler_NilContext(t *testing.T) {
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return "unused" },
	})
	s, err := handler(nil)
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestHandler_EmptyOutputPathString(t *testing.T) {
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return "" },
	})
	s, err := handler(&scanners.Context{})
	require.NoError(t, err)
	assert.Nil(t, s)
}

func TestHandler_EmptySARIF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(`{"runs":[{"tool":{"driver":{"name":"tflint"}},"results":[]}]}`), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return path },
	})
	s, err := handler(&scanners.Context{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, scanners.StatusSuccess, s.Status)
	assert.Equal(t, "no findings", s.Title)
}

func TestHandler_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte("{not-json"), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return path },
	})
	_, err := handler(&scanners.Context{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrParseFile), "expected ErrParseFile, got %v", err)
}

func TestHandler_ReadErrorUsesStaticError(t *testing.T) {
	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return t.TempDir() },
	})
	_, err := handler(&scanners.Context{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrReadFile), "expected ErrReadFile, got %v", err)
}

func TestHandler_HighestSeverityCriticalElevatesStatus(t *testing.T) {
	body := `{
		"runs": [{
			"tool": {"driver": {"name": "tflint"}},
			"results": [
				{"ruleId": "C1", "level": "error", "properties": {"severity": "CRITICAL"}, "message": {"text": "x"}}
			]
		}]
	}`
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return path },
	})
	s, err := handler(&scanners.Context{})
	require.NoError(t, err)
	assert.Equal(t, scanners.StatusWarning, s.Status, "critical findings warn the run (fail is opt-in via on_failure)")
	assert.Contains(t, s.Title, "CRIT")
}

func TestHandler_KindEmptyFallsBackToSARIFToolName(t *testing.T) {
	body := `{"runs":[{"tool":{"driver":{"name":"tfsec"}},"results":[]}]}`
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		OutputPath: func(_ *scanners.Context) string { return path },
	})
	s, err := handler(&scanners.Context{})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "tfsec", s.Kind)
}

func TestHandler_NormalizesRepoRelativeURI(t *testing.T) {
	// Mirrors the "keeps existing repo-relative uri" case: a scanner that
	// already emits a path relative to the repository root (e.g. trivy).
	workspace := t.TempDir()
	t.Setenv("GITHUB_WORKSPACE", workspace)

	sourceRoot := filepath.Join(workspace, "components", "terraform", "bucket")
	require.NoError(t, os.MkdirAll(sourceRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRoot, "main.tf"), []byte("resource \"aws_s3_bucket\" \"this\" {}\n"), 0o600))

	uri := "components/terraform/bucket/main.tf"
	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(sampleSARIFWithURI("tflint", uri)), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		Kind:       "tflint",
		OutputPath: func(_ *scanners.Context) string { return path },
	})
	s, err := handler(&scanners.Context{
		AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: filepath.Join(workspace, "components", "terraform")},
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "bucket", FinalComponent: "bucket"},
	})
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Len(t, s.Findings, 1)
	assert.Equal(t, uri, s.Findings[0].Path)
}

func TestHandler_LeavesUnsafeAndExternalURIsUnchanged(t *testing.T) {
	tests := []string{
		"https://example.com/main.tf",
		"../main.tf",
	}

	for _, uri := range tests {
		t.Run(uri, func(t *testing.T) {
			workspace := t.TempDir()
			t.Setenv("GITHUB_WORKSPACE", workspace)

			path := filepath.Join(t.TempDir(), "results.sarif")
			require.NoError(t, os.WriteFile(path, []byte(sampleSARIFWithURI("tflint", uri)), 0o600))

			handler := sarif.NewResultHandler(sarif.HandlerOptions{
				Kind:       "tflint",
				OutputPath: func(_ *scanners.Context) string { return path },
			})
			s, err := handler(&scanners.Context{
				AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: filepath.Join(workspace, "components", "terraform")},
				Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "bucket", FinalComponent: "bucket"},
			})
			require.NoError(t, err)
			require.NotNil(t, s)
			assert.Equal(t, uri, s.Findings[0].Path)
		})
	}
}

func TestDefaultOutputFile(t *testing.T) {
	assert.Equal(t, "", sarif.DefaultOutputFile(nil))
	assert.Equal(t, "/tmp/out.sarif", sarif.DefaultOutputFile(&scanners.Context{OutputFile: "/tmp/out.sarif"}))
}

func sampleSARIFWithURI(tool, uri string) string {
	return `{
		"runs": [{
			"tool": {"driver": {"name": "` + tool + `"}},
			"results": [{
				"ruleId": "TEST_RULE",
				"level": "warning",
				"message": {"text": "test finding"},
				"locations": [{
					"physicalLocation": {
						"artifactLocation": {"uri": "` + filepath.ToSlash(uri) + `"},
						"region": {"startLine": 1}
					}
				}]
			}]
		}]
	}`
}
