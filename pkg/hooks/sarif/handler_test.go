// Tests live in an external _test package so they can blank-import the
// concrete kind packages (checkov, trivy, kics) without creating an import
// cycle with the kind packages themselves, which depend on sarif.
package sarif_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/checkov" // self-register checkov kind
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/kics"    // self-register kics kind
	_ "github.com/cloudposse/atmos/pkg/hooks/kinds/trivy"   // self-register trivy kind
	"github.com/cloudposse/atmos/pkg/hooks/sarif"
	"github.com/cloudposse/atmos/pkg/schema"
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

func TestHandler_NormalizesArtifactURIsForBuiltInKinds(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("GITHUB_WORKSPACE", workspace)

	sourceRoot := filepath.Join(workspace, "components", "terraform", "bucket")
	require.NoError(t, os.MkdirAll(sourceRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRoot, "main.tf"), []byte("resource \"aws_s3_bucket\" \"this\" {}\n"), 0o600))

	ctx := &hooks.ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{
			BasePath:                 workspace,
			TerraformDirAbsolutePath: filepath.Join(workspace, "components", "terraform"),
		},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "test",
			ComponentFromArg: "bucket",
			FinalComponent:   "bucket",
			ComponentSection: map[string]any{},
		},
	}

	workdirRoot, _, err := component.BuildAndResolveWorkdirPath(ctx.AtmosConfig, ctx.Info, cfg.TerraformComponentType)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(workdirRoot, 0o755))

	tests := []struct {
		name       string
		kind       string
		writeSARIF func(string)
		uri        string
	}{
		{
			name: "checkov resolves scanner-relative uri",
			kind: "checkov",
			writeSARIF: func(body string) {
				ctx.OutputDir = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(ctx.OutputDir, "results_sarif.sarif"), []byte(body), 0o600))
			},
			uri: "main.tf",
		},
		{
			name: "trivy keeps existing repo-relative uri",
			kind: "trivy",
			writeSARIF: func(body string) {
				ctx.OutputFile = filepath.Join(t.TempDir(), "output.sarif")
				require.NoError(t, os.WriteFile(ctx.OutputFile, []byte(body), 0o600))
			},
			uri: "components/terraform/bucket/main.tf",
		},
		{
			name: "kics maps absolute scan-root uri to source component",
			kind: "kics",
			writeSARIF: func(body string) {
				ctx.OutputDir = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(ctx.OutputDir, "results.sarif"), []byte(body), 0o600))
			},
			uri: filepath.Join(workdirRoot, "main.tf"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, ok := hooks.GetKind(tt.kind)
			require.True(t, ok)
			require.NotNil(t, kind.ResultHandler)

			tt.writeSARIF(sampleSARIFWithURI(tt.kind, tt.uri))
			s, err := kind.ResultHandler(ctx)
			require.NoError(t, err)
			require.NotNil(t, s)
			require.Len(t, s.Findings, 1)
			assert.Equal(t, "components/terraform/bucket/main.tf", s.Findings[0].Path)
			assert.Equal(t, "components/terraform/bucket/main.tf", firstSARIFURI(t, s.SARIF))
		})
	}
}

func TestHandler_NormalizesAbsoluteWorkspaceURI(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("GITHUB_WORKSPACE", workspace)

	sourceRoot := filepath.Join(workspace, "components", "terraform", "bucket")
	require.NoError(t, os.MkdirAll(sourceRoot, 0o755))
	absURI := filepath.Join(sourceRoot, "main.tf")
	require.NoError(t, os.WriteFile(absURI, []byte("resource \"aws_s3_bucket\" \"this\" {}\n"), 0o600))

	path := filepath.Join(t.TempDir(), "results.sarif")
	require.NoError(t, os.WriteFile(path, []byte(sampleSARIFWithURI("tfsec", absURI)), 0o600))

	handler := sarif.NewResultHandler(sarif.HandlerOptions{
		OutputPath: func(_ *hooks.ExecContext) string { return path },
	})
	s, err := handler(&hooks.ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: filepath.Join(workspace, "components", "terraform")},
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "bucket", FinalComponent: "bucket"},
	})
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "components/terraform/bucket/main.tf", s.Findings[0].Path)
	assert.Equal(t, "components/terraform/bucket/main.tf", firstSARIFURI(t, s.SARIF))
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
			require.NoError(t, os.WriteFile(path, []byte(sampleSARIFWithURI("tfsec", uri)), 0o600))

			handler := sarif.NewResultHandler(sarif.HandlerOptions{
				OutputPath: func(_ *hooks.ExecContext) string { return path },
			})
			s, err := handler(&hooks.ExecContext{
				AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: filepath.Join(workspace, "components", "terraform")},
				Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "bucket", FinalComponent: "bucket"},
			})
			require.NoError(t, err)
			require.NotNil(t, s)
			assert.Equal(t, uri, s.Findings[0].Path)
			assert.Equal(t, uri, firstSARIFURI(t, s.SARIF))
		})
	}
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

func sampleSARIFWithURI(tool, uri string) string {
	body, err := json.Marshal(map[string]any{
		"runs": []any{
			map[string]any{
				"tool": map[string]any{
					"driver": map[string]any{"name": tool},
				},
				"results": []any{
					map[string]any{
						"ruleId":  "TEST_RULE",
						"level":   "warning",
						"message": map[string]any{"text": "test finding"},
						"locations": []any{
							map[string]any{
								"physicalLocation": map[string]any{
									"artifactLocation": map[string]any{"uri": filepath.ToSlash(uri)},
									"region":           map[string]any{"startLine": 1},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return string(body)
}

func firstSARIFURI(t *testing.T, body []byte) string {
	t.Helper()
	var doc struct {
		Runs []struct {
			Results []struct {
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(body, &doc))
	require.NotEmpty(t, doc.Runs)
	require.NotEmpty(t, doc.Runs[0].Results)
	require.NotEmpty(t, doc.Runs[0].Results[0].Locations)
	return doc.Runs[0].Results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI
}
