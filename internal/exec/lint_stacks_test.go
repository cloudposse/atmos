package exec

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/lint"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestMergedLintConfig verifies that mergedLintConfig applies defaults to zero-value fields
// and does not overwrite already-set values.
func TestMergedLintConfig(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults to empty config", func(t *testing.T) {
		t.Parallel()
		cfg := mergedLintConfig(schema.LintStacksConfig{}, nil)
		assert.Equal(t, 3, cfg.MaxImportDepth)
		assert.Equal(t, 80, cfg.DRYThresholdPct)
		assert.NotEmpty(t, cfg.SensitiveVarPatterns)
		// Verify the built-in defaults are applied when both lint patterns and mask key patterns are empty.
		assert.Contains(t, cfg.SensitiveVarPatterns, "*password*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*secret*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*token*")
		assert.NotEmpty(t, cfg.Rules)
		assert.Equal(t, "warning", cfg.Rules["L-01"])
		assert.Equal(t, "error", cfg.Rules["L-04"])
		assert.Equal(t, "error", cfg.Rules["L-09"])
		assert.Equal(t, "warning", cfg.Rules["L-10"])
	})

	t.Run("does not override MaxImportDepth when set", func(t *testing.T) {
		t.Parallel()
		cfg := mergedLintConfig(schema.LintStacksConfig{MaxImportDepth: 10}, nil)
		assert.Equal(t, 10, cfg.MaxImportDepth)
		// Other fields should still get defaults.
		assert.Equal(t, 80, cfg.DRYThresholdPct)
	})

	t.Run("does not override DRYThresholdPct when set", func(t *testing.T) {
		t.Parallel()
		cfg := mergedLintConfig(schema.LintStacksConfig{DRYThresholdPct: 90}, nil)
		assert.Equal(t, 90, cfg.DRYThresholdPct)
	})

	t.Run("does not override SensitiveVarPatterns when set", func(t *testing.T) {
		t.Parallel()
		patterns := []string{"*custom*", "*internal*"}
		cfg := mergedLintConfig(schema.LintStacksConfig{SensitiveVarPatterns: patterns}, nil)
		assert.Equal(t, patterns, cfg.SensitiveVarPatterns)
	})

	t.Run("does not override Rules when set", func(t *testing.T) {
		t.Parallel()
		customRules := map[string]string{"L-01": "error", "L-09": "warning"}
		cfg := mergedLintConfig(schema.LintStacksConfig{Rules: customRules}, nil)
		assert.Equal(t, customRules, cfg.Rules)
		// MaxImportDepth should still get its default.
		assert.Equal(t, 3, cfg.MaxImportDepth)
	})

	t.Run("no defaults applied when all fields are set", func(t *testing.T) {
		t.Parallel()
		input := schema.LintStacksConfig{
			MaxImportDepth:       5,
			DRYThresholdPct:      75,
			SensitiveVarPatterns: []string{"*token*"},
			Rules:                map[string]string{"L-01": "info"},
		}
		cfg := mergedLintConfig(input, nil)
		assert.Equal(t, 5, cfg.MaxImportDepth)
		assert.Equal(t, 75, cfg.DRYThresholdPct)
		assert.Equal(t, []string{"*token*"}, cfg.SensitiveVarPatterns)
		assert.Equal(t, "info", cfg.Rules["L-01"])
	})

	t.Run("uses maskKeyPatterns as defaults when SensitiveVarPatterns is empty", func(t *testing.T) {
		t.Parallel()
		maskPatterns := []string{"*api_key*", "*credentials*"}
		cfg := mergedLintConfig(schema.LintStacksConfig{}, maskPatterns)
		assert.Equal(t, maskPatterns, cfg.SensitiveVarPatterns)
	})

	t.Run("SensitiveVarPatterns takes precedence over maskKeyPatterns", func(t *testing.T) {
		t.Parallel()
		lintPatterns := []string{"*token*"}
		maskPatterns := []string{"*api_key*", "*credentials*"}
		cfg := mergedLintConfig(schema.LintStacksConfig{SensitiveVarPatterns: lintPatterns}, maskPatterns)
		assert.Equal(t, lintPatterns, cfg.SensitiveVarPatterns)
	})
}

// TestBuildImportGraph verifies that buildImportGraph correctly builds a file→imports map
// from raw stack configs with various import section formats.
func TestBuildImportGraph(t *testing.T) {
	t.Parallel()

	t.Run("nil raw configs produces empty graph", func(t *testing.T) {
		t.Parallel()
		graph := buildImportGraph(nil)
		assert.Empty(t, graph)
	})

	t.Run("empty raw configs produces empty graph", func(t *testing.T) {
		t.Parallel()
		graph := buildImportGraph(map[string]map[string]any{})
		assert.Empty(t, graph)
	})

	t.Run("config without import section is skipped", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				"vars": map[string]any{"env": "dev"},
			},
		}
		graph := buildImportGraph(raw)
		assert.Empty(t, graph)
	})

	t.Run("import as []string", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				"import": []string{"catalog/vpc", "catalog/ecs"},
			},
		}
		graph := buildImportGraph(raw)
		assert.Equal(t, []string{"catalog/vpc", "catalog/ecs"}, graph["stacks/dev.yaml"])
	})

	t.Run("import as []any with strings", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				"import": []any{"catalog/vpc", "catalog/ecs"},
			},
		}
		graph := buildImportGraph(raw)
		assert.Equal(t, []string{"catalog/vpc", "catalog/ecs"}, graph["stacks/dev.yaml"])
	})

	t.Run("import as []any with map containing path key", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				"import": []any{
					map[string]any{"path": "catalog/vpc"},
					map[string]any{"path": "catalog/rds"},
				},
			},
		}
		graph := buildImportGraph(raw)
		assert.Equal(t, []string{"catalog/vpc", "catalog/rds"}, graph["stacks/dev.yaml"])
	})

	t.Run("import as []any with map missing path key is ignored", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				"import": []any{
					map[string]any{"path": "catalog/vpc"},
					map[string]any{"other": "value"}, // no "path" key
				},
			},
		}
		graph := buildImportGraph(raw)
		// Only the entry with "path" is included.
		assert.Equal(t, []string{"catalog/vpc"}, graph["stacks/dev.yaml"])
	})

	t.Run("empty import list not added to graph", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				"import": []string{},
			},
		}
		graph := buildImportGraph(raw)
		assert.Empty(t, graph)
	})

	t.Run("multiple files each contribute to graph", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml":  {"import": []string{"catalog/vpc"}},
			"stacks/prod.yaml": {"import": []string{"catalog/vpc", "catalog/rds"}},
		}
		graph := buildImportGraph(raw)
		assert.Len(t, graph, 2)
		assert.Contains(t, graph["stacks/dev.yaml"], "catalog/vpc")
		assert.Contains(t, graph["stacks/prod.yaml"], "catalog/rds")
	})
}

// TestStackYAMLFiles verifies that stackYAMLFiles enumerates YAML files and
// excludes template files.
func TestStackYAMLFiles(t *testing.T) {
	t.Parallel()

	t.Run("empty root returns nil, no error", func(t *testing.T) {
		t.Parallel()
		files, err := stackYAMLFiles("")
		require.NoError(t, err)
		assert.Nil(t, files)
	})

	t.Run("filters out template files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(dir, "stack.yaml"), []byte("vars: {}"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml.tmpl"), []byte("{{ .var }}"), 0o600))

		files, err := stackYAMLFiles(dir)
		require.NoError(t, err)

		// Only the non-template YAML file should be present.
		assert.Len(t, files, 1)
		assert.Contains(t, files[0], "stack.yaml")
		for _, f := range files {
			assert.NotContains(t, f, ".tmpl")
		}
	})

	t.Run("returns absolute paths", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dev.yaml"), []byte("vars: {}"), 0o600))

		files, err := stackYAMLFiles(dir)
		require.NoError(t, err)
		require.Len(t, files, 1)
		assert.True(t, filepath.IsAbs(files[0]), "expected absolute path, got %s", files[0])
	})

	t.Run("empty directory returns empty slice", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		files, err := stackYAMLFiles(dir)
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("multiple yaml files returned", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dev.yaml"), []byte("vars: {}"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "prod.yaml"), []byte("vars: {}"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml.tmpl"), []byte("{{ . }}"), 0o600))

		files, err := stackYAMLFiles(dir)
		require.NoError(t, err)
		assert.Len(t, files, 2)
	})
}

// TestRenderLintText exercises renderLintText code paths.
// The function writes to stderr via ui.* so we only verify it does not panic.
func TestRenderLintText(t *testing.T) {
	t.Run("empty findings prints success message", func(t *testing.T) {
		result := &lint.LintResult{}
		// Should not panic and should print the success message.
		renderLintText(result)
	})

	t.Run("error finding with file and line", func(t *testing.T) {
		result := &lint.LintResult{
			Findings: []lint.LintFinding{
				{
					RuleID:   "L-09",
					Severity: lint.SeverityError,
					Message:  "Inheritance cycle detected",
					File:     "stacks/dev.yaml",
					Line:     12,
					FixHint:  "Remove circular import.",
				},
			},
			Summary: lint.LintSummary{Errors: 1},
		}
		renderLintText(result)
	})

	t.Run("warning finding with file but no line", func(t *testing.T) {
		result := &lint.LintResult{
			Findings: []lint.LintFinding{
				{
					RuleID:   "L-01",
					Severity: lint.SeverityWarning,
					Message:  "Dead variable detected",
					File:     "stacks/prod.yaml",
				},
			},
			Summary: lint.LintSummary{Warnings: 1},
		}
		renderLintText(result)
	})

	t.Run("info finding with no file", func(t *testing.T) {
		result := &lint.LintResult{
			Findings: []lint.LintFinding{
				{
					RuleID:   "L-03",
					Severity: lint.SeverityInfo,
					Message:  "Import depth exceeded",
				},
			},
			Summary: lint.LintSummary{Info: 1},
		}
		renderLintText(result)
	})

	t.Run("mixed findings with fix hints", func(t *testing.T) {
		result := &lint.LintResult{
			Findings: []lint.LintFinding{
				{RuleID: "L-09", Severity: lint.SeverityError, Message: "Cycle", File: "stacks/a.yaml", FixHint: "Fix the cycle."},
				{RuleID: "L-01", Severity: lint.SeverityWarning, Message: "Dead var", File: "stacks/b.yaml"},
				{RuleID: "L-03", Severity: lint.SeverityInfo, Message: "Deep", FixHint: "Flatten imports."},
				{RuleID: "L-05", Severity: lint.SeverityInfo, Message: "No file or hint"},
			},
			Summary: lint.LintSummary{Errors: 1, Warnings: 1, Info: 2},
		}
		renderLintText(result)
	})
}

// TestRenderLintJSON verifies that renderLintJSON encodes the result as valid JSON on stdout.
func TestRenderLintJSON(t *testing.T) {
	// Capture stdout during the call.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	result := &lint.LintResult{
		Findings: []lint.LintFinding{
			{RuleID: "L-01", Severity: lint.SeverityWarning, Message: "Dead variable"},
		},
		Summary: lint.LintSummary{Warnings: 1},
	}
	renderErr := renderLintJSON(result)

	w.Close()
	os.Stdout = origStdout

	require.NoError(t, renderErr)

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	var decoded lint.LintResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Len(t, decoded.Findings, 1)
	assert.Equal(t, "L-01", decoded.Findings[0].RuleID)
	assert.Equal(t, 1, decoded.Summary.Warnings)
}
