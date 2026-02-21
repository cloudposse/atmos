package yaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "gopkg.in/yaml.v3"

	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestDelimiterConflictsWithYAMLQuoting tests the detection of delimiter/YAML quoting conflicts.
func TestDelimiterConflictsWithYAMLQuoting(t *testing.T) {
	tests := []struct {
		name       string
		delimiters []string
		expected   bool
	}{
		{
			name:       "default delimiters have no conflict",
			delimiters: []string{"{{", "}}"},
			expected:   false,
		},
		{
			name:       "left delimiter with single quote conflicts",
			delimiters: []string{"'{{", "}}'"},
			expected:   true,
		},
		{
			name:       "right delimiter with single quote conflicts",
			delimiters: []string{"{{", "}}'"},
			expected:   true,
		},
		{
			name:       "both delimiters with single quotes conflict",
			delimiters: []string{"'{{", "}}'"},
			expected:   true,
		},
		{
			name:       "nil delimiters have no conflict",
			delimiters: nil,
			expected:   false,
		},
		{
			name:       "empty delimiters have no conflict",
			delimiters: []string{},
			expected:   false,
		},
		{
			name:       "single element delimiters have no conflict",
			delimiters: []string{"{{"},
			expected:   false,
		},
		{
			name:       "dollar brace delimiters have no conflict",
			delimiters: []string{"${", "}"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DelimiterConflictsWithYAMLQuoting(tt.delimiters)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEnsureDoubleQuotedForDelimiterSafety tests the node style modification logic.
func TestEnsureDoubleQuotedForDelimiterSafety(t *testing.T) {
	t.Run("changes single-quote-containing scalars to double-quoted", func(t *testing.T) {
		node := &goyaml.Node{
			Kind:  goyaml.ScalarNode,
			Tag:   "!!str",
			Value: "!terraform.state vpc '{{ .stack }}' vpc_id",
		}

		EnsureDoubleQuotedForDelimiterSafety(node)
		assert.Equal(t, goyaml.DoubleQuotedStyle, node.Style)
	})

	t.Run("does not change scalars without single quotes", func(t *testing.T) {
		node := &goyaml.Node{
			Kind:  goyaml.ScalarNode,
			Tag:   "!!str",
			Value: "!terraform.state vpc {{ .stack }} vpc_id",
		}

		EnsureDoubleQuotedForDelimiterSafety(node)
		assert.Equal(t, goyaml.Style(0), node.Style)
	})

	t.Run("recursively processes mapping nodes", func(t *testing.T) {
		keyNode := &goyaml.Node{
			Kind:  goyaml.ScalarNode,
			Tag:   "!!str",
			Value: "test",
		}
		valueNode := &goyaml.Node{
			Kind:  goyaml.ScalarNode,
			Tag:   "!!str",
			Value: "!terraform.state component '{{ .stack }}' output",
		}
		mappingNode := &goyaml.Node{
			Kind:    goyaml.MappingNode,
			Content: []*goyaml.Node{keyNode, valueNode},
		}

		EnsureDoubleQuotedForDelimiterSafety(mappingNode)
		assert.Equal(t, goyaml.Style(0), keyNode.Style)
		assert.Equal(t, goyaml.DoubleQuotedStyle, valueNode.Style)
	})

	t.Run("recursively processes sequence nodes", func(t *testing.T) {
		item1 := &goyaml.Node{
			Kind:  goyaml.ScalarNode,
			Tag:   "!!str",
			Value: "no quotes here",
		}
		item2 := &goyaml.Node{
			Kind:  goyaml.ScalarNode,
			Tag:   "!!str",
			Value: "has 'quotes' inside",
		}
		seqNode := &goyaml.Node{
			Kind:    goyaml.SequenceNode,
			Content: []*goyaml.Node{item1, item2},
		}

		EnsureDoubleQuotedForDelimiterSafety(seqNode)
		assert.Equal(t, goyaml.Style(0), item1.Style)
		assert.Equal(t, goyaml.DoubleQuotedStyle, item2.Style)
	})

	t.Run("handles nil node gracefully", func(t *testing.T) {
		assert.NotPanics(t, func() {
			EnsureDoubleQuotedForDelimiterSafety(nil)
		})
	})

	t.Run("handles document node", func(t *testing.T) {
		valueNode := &goyaml.Node{
			Kind:  goyaml.ScalarNode,
			Tag:   "!!str",
			Value: "value with 'quotes'",
		}
		docNode := &goyaml.Node{
			Kind:    goyaml.DocumentNode,
			Content: []*goyaml.Node{valueNode},
		}

		EnsureDoubleQuotedForDelimiterSafety(docNode)
		assert.Equal(t, goyaml.DoubleQuotedStyle, valueNode.Style)
	})
}

// TestConvertToYAMLPreservingDelimiters tests end-to-end YAML serialization
// with delimiter-safe quoting.
func TestConvertToYAMLPreservingDelimiters(t *testing.T) {
	t.Run("preserves single-quote delimiters in YAML function values", func(t *testing.T) {
		// This is the core test for GitHub issue #2052.
		// When custom delimiters contain single quotes (e.g., ["'{{", "}}'"]),
		// YAML function values like "!terraform.state vpc '{{ .stack }}' vpc_id"
		// must be serialized with double-quoted style to prevent single-quote
		// escaping from breaking the template delimiters.
		data := map[string]interface{}{
			"vars": map[string]interface{}{
				"test_val": "!terraform.state vpc '{{ .stack }}' vpc_id",
			},
		}

		delimiters := []string{"'{{", "}}'"}
		result, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
		require.NoError(t, err)

		// The value should be double-quoted, preserving the single quotes literally.
		assert.Contains(t, result, `"!terraform.state vpc '{{ .stack }}' vpc_id"`)
		// It should NOT contain single-quoted escaping ('').
		assert.NotContains(t, result, "''{{")
	})

	t.Run("falls back to standard encoding for default delimiters", func(t *testing.T) {
		data := map[string]interface{}{
			"vars": map[string]interface{}{
				"test_val": "!terraform.state vpc {{ .stack }} vpc_id",
			},
		}

		delimiters := []string{"{{", "}}"}
		result, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
		require.NoError(t, err)
		assert.NotEmpty(t, result)

		// Verify it's valid YAML.
		var parsed map[string]interface{}
		err = goyaml.Unmarshal([]byte(result), &parsed)
		require.NoError(t, err)
	})

	t.Run("falls back to standard encoding for nil delimiters", func(t *testing.T) {
		data := map[string]interface{}{
			"key": "value",
		}

		result, err := ConvertToYAMLPreservingDelimiters(data, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})

	t.Run("preserves all values correctly after double-quoting", func(t *testing.T) {
		// Verify that the YAML content is semantically identical
		// after the quoting style change.
		data := map[string]interface{}{
			"vars": map[string]interface{}{
				"with_quotes":    "!terraform.state vpc '{{ .stack }}' vpc_id",
				"without_quotes": "!terraform.state vpc vpc_id",
				"normal_value":   "just a regular string",
				"template_value": "value with '{{ .stack }}' inside",
			},
		}

		delimiters := []string{"'{{", "}}'"}
		result, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
		require.NoError(t, err)

		// Parse the result back and verify values are preserved.
		var parsed map[string]interface{}
		err = goyaml.Unmarshal([]byte(result), &parsed)
		require.NoError(t, err)

		vars := parsed["vars"].(map[string]interface{})
		assert.Equal(t, "!terraform.state vpc '{{ .stack }}' vpc_id", vars["with_quotes"])
		assert.Equal(t, "!terraform.state vpc vpc_id", vars["without_quotes"])
		assert.Equal(t, "just a regular string", vars["normal_value"])
		assert.Equal(t, "value with '{{ .stack }}' inside", vars["template_value"])
	})

	t.Run("template replacement produces valid YAML with custom delimiters", func(t *testing.T) {
		// Simulate the full template processing pipeline:
		// 1. Serialize to YAML with delimiter-safe quoting.
		// 2. Replace template expressions (as the Go template engine would).
		// 3. Parse the result back as YAML.
		data := map[string]interface{}{
			"vars": map[string]interface{}{
				"test_val": "!terraform.state vpc '{{ .stack }}' vpc_id",
			},
		}

		delimiters := []string{"'{{", "}}'"}
		yamlStr, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
		require.NoError(t, err)

		// Simulate template replacement: '{{ .stack }}' -> nonprod.
		processed := strings.ReplaceAll(yamlStr, "'{{ .stack }}'", "nonprod")

		// The result should be valid YAML.
		var parsed map[string]interface{}
		err = goyaml.Unmarshal([]byte(processed), &parsed)
		require.NoError(t, err, "YAML should be valid after template replacement, got: %s", processed)

		vars := parsed["vars"].(map[string]interface{})
		assert.Equal(t, "!terraform.state vpc nonprod vpc_id", vars["test_val"])
	})

	t.Run("standard encoding breaks with custom delimiters", func(t *testing.T) {
		// This test demonstrates the bug that the fix addresses.
		// With standard ConvertToYAML, single-quote escaping breaks template delimiters.
		data := map[string]interface{}{
			"vars": map[string]interface{}{
				"test_val": "!terraform.state vpc '{{ .stack }}' vpc_id",
			},
		}

		yamlStr, err := u.ConvertToYAML(data)
		require.NoError(t, err)

		// Standard encoding uses single quotes for strings starting with '!',
		// which escapes internal single quotes as ''.
		assert.Contains(t, yamlStr, "''{{", "standard encoding should escape single quotes")

		// Simulate template replacement with custom delimiters '{{ and }}'.
		// The template engine would look for '{{ and }}' in the raw YAML text.
		// It finds ''{{ (YAML-escaped) and replaces it, breaking the YAML.
		processed := strings.ReplaceAll(yamlStr, "'{{ .stack }}'", "nonprod")

		// The result should be INVALID YAML because single-quote escaping is broken.
		var parsed map[string]interface{}
		err = goyaml.Unmarshal([]byte(processed), &parsed)
		assert.Error(t, err, "standard encoding should produce invalid YAML after template replacement with custom delimiters")
	})

	t.Run("handles nested maps with YAML function values", func(t *testing.T) {
		data := map[string]interface{}{
			"outer": map[string]interface{}{
				"inner": map[string]interface{}{
					"deep_value": "!terraform.state component '{{ .stack }}' output",
				},
			},
		}

		delimiters := []string{"'{{", "}}'"}
		result, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
		require.NoError(t, err)

		// Verify the deep value is double-quoted.
		assert.Contains(t, result, `"!terraform.state component '{{ .stack }}' output"`)
	})

	t.Run("handles lists with YAML function values", func(t *testing.T) {
		data := map[string]interface{}{
			"items": []interface{}{
				"!terraform.state component '{{ .stack }}' output1",
				"no quotes here",
				"!terraform.state component '{{ .stack }}' output2",
			},
		}

		delimiters := []string{"'{{", "}}'"}
		result, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
		require.NoError(t, err)

		// Both values with quotes should be double-quoted.
		assert.Contains(t, result, `"!terraform.state component '{{ .stack }}' output1"`)
		assert.Contains(t, result, `"!terraform.state component '{{ .stack }}' output2"`)

		// Parse back and verify.
		var parsed map[string]interface{}
		err = goyaml.Unmarshal([]byte(result), &parsed)
		require.NoError(t, err)
	})

	t.Run("respects custom indent option", func(t *testing.T) {
		data := map[string]interface{}{
			"vars": map[string]interface{}{
				"test": "value with 'quotes'",
			},
		}

		delimiters := []string{"'{{", "}}'"}
		result, err := ConvertToYAMLPreservingDelimiters(data, delimiters, u.YAMLOptions{Indent: 4})
		require.NoError(t, err)

		// Verify the indentation is 4 spaces.
		assert.Contains(t, result, "    test:")
	})

	t.Run("handles empty data", func(t *testing.T) {
		data := map[string]interface{}{}

		delimiters := []string{"'{{", "}}'"}
		result, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})
}

// TestAllYAMLFunctionsPreservedWithCustomDelimiters verifies that the fix applies
// to ALL Atmos YAML functions, not just !terraform.state. The fix is generic at
// the YAML serialization level — it forces double-quoted style for any scalar
// containing single quotes — but this test explicitly covers every function prefix.
func TestAllYAMLFunctionsPreservedWithCustomDelimiters(t *testing.T) {
	delimiters := []string{"'{{", "}}'"}

	// All YAML function prefixes that accept arguments which may contain template expressions.
	yamlFunctions := []struct {
		name  string
		value string
	}{
		{
			name:  "terraform.state with templated stack arg",
			value: "!terraform.state vpc '{{ .stack }}' vpc_id",
		},
		{
			name:  "terraform.output with templated stack arg",
			value: "!terraform.output vpc '{{ .stack }}' vpc_id",
		},
		{
			name:  "store with templated stack",
			value: "!store my-store '{{ .stack }}' key",
		},
		{
			name:  "store.get with templated stack",
			value: "!store.get my-store '{{ .stack }}' key",
		},
		{
			name:  "env with templated var name",
			value: "!env '{{ .settings.env_var_name }}'",
		},
		{
			name:  "exec with templated command",
			value: "!exec echo '{{ .stack }}'",
		},
		{
			name:  "template with single quotes in JSON",
			value: `!template {"key": "'{{ .stack }}'"}`,
		},
		{
			name:  "include with templated path",
			value: "!include configs/'{{ .stack }}'.yaml",
		},
		{
			name:  "include.raw with templated path",
			value: "!include.raw configs/'{{ .stack }}'.txt",
		},
		{
			name:  "repo-root (no args but with quotes in context)",
			value: "!repo-root",
		},
		{
			name:  "cwd (no args but with quotes in context)",
			value: "!cwd",
		},
		{
			name:  "random with templated params",
			value: "!random int 1 '{{ .settings.max }}'",
		},
	}

	for _, fn := range yamlFunctions {
		t.Run(fn.name, func(t *testing.T) {
			data := map[string]interface{}{
				"vars": map[string]interface{}{
					"test_val": fn.value,
				},
			}

			result, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
			require.NoError(t, err)

			// Values containing single quotes should use double-quoted style,
			// preserving the delimiter characters literally.
			if strings.Contains(fn.value, "'") {
				assert.NotContains(t, result, "''{{",
					"YAML function %q should not have single-quote escaping that breaks delimiters", fn.name)

				// Verify the value is preserved by parsing back.
				var parsed map[string]interface{}
				err = goyaml.Unmarshal([]byte(result), &parsed)
				require.NoError(t, err)

				vars := parsed["vars"].(map[string]interface{})
				assert.Equal(t, fn.value, vars["test_val"],
					"YAML function %q value should round-trip correctly", fn.name)
			}
		})
	}
}

// TestAllYAMLFunctionsTemplateReplacementWithCustomDelimiters simulates the full
// template processing pipeline for each YAML function: serialize → replace → parse.
// This proves that after template replacement, the YAML remains valid for all functions.
func TestAllYAMLFunctionsTemplateReplacementWithCustomDelimiters(t *testing.T) {
	delimiters := []string{"'{{", "}}'"}

	// Each test case has a YAML function value with a template expression,
	// and the expected value after the template replacement.
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{
			name:     "terraform.state",
			value:    "!terraform.state vpc '{{ .stack }}' vpc_id",
			expected: "!terraform.state vpc nonprod vpc_id",
		},
		{
			name:     "terraform.output",
			value:    "!terraform.output vpc '{{ .stack }}' vpc_id",
			expected: "!terraform.output vpc nonprod vpc_id",
		},
		{
			name:     "store",
			value:    "!store my-store '{{ .stack }}' key",
			expected: "!store my-store nonprod key",
		},
		{
			name:     "store.get",
			value:    "!store.get my-store '{{ .stack }}' key",
			expected: "!store.get my-store nonprod key",
		},
		{
			name:     "env",
			value:    "!env MY_VAR_'{{ .stack }}'",
			expected: "!env MY_VAR_nonprod",
		},
		{
			name:     "exec",
			value:    "!exec echo '{{ .stack }}'",
			expected: "!exec echo nonprod",
		},
		{
			name:     "include",
			value:    "!include configs/'{{ .stack }}'.yaml",
			expected: "!include configs/nonprod.yaml",
		},
		{
			name:     "include.raw",
			value:    "!include.raw configs/'{{ .stack }}'.txt",
			expected: "!include.raw configs/nonprod.txt",
		},
		{
			name:     "random",
			value:    "!random int '{{ .settings.min }}' 100",
			expected: "!random int nonprod 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]interface{}{
				"vars": map[string]interface{}{
					"test_val": tt.value,
				},
			}

			// Step 1: Serialize with delimiter-safe quoting.
			yamlStr, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
			require.NoError(t, err)

			// Step 2: Simulate template replacement ('{{ .stack }}' → nonprod, '{{ .settings.min }}' → nonprod).
			processed := yamlStr
			processed = strings.ReplaceAll(processed, "'{{ .stack }}'", "nonprod")
			processed = strings.ReplaceAll(processed, "'{{ .settings.min }}'", "nonprod")
			processed = strings.ReplaceAll(processed, "'{{ .settings.max }}'", "nonprod")
			processed = strings.ReplaceAll(processed, "'{{ .settings.env_var_name }}'", "nonprod")

			// Step 3: Parse the result back as YAML — it must be valid.
			var parsed map[string]interface{}
			err = goyaml.Unmarshal([]byte(processed), &parsed)
			require.NoError(t, err, "YAML should be valid after template replacement for %s, got: %s", tt.name, processed)

			vars := parsed["vars"].(map[string]interface{})
			assert.Equal(t, tt.expected, vars["test_val"],
				"value should match after template replacement for %s", tt.name)
		})
	}
}

// TestStandardEncodingBreaksAllYAMLFunctionsWithCustomDelimiters demonstrates that
// the bug (standard ConvertToYAML breaking custom delimiters) affects ALL YAML
// functions that start with '!', not just !terraform.state.
func TestStandardEncodingBreaksAllYAMLFunctionsWithCustomDelimiters(t *testing.T) {
	// All YAML function values starting with '!' that also contain single-quote delimiters.
	// yaml.v3 will use single-quoted style for all of these, breaking the delimiters.
	functions := []struct {
		name  string
		value string
	}{
		{"terraform.state", "!terraform.state vpc '{{ .stack }}' vpc_id"},
		{"terraform.output", "!terraform.output vpc '{{ .stack }}' vpc_id"},
		{"store", "!store my-store '{{ .stack }}' key"},
		{"store.get", "!store.get my-store '{{ .stack }}' key"},
		{"env", "!env MY_VAR_'{{ .stack }}'"},
		{"exec", "!exec echo '{{ .stack }}'"},
		{"include", "!include configs/'{{ .stack }}'.yaml"},
		{"include.raw", "!include.raw configs/'{{ .stack }}'.txt"},
		{"random", "!random int '{{ .stack }}' 100"},
	}

	for _, fn := range functions {
		t.Run(fn.name+" breaks with standard encoding", func(t *testing.T) {
			data := map[string]interface{}{
				"vars": map[string]interface{}{
					"test_val": fn.value,
				},
			}

			// Standard encoding will use single-quoted style for strings starting with '!'.
			yamlStr, err := u.ConvertToYAML(data)
			require.NoError(t, err)

			// Should contain '' (escaped single quotes).
			assert.Contains(t, yamlStr, "''",
				"standard encoding should escape single quotes for %s", fn.name)

			// After template replacement, the YAML should be INVALID.
			processed := strings.ReplaceAll(yamlStr, "'{{ .stack }}'", "nonprod")
			var parsed map[string]interface{}
			err = goyaml.Unmarshal([]byte(processed), &parsed)
			assert.Error(t, err,
				"standard encoding should produce invalid YAML after template replacement for %s", fn.name)
		})

		t.Run(fn.name+" works with delimiter-safe encoding", func(t *testing.T) {
			data := map[string]interface{}{
				"vars": map[string]interface{}{
					"test_val": fn.value,
				},
			}

			delimiters := []string{"'{{", "}}'"}
			yamlStr, err := ConvertToYAMLPreservingDelimiters(data, delimiters)
			require.NoError(t, err)

			// Should NOT contain escaped single quotes.
			assert.NotContains(t, yamlStr, "''{{",
				"delimiter-safe encoding should not escape single quotes for %s", fn.name)

			// After template replacement, the YAML should be VALID.
			processed := strings.ReplaceAll(yamlStr, "'{{ .stack }}'", "nonprod")
			var parsed map[string]interface{}
			err = goyaml.Unmarshal([]byte(processed), &parsed)
			require.NoError(t, err,
				"delimiter-safe encoding should produce valid YAML after template replacement for %s", fn.name)
		})
	}
}
