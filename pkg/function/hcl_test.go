package function

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestHCLFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)

	t.Run("returns all registered functions in atmos namespace", func(t *testing.T) {
		funcs := HCLFunctions(registry, nil)

		// Check that core functions are registered with atmos:: namespace.
		assert.Contains(t, funcs, "atmos::env")
		assert.Contains(t, funcs, "atmos::template")
		assert.Contains(t, funcs, "atmos::repo_root") // Normalized from repo-root.
	})

	t.Run("normalizes function names", func(t *testing.T) {
		assert.Equal(t, "repo_root", normalizeHCLName("repo-root"))
		assert.Equal(t, "env", normalizeHCLName("env"))
		assert.Equal(t, "terraform_output", normalizeHCLName("terraform-output"))
	})
}

func TestHCLEvalContextWithFunctions(t *testing.T) {
	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("creates eval context with namespaced functions", func(t *testing.T) {
		require.NotNil(t, evalCtx)
		require.NotNil(t, evalCtx.Functions)
		assert.Contains(t, evalCtx.Functions, "atmos::env")
	})
}

func TestHCLEnvFunction(t *testing.T) {
	// Set test environment variable.
	t.Setenv("TEST_HCL_VAR", "test_value")

	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("atmos::env returns environment variable", func(t *testing.T) {
		hclContent := `test_var = atmos::env("TEST_HCL_VAR")`
		result := parseHCLWithContext(t, hclContent, evalCtx)

		assert.Equal(t, "test_value", result["test_var"])
	})

	t.Run("atmos::env returns default when var not set", func(t *testing.T) {
		hclContent := `test_var = atmos::env("NONEXISTENT_VAR default_value")`
		result := parseHCLWithContext(t, hclContent, evalCtx)

		assert.Equal(t, "default_value", result["test_var"])
	})
}

func TestHCLWithExecutionContext(t *testing.T) {
	registry := DefaultRegistry(nil)
	execCtx := &ExecutionContext{
		Env: map[string]string{
			"COMPONENT_VAR": "component_value",
		},
	}
	evalCtx := HCLEvalContextWithFunctions(registry, execCtx)

	t.Run("atmos::env uses execution context", func(t *testing.T) {
		hclContent := `test_var = atmos::env("COMPONENT_VAR")`
		result := parseHCLWithContext(t, hclContent, evalCtx)

		assert.Equal(t, "component_value", result["test_var"])
	})
}

func TestToCtyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string // Expected type as string.
	}{
		{"string", "hello", "string"},
		{"int", 42, "number"},
		{"int64", int64(42), "number"},
		{"float64", 3.14, "number"},
		{"bool", true, "bool"},
		{"empty slice", []any{}, "list of string"},
		{"nil", nil, "string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toCtyValue(tt.input)
			assert.NotNil(t, result)
		})
	}
}

func TestHCLRepoRootFunction(t *testing.T) {
	// Set test environment variable for git root override.
	t.Setenv("TEST_GIT_ROOT", "/test/repo/root")

	registry := DefaultRegistry(nil)
	evalCtx := HCLEvalContextWithFunctions(registry, nil)

	t.Run("atmos::repo_root function returns git root", func(t *testing.T) {
		hclContent := `root = atmos::repo_root("")`
		result := parseHCLWithContext(t, hclContent, evalCtx)

		assert.Equal(t, "/test/repo/root", result["root"])
	})
}

// parseHCLWithContext is a helper that parses HCL content with a given eval context.
func parseHCLWithContext(t *testing.T, content string, evalCtx *hcl.EvalContext) map[string]any {
	t.Helper()

	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL([]byte(content), "test.hcl")
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	attrs, diags := file.Body.JustAttributes()
	require.False(t, diags.HasErrors(), "attributes error: %s", diags.Error())

	result := make(map[string]any)
	for name, attr := range attrs {
		val, valDiags := attr.Expr.Value(evalCtx)
		require.False(t, valDiags.HasErrors(), "value error for %s: %s", name, valDiags.Error())

		// Convert cty value to Go.
		switch {
		case val.Type().Equals(cty.String):
			result[name] = val.AsString()
		case val.Type().Equals(cty.Number):
			f, _ := val.AsBigFloat().Float64()
			result[name] = f
		case val.Type().Equals(cty.Bool):
			result[name] = val.True()
		}
	}

	return result
}
