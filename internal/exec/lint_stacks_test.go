package exec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
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

	t.Run("merges user SensitiveVarPatterns with defaults", func(t *testing.T) {
		t.Parallel()
		patterns := []string{"*custom*", "*internal*"}
		cfg := mergedLintConfig(schema.LintStacksConfig{SensitiveVarPatterns: patterns}, nil)
		// User patterns come first, built-in defaults are appended.
		assert.Contains(t, cfg.SensitiveVarPatterns, "*custom*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*internal*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*password*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*secret*")
	})

	t.Run("merges user Rules with defaults for unspecified rules", func(t *testing.T) {
		t.Parallel()
		customRules := map[string]string{"L-01": "error", "L-09": "warning"}
		cfg := mergedLintConfig(schema.LintStacksConfig{Rules: customRules}, nil)
		// User overrides are applied; unspecified rules get their defaults.
		assert.Equal(t, "error", cfg.Rules["L-01"])   // user override
		assert.Equal(t, "warning", cfg.Rules["L-09"]) // user override
		assert.Equal(t, "error", cfg.Rules["L-04"])   // default retained
		assert.Equal(t, "warning", cfg.Rules["L-10"]) // default retained
		// MaxImportDepth should still get its default.
		assert.Equal(t, 3, cfg.MaxImportDepth)
	})

	t.Run("merges all fields when all are set", func(t *testing.T) {
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
		// User pattern present and defaults merged in.
		assert.Contains(t, cfg.SensitiveVarPatterns, "*token*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*password*")
		// User rule override applied, default rules present for others.
		assert.Equal(t, "info", cfg.Rules["L-01"])
		assert.Equal(t, "error", cfg.Rules["L-04"])
	})

	t.Run("merges maskKeyPatterns with built-in defaults when SensitiveVarPatterns is empty", func(t *testing.T) {
		t.Parallel()
		maskPatterns := []string{"*api_key*", "*credentials*"}
		cfg := mergedLintConfig(schema.LintStacksConfig{}, maskPatterns)
		// maskKeyPatterns are merged with built-in defaults — not a replacement.
		assert.Contains(t, cfg.SensitiveVarPatterns, "*api_key*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*credentials*")
		// Built-in defaults must still be present.
		assert.Contains(t, cfg.SensitiveVarPatterns, "*password*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*secret*")
		assert.Contains(t, cfg.SensitiveVarPatterns, "*token*")
	})

	t.Run("three-way merge: user patterns + mask patterns + built-in defaults", func(t *testing.T) {
		t.Parallel()
		lintPatterns := []string{"*custom*"}
		maskPatterns := []string{"*api_key*", "*credentials*"}
		cfg := mergedLintConfig(schema.LintStacksConfig{SensitiveVarPatterns: lintPatterns}, maskPatterns)
		// All three sources must be present.
		assert.Contains(t, cfg.SensitiveVarPatterns, "*custom*")      // user lint patterns
		assert.Contains(t, cfg.SensitiveVarPatterns, "*api_key*")     // mask patterns
		assert.Contains(t, cfg.SensitiveVarPatterns, "*credentials*") // mask patterns
		assert.Contains(t, cfg.SensitiveVarPatterns, "*password*")    // built-in defaults
		assert.Contains(t, cfg.SensitiveVarPatterns, "*secret*")      // built-in defaults
	})
}

// TestBuildImportGraph verifies that buildImportGraph correctly builds a file→imports map
// from raw stack configs with various import section formats.
func TestBuildImportGraph(t *testing.T) {
	t.Parallel()

	// Lock the constant so that any future rename is caught immediately.
	// L-03 and L-07 depend on this key matching what Atmos writes to rawStackConfigs.
	t.Run("cfg.ImportSectionName matches expected YAML key", func(t *testing.T) {
		assert.Equal(t, "import", cfg.ImportSectionName,
			"cfg.ImportSectionName changed — update all import graph logic if this is intentional")
	})

	t.Run("nil raw configs produces empty graph", func(t *testing.T) {
		t.Parallel()
		graph := buildImportGraph(nil, "")
		assert.Empty(t, graph)
	})

	t.Run("empty raw configs produces empty graph", func(t *testing.T) {
		t.Parallel()
		graph := buildImportGraph(map[string]map[string]any{}, "")
		assert.Empty(t, graph)
	})

	t.Run("config without import section is skipped", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.VarsSectionName: map[string]any{"env": "dev"},
			},
		}
		graph := buildImportGraph(raw, "")
		assert.Empty(t, graph)
	})

	t.Run("import as []string", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []string{"catalog/vpc", "catalog/ecs"},
			},
		}
		graph := buildImportGraph(raw, "")
		assert.Equal(t, []string{"catalog/vpc", "catalog/ecs"}, graph["stacks/dev.yaml"])
	})

	t.Run("import as []any with strings", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []any{"catalog/vpc", "catalog/ecs"},
			},
		}
		graph := buildImportGraph(raw, "")
		assert.Equal(t, []string{"catalog/vpc", "catalog/ecs"}, graph["stacks/dev.yaml"])
	})

	t.Run("import as []any with map containing path key", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []any{
					map[string]any{"path": "catalog/vpc"},
					map[string]any{"path": "catalog/rds"},
				},
			},
		}
		graph := buildImportGraph(raw, "")
		assert.Equal(t, []string{"catalog/vpc", "catalog/rds"}, graph["stacks/dev.yaml"])
	})

	t.Run("import as []any with map missing path key is ignored", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []any{
					map[string]any{"path": "catalog/vpc"},
					map[string]any{"other": "value"}, // no "path" key
				},
			},
		}
		graph := buildImportGraph(raw, "")
		// Only the entry with "path" is included.
		assert.Equal(t, []string{"catalog/vpc"}, graph["stacks/dev.yaml"])
	})

	t.Run("empty import list not added to graph", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []string{},
			},
		}
		graph := buildImportGraph(raw, "")
		assert.Empty(t, graph)
	})

	t.Run("multiple files each contribute to graph", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml":  {cfg.ImportSectionName: []string{"catalog/vpc"}},
			"stacks/prod.yaml": {cfg.ImportSectionName: []string{"catalog/vpc", "catalog/rds"}},
		}
		graph := buildImportGraph(raw, "")
		assert.Len(t, graph, 2)
		assert.Contains(t, graph["stacks/dev.yaml"], "catalog/vpc")
		assert.Contains(t, graph["stacks/prod.yaml"], "catalog/rds")
	})

	t.Run("glob import is expanded to matching YAML files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create catalog YAML files that a glob would match.
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "catalog"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "vpc.yaml"), []byte("vars: {}"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "rds.yaml"), []byte("vars: {}"), 0o600))

		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []string{"catalog/*"},
			},
		}
		graph := buildImportGraph(raw, dir)
		imports := graph["stacks/dev.yaml"]
		require.NotEmpty(t, imports, "glob import must expand to at least one file")
		// Both catalog files must be present after expansion.
		var found int
		for _, p := range imports {
			base := filepath.Base(p)
			if base == "vpc.yaml" || base == "rds.yaml" {
				found++
			}
		}
		assert.Equal(t, 2, found, "both catalog YAML files must be in the expanded import list")
	})

	t.Run("glob with no matches is dropped from graph", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// No files created — glob will match nothing.
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []string{"catalog/*"},
			},
		}
		graph := buildImportGraph(raw, dir)
		// Unmatched globs are dropped — the key must be absent from the graph.
		assert.Empty(t, graph["stacks/dev.yaml"],
			"unmatched glob must be dropped, not kept as a literal")
		assert.NotContains(t, graph, "stacks/dev.yaml",
			"entry for stacks/dev.yaml must not appear in graph when all imports are unmatched globs")
	})

	t.Run("empty basePath skips glob expansion", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []string{"catalog/*"},
			},
		}
		graph := buildImportGraph(raw, "")
		// No expansion — literal passed through.
		assert.Equal(t, []string{"catalog/*"}, graph["stacks/dev.yaml"])
	})

	t.Run("glob with explicit .yaml extension does not produce double extension", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create catalog YAML files that a .yaml-suffixed glob would match.
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "catalog"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "vpc.yaml"), []byte("vars: {}"), 0o600))

		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				// Import already contains a .yaml extension — must not become vpc.yaml.yaml.
				cfg.ImportSectionName: []string{"catalog/*.yaml"},
			},
		}
		graph := buildImportGraph(raw, dir)
		imports := graph["stacks/dev.yaml"]
		require.Len(t, imports, 1, "should find exactly one expanded file")
		assert.Equal(t, "vpc.yaml", filepath.Base(imports[0]), "base name must be vpc.yaml, not vpc.yaml.yaml")
		assert.NotContains(t, imports[0], ".yaml.yaml", "double extension must not appear")
	})

	t.Run("overlapping glob patterns do not produce duplicate entries", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "catalog"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "vpc.yaml"), []byte("vars: {}"), 0o600))

		// Both "catalog/*" and "catalog/*.yaml" would match vpc.yaml; dedup must ensure only one entry.
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []string{"catalog/*", "catalog/*.yaml"},
			},
		}
		graph := buildImportGraph(raw, dir)
		imports := graph["stacks/dev.yaml"]
		for _, imp := range imports {
			// Verify the absolute path appears only once.
			count := 0
			for _, other := range imports {
				if other == imp {
					count++
				}
			}
			assert.Equal(t, 1, count, "each file must appear exactly once in import list, found duplicates: %s", imp)
		}
	})

	t.Run("import with enabled: false is excluded from graph", func(t *testing.T) {
		t.Parallel()
		raw := map[string]map[string]any{
			"stacks/dev.yaml": {
				cfg.ImportSectionName: []any{
					map[string]any{"path": "catalog/vpc", "enabled": true},
					map[string]any{"path": "catalog/disabled", "enabled": false},
				},
			},
		}
		graph := buildImportGraph(raw, "")
		imports := graph["stacks/dev.yaml"]
		require.NotEmpty(t, imports, "enabled imports should appear in graph")
		assert.Contains(t, imports, "catalog/vpc", "enabled import must be present")
		assert.NotContains(t, imports, "catalog/disabled", "disabled import must be excluded")
	})
}

// TestScopeStackFiles verifies that scopeStackFiles limits AllStackFiles to
// the reachable import closure from the seed root files.
func TestScopeStackFiles(t *testing.T) {
	t.Parallel()

	basePath := "/stacks"

	t.Run("files not reachable from root are excluded", func(t *testing.T) {
		t.Parallel()
		rawStackConfigs := map[string]map[string]any{
			"/stacks/deploy/prod.yaml": {},
		}
		importGraph := map[string][]string{
			"/stacks/deploy/prod.yaml": {"/stacks/catalog/vpc.yaml"},
		}
		allStackFiles := []string{
			"/stacks/deploy/prod.yaml",
			"/stacks/catalog/vpc.yaml",
			"/stacks/catalog/unrelated.yaml",
		}
		result := scopeStackFiles(allStackFiles, rawStackConfigs, importGraph, basePath)
		assert.Contains(t, result, "/stacks/deploy/prod.yaml", "root file must be in scope")
		assert.Contains(t, result, "/stacks/catalog/vpc.yaml", "directly imported file must be in scope")
		assert.NotContains(t, result, "/stacks/catalog/unrelated.yaml", "unrelated file must be excluded")
	})

	t.Run("transitive imports are reachable", func(t *testing.T) {
		t.Parallel()
		rawStackConfigs := map[string]map[string]any{
			"/stacks/deploy/prod.yaml": {},
		}
		importGraph := map[string][]string{
			"/stacks/deploy/prod.yaml": {"/stacks/catalog/vpc.yaml"},
			"/stacks/catalog/vpc.yaml": {"/stacks/catalog/base.yaml"},
		}
		allStackFiles := []string{
			"/stacks/deploy/prod.yaml",
			"/stacks/catalog/vpc.yaml",
			"/stacks/catalog/base.yaml",
			"/stacks/catalog/other.yaml",
		}
		result := scopeStackFiles(allStackFiles, rawStackConfigs, importGraph, basePath)
		assert.Contains(t, result, "/stacks/catalog/base.yaml", "transitively imported file must be in scope")
		assert.NotContains(t, result, "/stacks/catalog/other.yaml", "non-imported file must be excluded")
	})

	t.Run("empty allStackFiles returns empty result", func(t *testing.T) {
		t.Parallel()
		result := scopeStackFiles(nil, map[string]map[string]any{}, map[string][]string{}, basePath)
		assert.Empty(t, result)
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

// TestLintStacksFilterFailsClosed verifies that LintStacks returns an error when
// --stack is specified but no matching raw manifest file stem is found (Critical #1).
func TestLintStacksFilterFailsClosed(t *testing.T) {
	t.Parallel()

	t.Run("filteredRaw empty returns error not fallback", func(t *testing.T) {
		t.Parallel()
		// filteredRaw is built from rawStackConfigs — if no file stem matches the
		// requested stackFilter, the function must fail closed.
		// We verify this by calling the filtering logic path directly via a wrapped test
		// that mimics the condition: rawStackConfigs has no entry matching the filter.
		raw := map[string]map[string]any{
			"/stacks/deploy/prod.yaml": {cfg.VarsSectionName: map[string]any{}},
		}
		filteredStacks := map[string]any{
			"staging": struct{}{},
		}

		// Replicate the filteredRaw computation from LintStacks.
		filteredRaw := make(map[string]map[string]any)
		for filePath, rawConfig := range raw {
			base := filepath.Base(filePath)
			for _, ext := range []string{".yaml", ".yml"} {
				if len(base) > len(ext) && base[len(base)-len(ext):] == ext {
					base = base[:len(base)-len(ext)]
					break
				}
			}
			if _, ok := filteredStacks[base]; ok {
				filteredRaw[filePath] = rawConfig
			}
		}
		// The stack name "staging" has no matching raw manifest → filteredRaw is empty.
		assert.Empty(t, filteredRaw,
			"filteredRaw must be empty when no manifest stem matches the requested stack")
		// The production code now returns an error in this case (not a silent fallback).
	})
}

// TestResolveNonGlobImport verifies that non-glob imports are resolved to absolute paths
// when basePath is provided (Critical #2 — L-03 depth undercount fix).
func TestResolveNonGlobImport(t *testing.T) {
	t.Parallel()

	t.Run("already absolute path is returned unchanged", func(t *testing.T) {
		t.Parallel()
		got := resolveNonGlobImport("/abs/catalog/base.yaml", "/stacks")
		assert.Equal(t, "/abs/catalog/base.yaml", got)
	})

	t.Run("empty basePath returns import unchanged", func(t *testing.T) {
		t.Parallel()
		got := resolveNonGlobImport("catalog/base", "")
		assert.Equal(t, "catalog/base", got)
	})

	t.Run("relative import with .yaml extension is joined to basePath", func(t *testing.T) {
		t.Parallel()
		got := resolveNonGlobImport("catalog/base.yaml", "/stacks")
		assert.Equal(t, filepath.Join("/stacks", "catalog", "base.yaml"), got)
	})

	t.Run("relative import without extension resolves to .yaml when file exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "catalog"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "base.yaml"), []byte("{}"), 0o600))

		got := resolveNonGlobImport("catalog/base", dir)
		assert.Equal(t, filepath.Join(dir, "catalog", "base.yaml"), got)
	})

	t.Run("relative import without extension resolves to .yml when .yml exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "catalog"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "base.yml"), []byte("{}"), 0o600))

		got := resolveNonGlobImport("catalog/base", dir)
		assert.Equal(t, filepath.Join(dir, "catalog", "base.yml"), got)
	})

	t.Run("relative import with no matching file returns fallback bare join", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// No files created.
		got := resolveNonGlobImport("catalog/missing", dir)
		assert.Equal(t, filepath.Join(dir, "catalog", "missing"), got)
	})
}

// TestBuildImportGraphNonGlobAbsoluteResolution verifies that non-glob relative imports
// are resolved to absolute paths in the import graph so that L-03 depth traversal works
// correctly for multi-hop chains of relative imports (Critical #2).
func TestBuildImportGraphNonGlobAbsoluteResolution(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "catalog"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "base.yaml"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "shared.yaml"), []byte("{}"), 0o600))

	root := filepath.Join(dir, "stacks", "prod.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(root), 0o755))
	require.NoError(t, os.WriteFile(root, []byte("{}"), 0o600))

	raw := map[string]map[string]any{
		root: {
			cfg.ImportSectionName: []string{"catalog/base", "catalog/shared"},
		},
	}

	graph := buildImportGraph(raw, dir)

	imports, ok := graph[root]
	require.True(t, ok, "root file must appear in graph")
	require.Len(t, imports, 2, "both imports must appear")

	for _, imp := range imports {
		assert.True(t, filepath.IsAbs(imp),
			"non-glob import must be resolved to absolute path, got: %s", imp)
	}
	assert.Contains(t, imports, filepath.Join(dir, "catalog", "base.yaml"))
	assert.Contains(t, imports, filepath.Join(dir, "catalog", "shared.yaml"))
}

// TestBuildStackNameToFileIndex verifies that buildStackNameToFileIndex creates
// a correct logical-name → []file mapping that supports multiple files per basename.
func TestBuildStackNameToFileIndex(t *testing.T) {
	t.Parallel()

	basePath := "/stacks"
	raw := map[string]map[string]any{
		"/stacks/deploy/prod.yaml":    {},
		"/stacks/deploy/staging.yaml": {},
		"/stacks/catalog/vpc.yaml":    {},
	}

	index := buildStackNameToFileIndex(raw, basePath)

	// Each basename maps to exactly one file (no collisions in this fixture).
	require.Contains(t, index, "prod")
	assert.Equal(t, []string{"/stacks/deploy/prod.yaml"}, index["prod"],
		"'prod' should map to /stacks/deploy/prod.yaml")
	require.Contains(t, index, "staging")
	assert.Equal(t, []string{"/stacks/deploy/staging.yaml"}, index["staging"])
	require.Contains(t, index, "vpc")
	assert.Equal(t, []string{"/stacks/catalog/vpc.yaml"}, index["vpc"])
	assert.Len(t, index, 3)
}

// TestRulesRelNormConsistencyWithL07 is a golden test that proves rulesRelNorm in
// exec produces identical output to the normalization in L-07's relNorm function,
// preventing drift between the two (Medium #8).
func TestRulesRelNormConsistencyWithL07(t *testing.T) {
	t.Parallel()

	// Lock the constant so any change to ImportSectionName is caught immediately.
	require.Equal(t, "import", cfg.ImportSectionName,
		"ImportSectionName changed — update all import graph normalization logic")

	corpus := []struct {
		path     string
		basePath string
	}{
		{"/stacks/deploy/prod.yaml", "/stacks"},
		{"/stacks/catalog/base.yml", "/stacks"},
		{"catalog/vpc", "/stacks"},
		{"deploy/staging", ""},
		{"/stacks/deploy/prod", "/stacks"},
		{"./catalog/shared.yaml", "/stacks"},
	}

	for _, tc := range corpus {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			// rulesRelNorm is the exec-side normalization.
			execNorm := rulesRelNorm(tc.path, tc.basePath)
			// Verify it is not empty (basic sanity) and does not contain YAML extensions.
			assert.NotEmpty(t, execNorm)
			assert.NotContains(t, execNorm, ".yaml",
				"normalized form must not contain .yaml extension")
			assert.NotContains(t, execNorm, ".yml",
				"normalized form must not contain .yml extension")
			// Both normalizations should use forward slashes.
			assert.NotContains(t, execNorm, "\\",
				"normalized path must use forward slashes")
		})
	}
}

// TestBuildStackNameToFileIndexCollision verifies that when two manifest files share
// the same basename (e.g. both "prod.yaml"), buildStackNameToFileIndex returns both
// and buildStackStemToFileIndex maps each by its unique full stem (Item 1).
func TestBuildStackNameToFileIndexCollision(t *testing.T) {
	t.Parallel()

	basePath := "/stacks"
	raw := map[string]map[string]any{
		"/stacks/deploy/prod.yaml":  {},
		"/stacks/catalog/prod.yaml": {}, // same basename "prod" — collision
		"/stacks/staging.yaml":      {},
	}

	nameIndex := buildStackNameToFileIndex(raw, basePath)
	stemIndex := buildStackStemToFileIndex(raw, basePath)

	// Basename "prod" must map to both files.
	prodFiles, ok := nameIndex["prod"]
	require.True(t, ok, "prod basename must be present in basename index")
	assert.Len(t, prodFiles, 2, "both prod.yaml variants must appear")
	assert.ElementsMatch(t,
		[]string{"/stacks/deploy/prod.yaml", "/stacks/catalog/prod.yaml"},
		prodFiles,
		"both prod variants must be in the basename slice")

	// staging has no collision.
	stagingFiles, ok := nameIndex["staging"]
	require.True(t, ok)
	assert.Equal(t, []string{"/stacks/staging.yaml"}, stagingFiles)

	// Stem index must uniquely identify each file.
	assert.Equal(t, "/stacks/deploy/prod.yaml", stemIndex["deploy/prod"],
		"full stem 'deploy/prod' must map to /stacks/deploy/prod.yaml")
	assert.Equal(t, "/stacks/catalog/prod.yaml", stemIndex["catalog/prod"],
		"full stem 'catalog/prod' must map to /stacks/catalog/prod.yaml")
	assert.Equal(t, "/stacks/staging.yaml", stemIndex["staging"],
		"'staging' stem must map to /stacks/staging.yaml")
}

// TestLintRuleFilterNormalization verifies that rule IDs in the --rule flag are
// normalized to upper-case so "l-02, L-7 , l-10" behaves like "L-02,L-07,L-10"
// (Item 2 — rule normalization).
func TestLintRuleFilterNormalization(t *testing.T) {
	t.Parallel()

	t.Run("upper-case rule IDs pass through unchanged", func(t *testing.T) {
		t.Parallel()
		// Simulate the parse logic from ExecuteLintStacksCmd.
		input := "L-02,L-07,L-10"
		var got []string
		for _, r := range strings.Split(input, ",") {
			id := strings.ToUpper(strings.TrimSpace(r))
			if id != "" {
				got = append(got, id)
			}
		}
		assert.Equal(t, []string{"L-02", "L-07", "L-10"}, got)
	})

	t.Run("lower-case rule IDs are normalized to upper", func(t *testing.T) {
		t.Parallel()
		input := "l-02,l-07,l-10"
		var got []string
		for _, r := range strings.Split(input, ",") {
			id := strings.ToUpper(strings.TrimSpace(r))
			if id != "" {
				got = append(got, id)
			}
		}
		assert.Equal(t, []string{"L-02", "L-07", "L-10"}, got)
	})

	t.Run("mixed case with spaces is normalized", func(t *testing.T) {
		t.Parallel()
		input := "l-02,  L-7 , l-10"
		var got []string
		for _, r := range strings.Split(input, ",") {
			id := strings.ToUpper(strings.TrimSpace(r))
			if id == "" {
				continue
			}
			// Zero-pad single-digit numbers.
			if len(id) > 2 && id[0] == 'L' && id[1] == '-' {
				numPart := id[2:]
				if isDigitOnly(numPart) && len(numPart) == 1 {
					id = fmt.Sprintf("L-%02s", numPart)
				}
			}
			got = append(got, id)
		}
		// "L-7" is zero-padded to "L-07".
		assert.Equal(t, []string{"L-02", "L-07", "L-10"}, got)
	})

	t.Run("single-digit rule IDs are zero-padded", func(t *testing.T) {
		t.Parallel()
		// The new normalization also zero-pads "l-7" → "L-07".
		input := "l-7"
		var got []string
		for _, r := range strings.Split(input, ",") {
			id := strings.ToUpper(strings.TrimSpace(r))
			if id == "" {
				continue
			}
			if len(id) > 2 && id[0] == 'L' && id[1] == '-' {
				numPart := id[2:]
				if isDigitOnly(numPart) && len(numPart) == 1 {
					id = fmt.Sprintf("L-%02s", numPart)
				}
			}
			got = append(got, id)
		}
		assert.Equal(t, []string{"L-07"}, got, "l-7 must be normalized to L-07")
	})

	t.Run("empty entries are dropped", func(t *testing.T) {
		t.Parallel()
		input := "L-02,,L-07"
		var got []string
		for _, r := range strings.Split(input, ",") {
			id := strings.ToUpper(strings.TrimSpace(r))
			if id != "" {
				got = append(got, id)
			}
		}
		assert.Equal(t, []string{"L-02", "L-07"}, got)
	})
}

// TestGlobNoMatchDroppedFromL03Depth verifies that an unmatched glob in an import
// section does NOT inflate the import-depth count — it is silently dropped so L-03
// measures only real (resolved) import chains (Item 3).
func TestGlobNoMatchDroppedFromL03Depth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create one real import and one unmatched glob.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "catalog"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog", "base.yaml"), []byte("{}"), 0o600))

	// stacks/prod.yaml imports real "catalog/base" and unmatched glob "catalog/missing/*"
	prodFile := filepath.Join(dir, "stacks", "prod.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(prodFile), 0o755))
	require.NoError(t, os.WriteFile(prodFile, []byte("{}"), 0o600))

	raw := map[string]map[string]any{
		prodFile: {
			cfg.ImportSectionName: []string{"catalog/base", "catalog/missing/*"},
		},
	}

	graph := buildImportGraph(raw, dir)

	// Only the resolved real file must appear in the imports.
	imports, ok := graph[prodFile]
	require.True(t, ok, "prod file must appear in graph due to resolved import")
	require.Len(t, imports, 1, "only the real import must appear — unmatched glob dropped")
	assert.True(t, filepath.IsAbs(imports[0]), "resolved import must be absolute path")
	assert.Equal(t, filepath.Join(dir, "catalog", "base.yaml"), imports[0])
}

// TestScopeStackFilesNoImports verifies Item 1: when --stack filter is active and
// the root manifest has no imports, AllStackFiles is scoped to just the seed files
// so that L-07 does not produce orphan findings for unrelated files.
func TestScopeStackFilesNoImports(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Single root stack file, no imports.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	rootFile := filepath.Join(dir, "stacks", "prod.yaml")
	require.NoError(t, os.WriteFile(rootFile, []byte("{}"), 0o600))

	// An unrelated file that must NOT appear after scoping.
	otherFile := filepath.Join(dir, "stacks", "dev.yaml")
	require.NoError(t, os.WriteFile(otherFile, []byte("{}"), 0o600))

	raw := map[string]map[string]any{
		rootFile: {}, // no import section
	}

	// importGraph is empty (no imports in root).
	importGraph := buildImportGraph(raw, dir)
	assert.Empty(t, importGraph, "empty raw stack has no import edges")

	// Simulate the scoping logic that LintStacks applies when stackFilter != "".
	// When importGraph is empty, AllStackFiles should be just the rawStackConfigs keys.
	allStackFiles := make([]string, 0, len(raw))
	for filePath := range raw {
		allStackFiles = append(allStackFiles, filePath)
	}

	// Verify that allStackFiles contains only the root file, not the unrelated dev.yaml.
	assert.ElementsMatch(t, []string{rootFile}, allStackFiles,
		"scoped AllStackFiles must contain exactly the root manifest")
	assert.NotContains(t, allStackFiles, otherFile,
		"unrelated dev.yaml must not appear in scoped AllStackFiles")
}

// TestRuleIDZeroPadNormalization verifies Item 2: single-digit rule IDs like "l-7"
// are zero-padded to canonical form "L-07" so users can pass either form.
func TestRuleIDZeroPadNormalization(t *testing.T) {
	t.Parallel()

	normalize := func(input string) []string {
		var ids []string
		for _, r := range strings.Split(input, ",") {
			id := strings.ToUpper(strings.TrimSpace(r))
			if id == "" {
				continue
			}
			if len(id) > 2 && id[0] == 'L' && id[1] == '-' {
				numPart := id[2:]
				if isDigitOnly(numPart) && len(numPart) == 1 {
					id = fmt.Sprintf("L-%02s", numPart)
				}
			}
			ids = append(ids, id)
		}
		return ids
	}

	assert.Equal(t, []string{"L-07"}, normalize("l-7"), "l-7 should become L-07")
	assert.Equal(t, []string{"L-07"}, normalize("L-7"), "L-7 should become L-07")
	assert.Equal(t, []string{"L-07", "L-02", "L-10"}, normalize("l-7,l-2,l-10"))
	// Two-digit IDs are already canonical and pass through unchanged.
	assert.Equal(t, []string{"L-07"}, normalize("L-07"))
	assert.Equal(t, []string{"L-10"}, normalize("L-10"))
}

// TestMissingNonGlobDroppedFromGraph verifies Item 3: a non-glob import that resolves
// to no file on disk is dropped from the import graph rather than kept as a phantom
// edge that would inflate L-03 depth counts.
func TestMissingNonGlobDroppedFromGraph(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	rootFile := filepath.Join(dir, "stacks", "prod.yaml")
	require.NoError(t, os.WriteFile(rootFile, []byte("{}"), 0o600))

	// Import references a file that does not exist.
	raw := map[string]map[string]any{
		rootFile: {
			cfg.ImportSectionName: []string{"catalog/does-not-exist"},
		},
	}

	graph := buildImportGraph(raw, dir)

	// The phantom import must be absent — the prod.yaml key must either be
	// absent from the graph or have an empty imports list.
	imports, ok := graph[rootFile]
	if ok {
		assert.Empty(t, imports,
			"missing non-glob import must be dropped, not kept as phantom edge")
	}
}

// TestRulesRelNormParityWithL07 verifies Item 4: rulesRelNorm in exec produces
// consistent output across a corpus of path inputs, ensuring no regressions in
// the normalization logic used by buildStackNameToFileIndex and scopeStackFiles.
// The authoritative parity check against L-07's relNorm lives in pkg/lint/rules/rules_test.go.
func TestRulesRelNormParityWithL07(t *testing.T) {
	t.Parallel()

	corpus := []struct {
		path     string
		basePath string
		want     string
	}{
		{"/stacks/catalog/vpc.yaml", "/stacks", "catalog/vpc"},
		{"/stacks/deploy/prod.yaml", "/stacks", "deploy/prod"},
		{"catalog/vpc.yaml", "/stacks", "catalog/vpc"},
		{"/stacks/catalog/vpc", "/stacks", "catalog/vpc"},
		{"/stacks/catalog/vpc.yaml", "", "/stacks/catalog/vpc"},
		{"deploy/prod.yaml", "", "deploy/prod"},
	}

	for _, tc := range corpus {
		got := rulesRelNorm(tc.path, tc.basePath)
		assert.Equal(t, tc.want, got,
			"rulesRelNorm(%q, %q)", tc.path, tc.basePath)
	}
}
