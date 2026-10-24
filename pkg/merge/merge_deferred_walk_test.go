package merge

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAtmosYAMLFunction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "detects !template function",
			input:    "!template '{{ .settings.base }}'",
			expected: true,
		},
		{
			name:     "detects !terraform.output function",
			input:    "!terraform.output vpc.id",
			expected: true,
		},
		{
			name:     "detects !terraform.state function",
			input:    "!terraform.state vpc.arn",
			expected: true,
		},
		{
			name:     "detects !store.get function",
			input:    "!store.get secret.key",
			expected: true,
		},
		{
			name:     "detects !store function",
			input:    "!store secret.key",
			expected: true,
		},
		{
			name:     "detects !exec function",
			input:    "!exec echo hello",
			expected: true,
		},
		{
			name:     "detects !env function",
			input:    "!env AWS_REGION",
			expected: true,
		},
		{
			name:     "detects !labels function",
			input:    "!labels",
			expected: true,
		},
		{
			name:     "detects !labels.keys function",
			input:    "!labels.keys",
			expected: true,
		},
		{
			name:     "detects !labels.values function",
			input:    "!labels.values",
			expected: true,
		},
		{
			name:     "detects !tags function",
			input:    "!tags",
			expected: true,
		},
		{
			name:     "returns false for regular string",
			input:    "regular string",
			expected: false,
		},
		{
			name:     "returns false for empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "returns false for !include (pre-merge function)",
			input:    "!include catalog/base",
			expected: false,
		},
		{
			name:     "returns false for partial match",
			input:    "template without tag",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAtmosYAMLFunction(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWalkAndDeferYAMLFunctions(t *testing.T) {
	t.Run("defers YAML function strings", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"config": "!template '{{ .settings.base }}'",
			"region": "us-east-1",
		}

		result := WalkAndDeferYAMLFunctions(dctx, input, []string{"vars"})

		// YAML function should be replaced with nil.
		assert.Nil(t, result["config"])
		// Regular values should be preserved.
		assert.Equal(t, "us-east-1", result["region"])

		// Check deferred context.
		assert.True(t, dctx.HasDeferredValues())
		values := dctx.GetDeferredValues()
		assert.Contains(t, values, "vars.config")
		assert.Equal(t, "!template '{{ .settings.base }}'", values["vars.config"][0].Value)
	})

	t.Run("recursively processes nested maps", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"level1": map[string]interface{}{
				"level2": map[string]interface{}{
					"template": "!template 'value'",
					"regular":  "string",
				},
			},
		}

		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})

		// Navigate to nested value using require guards to prevent panics on type mismatch.
		level1, ok := result["level1"].(map[string]interface{})
		require.True(t, ok, "level1 should be a map")
		level2, ok := level1["level2"].(map[string]interface{})
		require.True(t, ok, "level2 should be a map")

		assert.Nil(t, level2["template"])
		assert.Equal(t, "string", level2["regular"])

		// Check deferred context.
		values := dctx.GetDeferredValues()
		assert.Contains(t, values, "level1.level2.template")
	})

	t.Run("preserves non-YAML-function strings", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"normal":  "just a string",
			"number":  42,
			"boolean": true,
		}

		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})

		assert.Equal(t, "just a string", result["normal"])
		assert.Equal(t, 42, result["number"])
		assert.Equal(t, true, result["boolean"])
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("handles nil input", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		result := WalkAndDeferYAMLFunctions(dctx, nil, []string{})
		assert.Nil(t, result)
	})

	t.Run("handles empty map", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{}
		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})
}

// TestWalkAndDeferYAMLFunctions_NoFunctionsShortCircuit verifies the Phase 3
// optimization: when the input contains no YAML functions anywhere in its
// nested structure, WalkAndDeferYAMLFunctions returns the input map as-is
// without allocating a deep copy. The function-free fast path is critical
// for the merge pipeline in describe affected, where most component
// configurations contain no YAML functions but were previously deep-copied
// on every merge call.
func TestWalkAndDeferYAMLFunctions_NoFunctionsShortCircuit(t *testing.T) {
	t.Run("returns same map reference for function-free flat map", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"region":      "us-east-1",
			"environment": "prod",
			"replicas":    3,
			"enabled":     true,
		}
		// Capture a pointer-identity reference. The fast path must return
		// the same map object, not a copy.
		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})
		require.True(t, sameMapHeader(input, result),
			"function-free input should be returned as-is (zero allocation)")
		assert.False(t, dctx.HasDeferredValues(),
			"no deferrals expected when no YAML functions are present")
	})

	t.Run("returns same map reference for function-free nested map", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"tags": map[string]interface{}{
				"namespace": "acme",
				"stage":     "prod",
				"meta": map[string]interface{}{
					"owner": "platform",
				},
			},
			"settings": map[string]interface{}{
				"spacelift": map[string]interface{}{
					"workspace_enabled": true,
				},
			},
		}
		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})
		require.True(t, sameMapHeader(input, result),
			"function-free nested input should be returned as-is (zero allocation)")
		assert.False(t, dctx.HasDeferredValues())
	})

	t.Run("walks normally when any subtree contains a YAML function", func(t *testing.T) {
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"tags": map[string]interface{}{
				"namespace": "acme", // No function here.
			},
			"vars": map[string]interface{}{
				"region": "!template '{{ .stage }}'", // Function here — must walk.
			},
		}
		result := WalkAndDeferYAMLFunctions(dctx, input, []string{})
		require.False(t, sameMapHeader(input, result),
			"presence of any YAML function must force a deep walk")
		assert.True(t, dctx.HasDeferredValues())

		// The non-function tags subtree should still be reachable in the result.
		tags, ok := result["tags"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "acme", tags["namespace"])

		// The function value should be replaced with nil placeholder.
		vars, ok := result["vars"].(map[string]interface{})
		require.True(t, ok)
		assert.Nil(t, vars["region"])
	})

	t.Run("short-circuit return is safe under read-only access", func(t *testing.T) {
		// The fast path returns the input directly. The caller contract says
		// the result is read-only. This test documents the contract by
		// asserting that subsequent reads yield identical values, and that
		// the input itself was not modified.
		dctx := NewDeferredMergeContext()
		input := map[string]interface{}{
			"key":   "value",
			"count": 42,
		}
		original := map[string]interface{}{
			"key":   "value",
			"count": 42,
		}

		_ = WalkAndDeferYAMLFunctions(dctx, input, []string{})

		// Input must be unchanged.
		assert.Equal(t, original, input,
			"WalkAndDeferYAMLFunctions must not mutate function-free inputs")
	})
}

// sameMapHeader returns true if a and b reference the same underlying
// runtime map. The reflect.Value.Pointer documentation says the returned
// value is the underlying pointer for Map/Chan/Func/Pointer/Slice/etc.,
// so reflect.ValueOf(m).Pointer() is the canonical way to obtain the map
// pointer for identity comparison. Previously this used fmt.Sprintf("%p",
// ...); that works in practice but %p formatting isn't a formal documented
// mechanism for map identity.
func sameMapHeader(a, b map[string]interface{}) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}
