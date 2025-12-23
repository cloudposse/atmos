package list

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestNormalizeComponentQuery tests query normalization logic.
func TestNormalizeComponentQuery(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
		errorMsg    string
	}{
		// === Empty and identity queries ===
		{
			name:     "empty query",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "identity query",
			input:    ".",
			expected: ".",
		},

		// === Already wrapped queries ===
		{
			name:     "already wrapped simple",
			input:    "[.[] | select(.x)]",
			expected: "[.[] | select(.x)]",
		},
		{
			name:     "already wrapped complex",
			input:    "[.[] | select(.locked == true) | select(.stack | test(\"prod\"))]",
			expected: "[.[] | select(.locked == true) | select(.stack | test(\"prod\"))]",
		},

		// === Simplified select() syntax ===
		{
			name:     "bare select - should add .[] prefix and wrap",
			input:    "select(.locked == true)",
			expected: "[.[] | select(.locked == true)]",
		},
		{
			name:     "select with test function",
			input:    "select(.stack | test(\"prod\"))",
			expected: "[.[] | select(.stack | test(\"prod\"))]",
		},
		{
			name:     "select with complex condition",
			input:    "select(.enabled == true and .locked == false)",
			expected: "[.[] | select(.enabled == true and .locked == false)]",
		},

		// === .[] prefix queries - should wrap in array ===
		{
			name:     ".[] | select - should wrap",
			input:    ".[] | select(.locked)",
			expected: "[.[] | select(.locked)]",
		},
		{
			name:     ".[] | select with equality",
			input:    ".[] | select(.type == \"terraform\")",
			expected: "[.[] | select(.type == \"terraform\")]",
		},
		{
			name:     "chained selects",
			input:    ".[] | select(.a) | select(.b)",
			expected: "[.[] | select(.a) | select(.b)]",
		},

		// === Scalar extraction queries - should ERROR ===
		{
			name:        "scalar .[].field",
			input:       ".[].component",
			expectError: true,
			errorMsg:    "scalar extraction queries",
		},
		{
			name:        "scalar .[] | .field",
			input:       ".[] | .component",
			expectError: true,
			errorMsg:    "scalar extraction queries",
		},
		{
			name:        "scalar .[0].field",
			input:       ".[0].component",
			expectError: true,
			errorMsg:    "scalar extraction queries",
		},
		{
			name:        "scalar .[].field with sort",
			input:       ".[].stack",
			expectError: true,
			errorMsg:    "scalar extraction queries",
		},

		// === Edge cases with select after extraction - OK because filter then extract returns maps ===
		{
			name:     "index access without field extraction",
			input:    ".[0]",
			expected: "[.[0]]",
		},

		// === Default wrapping for other expressions ===
		{
			name:     "map function",
			input:    "map(select(.locked))",
			expected: "[map(select(.locked))]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeComponentQuery(tt.input)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestIsScalarExtractionQuery tests detection of scalar extraction patterns.
func TestIsScalarExtractionQuery(t *testing.T) {
	scalarQueries := []string{
		".[].component",
		".[].stack",
		".[] | .component",
		".[] | .stack",
		".[0].component",
		".[1].stack",
		".[10].type",
	}

	nonScalarQueries := []string{
		".[] | select(.x)",
		"select(.x)",
		"[.[] | select(.x)]",
		".",
		".[0]",
		".[1]",
		".[] | select(.locked) | .component", // select is present, so allowed
		".[].component | select(. != null)",  // has select, so allowed
	}

	for _, query := range scalarQueries {
		t.Run("scalar: "+query, func(t *testing.T) {
			assert.True(t, isScalarExtractionQuery(query), "expected %q to be detected as scalar extraction", query)
		})
	}

	for _, query := range nonScalarQueries {
		t.Run("non-scalar: "+query, func(t *testing.T) {
			assert.False(t, isScalarExtractionQuery(query), "expected %q to NOT be detected as scalar extraction", query)
		})
	}
}

// TestFilterComponentsWithQuery tests the actual filtering logic with YQ expressions.
func TestFilterComponentsWithQuery(t *testing.T) {
	// Create test components.
	components := []map[string]any{
		{"component": "vpc", "stack": "prod-us-east-1", "locked": true, "kind": "terraform", "type": "real"},
		{"component": "eks", "stack": "prod-us-east-1", "locked": false, "kind": "terraform", "type": "real"},
		{"component": "vpc", "stack": "dev-us-west-2", "locked": false, "kind": "terraform", "type": "real"},
		{"component": "redis", "stack": "prod-us-east-1", "locked": false, "kind": "helmfile", "type": "real"},
		{"component": "base", "stack": "staging-us-east-1", "locked": false, "kind": "terraform", "type": "abstract"},
	}

	// Create minimal atmos config for YQ evaluation.
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name          string
		query         string
		expectedCount int
		expectedFirst string // component name of first result (for ordering verification)
	}{
		{
			name:          "filter locked components",
			query:         "[.[] | select(.locked == true)]",
			expectedCount: 1,
			expectedFirst: "vpc",
		},
		{
			name:          "filter by stack pattern",
			query:         "[.[] | select(.stack | test(\"prod\"))]",
			expectedCount: 3, // vpc, eks, redis (all in prod)
		},
		{
			name:          "filter by kind helmfile",
			query:         "[.[] | select(.kind == \"helmfile\")]",
			expectedCount: 1,
			expectedFirst: "redis",
		},
		{
			name:          "filter abstract components",
			query:         "[.[] | select(.type == \"abstract\")]",
			expectedCount: 1,
			expectedFirst: "base",
		},
		{
			name:          "no matches",
			query:         "[.[] | select(.stack == \"staging\")]",
			expectedCount: 0,
		},
		{
			name:          "combined filter - prod and unlocked",
			query:         "[.[] | select(.stack | test(\"prod\")) | select(.locked == false)]",
			expectedCount: 2, // eks, redis
		},
		{
			name:          "identity query returns all",
			query:         ".",
			expectedCount: 5,
		},
		{
			name:          "filter by component name",
			query:         "[.[] | select(.component == \"vpc\")]",
			expectedCount: 2, // vpc appears in both prod and dev
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filterComponentsWithQuery(atmosConfig, components, tt.query)
			require.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)

			if tt.expectedFirst != "" && len(result) > 0 {
				assert.Equal(t, tt.expectedFirst, result[0]["component"])
			}
		})
	}
}

// TestFilterComponentsWithQueryErrors tests error handling.
func TestFilterComponentsWithQueryErrors(t *testing.T) {
	components := []map[string]any{
		{"component": "vpc", "stack": "prod"},
	}
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name     string
		query    string
		errorMsg string
	}{
		{
			name:     "invalid YQ syntax",
			query:    "[.[] | invalid syntax here",
			errorMsg: "failed to evaluate query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := filterComponentsWithQuery(atmosConfig, components, tt.query)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// TestNormalizeAndFilterIntegration tests the full flow from user query to filtering.
func TestNormalizeAndFilterIntegration(t *testing.T) {
	components := []map[string]any{
		{"component": "vpc", "stack": "prod", "locked": true},
		{"component": "eks", "stack": "prod", "locked": false},
		{"component": "vpc", "stack": "dev", "locked": false},
	}
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name          string
		userQuery     string // What user types
		expectedCount int
	}{
		{
			name:          "simplified select syntax",
			userQuery:     "select(.locked == true)",
			expectedCount: 1,
		},
		{
			name:          "verbose wrapped syntax",
			userQuery:     "[.[] | select(.locked == true)]",
			expectedCount: 1,
		},
		{
			name:          ".[] with select",
			userQuery:     ".[] | select(.stack == \"prod\")",
			expectedCount: 2,
		},
		{
			name:          "identity shows all",
			userQuery:     ".",
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: Normalize the query.
			normalizedQuery, err := normalizeComponentQuery(tt.userQuery)
			require.NoError(t, err)

			// Step 2: Filter with normalized query.
			result, err := filterComponentsWithQuery(atmosConfig, components, normalizedQuery)
			require.NoError(t, err)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

// TestScalarExtractionErrorMessage tests that scalar extraction gives helpful error.
func TestScalarExtractionErrorMessage(t *testing.T) {
	_, err := normalizeComponentQuery(".[].component")
	require.Error(t, err)

	// Should use the correct sentinel error.
	assert.True(t, errors.Is(err, errUtils.ErrScalarExtractionNotSupported))

	// Should mention alternatives.
	assert.Contains(t, err.Error(), "describe component --query")
	assert.Contains(t, err.Error(), "select(...)")
}
