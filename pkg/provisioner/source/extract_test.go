package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractSource(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expectError     error
		expectNil       bool // If true, result should be nil (no source configured).
		expectedURI     string
		expectedVersion string
	}{
		{
			name: "top-level string source",
			componentConfig: map[string]any{
				"source": "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
			expectedVersion: "",
		},
		{
			name: "top-level map source",
			componentConfig: map[string]any{
				"source": map[string]any{
					"uri":     "github.com/cloudposse/terraform-aws-components//modules/vpc",
					"version": "v1.0.0",
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
			expectedVersion: "v1.0.0",
		},
		{
			name: "map with included_paths and excluded_paths",
			componentConfig: map[string]any{
				"source": map[string]any{
					"uri":            "github.com/cloudposse/terraform-aws-components//modules/vpc",
					"version":        "v2.0.0",
					"included_paths": []any{"*.tf", "*.tfvars"},
					"excluded_paths": []any{"*.md", "tests/*"},
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
			expectedVersion: "v2.0.0",
		},
		{
			name: "no source returns nil (not an error)",
			componentConfig: map[string]any{
				"vars": map[string]any{
					"foo": "bar",
				},
			},
			expectError: nil,
			expectNil:   true,
		},
		{
			name: "metadata but no source field returns nil",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expectError: nil,
			expectNil:   true,
		},
		{
			name:            "empty component config returns nil",
			componentConfig: map[string]any{},
			expectError:     nil,
			expectNil:       true,
		},
		{
			name:            "nil component config returns nil",
			componentConfig: nil,
			expectError:     nil,
			expectNil:       true,
		},
		{
			name: "map without uri field returns nil (no source configured)",
			componentConfig: map[string]any{
				"source": map[string]any{
					"version": "v1.0.0",
				},
			},
			expectError: nil,
			expectNil:   true,
		},
		{
			name: "empty string URI returns nil (no source configured)",
			componentConfig: map[string]any{
				"source": "",
			},
			expectError: nil,
			expectNil:   true,
		},
		{
			name: "invalid type returns error",
			componentConfig: map[string]any{
				"source": 12345,
			},
			expectError: errUtils.ErrSourceInvalidSpec,
			expectNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractSource(tt.componentConfig)

			switch {
			case tt.expectError != nil:
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectError)
				assert.Nil(t, result)
			case tt.expectNil:
				require.NoError(t, err)
				assert.Nil(t, result)
			default:
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedURI, result.Uri)
				assert.Equal(t, tt.expectedVersion, result.Version)
			}
		})
	}
}

func TestExtractSource_IncludedExcludedPaths(t *testing.T) {
	componentConfig := map[string]any{
		"source": map[string]any{
			"uri":            "github.com/cloudposse/terraform-aws-components//modules/vpc",
			"included_paths": []any{"*.tf", "*.tfvars"},
			"excluded_paths": []any{"*.md", "tests/*"},
		},
	}

	result, err := ExtractSource(componentConfig)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, []string{"*.tf", "*.tfvars"}, result.IncludedPaths)
	assert.Equal(t, []string{"*.md", "tests/*"}, result.ExcludedPaths)
}

func TestHasSource(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        bool
	}{
		{
			name: "has top-level string source",
			componentConfig: map[string]any{
				"source": "github.com/example/repo//module",
			},
			expected: true,
		},
		{
			name: "has top-level map source",
			componentConfig: map[string]any{
				"source": map[string]any{
					"uri": "github.com/example/repo//module",
				},
			},
			expected: true,
		},
		{
			name: "no source",
			componentConfig: map[string]any{
				"vars": map[string]any{},
			},
			expected: false,
		},
		{
			name: "metadata without source is not detected",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expected: false,
		},
		{
			name: "empty source string is not considered valid source",
			componentConfig: map[string]any{
				"source": "",
			},
			expected: false,
		},
		{
			name:            "nil config",
			componentConfig: nil,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasSource(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseDurationPtr(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		key      string
		expected string // Expected duration as string, or "" for nil.
		isNil    bool   // Whether nil is expected.
	}{
		{
			name:     "valid duration string",
			input:    map[string]any{"delay": "5s"},
			key:      "delay",
			expected: "5s",
			isNil:    false,
		},
		{
			name:     "valid duration with minutes",
			input:    map[string]any{"timeout": "2m30s"},
			key:      "timeout",
			expected: "2m30s",
			isNil:    false,
		},
		{
			name:  "key not found returns nil",
			input: map[string]any{"other": "5s"},
			key:   "delay",
			isNil: true,
		},
		{
			name:  "invalid duration string returns nil",
			input: map[string]any{"delay": "invalid"},
			key:   "delay",
			isNil: true,
		},
		{
			name:  "non-string value returns nil",
			input: map[string]any{"delay": 123},
			key:   "delay",
			isNil: true,
		},
		{
			name:  "empty string returns nil",
			input: map[string]any{"delay": ""},
			key:   "delay",
			isNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDurationPtr(tt.input, tt.key)
			if tt.isNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected, result.String())
			}
		})
	}
}

func TestParseRetryConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		validate func(t *testing.T, cfg *schema.RetryConfig)
	}{
		{
			name: "full config",
			input: map[string]any{
				"max_attempts":     5,
				"initial_delay":    "2s",
				"max_delay":        "30s",
				"max_elapsed_time": "5m",
				"backoff_strategy": "exponential",
				"random_jitter":    0.1,
				"multiplier":       2.0,
			},
			validate: func(t *testing.T, cfg *schema.RetryConfig) {
				require.NotNil(t, cfg.MaxAttempts)
				assert.Equal(t, 5, *cfg.MaxAttempts)
				require.NotNil(t, cfg.InitialDelay)
				assert.Equal(t, "2s", cfg.InitialDelay.String())
				require.NotNil(t, cfg.MaxDelay)
				assert.Equal(t, "30s", cfg.MaxDelay.String())
				require.NotNil(t, cfg.MaxElapsedTime)
				assert.Equal(t, "5m0s", cfg.MaxElapsedTime.String())
				assert.Equal(t, schema.BackoffStrategy("exponential"), cfg.BackoffStrategy)
				require.NotNil(t, cfg.RandomJitter)
				assert.Equal(t, 0.1, *cfg.RandomJitter)
				require.NotNil(t, cfg.Multiplier)
				assert.Equal(t, 2.0, *cfg.Multiplier)
			},
		},
		{
			name: "partial config",
			input: map[string]any{
				"max_attempts":  3,
				"initial_delay": "1s",
			},
			validate: func(t *testing.T, cfg *schema.RetryConfig) {
				require.NotNil(t, cfg.MaxAttempts)
				assert.Equal(t, 3, *cfg.MaxAttempts)
				require.NotNil(t, cfg.InitialDelay)
				assert.Equal(t, "1s", cfg.InitialDelay.String())
				assert.Nil(t, cfg.MaxDelay, "MaxDelay should be nil when not specified")
				assert.Equal(t, schema.BackoffStrategy(""), cfg.BackoffStrategy)
			},
		},
		{
			name:  "empty config",
			input: map[string]any{},
			validate: func(t *testing.T, cfg *schema.RetryConfig) {
				assert.Nil(t, cfg.MaxAttempts, "MaxAttempts should be nil when not specified")
				assert.Nil(t, cfg.InitialDelay, "InitialDelay should be nil when not specified")
			},
		},
		{
			name: "integer values parsed as float64",
			input: map[string]any{
				"random_jitter": int(1),
				"multiplier":    int(2),
			},
			validate: func(t *testing.T, cfg *schema.RetryConfig) {
				require.NotNil(t, cfg.RandomJitter, "RandomJitter should parse int to float64")
				assert.Equal(t, 1.0, *cfg.RandomJitter)
				require.NotNil(t, cfg.Multiplier, "Multiplier should parse int to float64")
				assert.Equal(t, 2.0, *cfg.Multiplier)
			},
		},
		{
			name: "invalid types are ignored",
			input: map[string]any{
				"max_attempts":     "not-an-int",
				"backoff_strategy": 123,
				"random_jitter":    "not-a-float",
			},
			validate: func(t *testing.T, cfg *schema.RetryConfig) {
				assert.Nil(t, cfg.MaxAttempts, "MaxAttempts should be nil when invalid type")
				assert.Equal(t, schema.BackoffStrategy(""), cfg.BackoffStrategy)
				assert.Nil(t, cfg.RandomJitter, "RandomJitter should be nil when invalid type")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRetryConfig(tt.input)
			require.NotNil(t, result)
			tt.validate(t, result)
		})
	}
}

func TestExtractSource_WithRetryConfig(t *testing.T) {
	componentConfig := map[string]any{
		"source": map[string]any{
			"uri":     "github.com/example/repo//module",
			"version": "v1.0.0",
			"retry": map[string]any{
				"max_attempts":     5,
				"initial_delay":    "2s",
				"max_delay":        "60s",
				"backoff_strategy": "exponential",
			},
		},
	}

	result, err := ExtractSource(componentConfig)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Retry)

	require.NotNil(t, result.Retry.MaxAttempts)
	assert.Equal(t, 5, *result.Retry.MaxAttempts)
	require.NotNil(t, result.Retry.InitialDelay)
	assert.Equal(t, "2s", result.Retry.InitialDelay.String())
	require.NotNil(t, result.Retry.MaxDelay)
	assert.Equal(t, "1m0s", result.Retry.MaxDelay.String())
	assert.Equal(t, schema.BackoffStrategy("exponential"), result.Retry.BackoffStrategy)
}

func TestExtractSource_WithTypeField(t *testing.T) {
	componentConfig := map[string]any{
		"source": map[string]any{
			"type":    "git",
			"uri":     "github.com/example/repo//module",
			"version": "v1.0.0",
		},
	}

	result, err := ExtractSource(componentConfig)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "git", result.Type)
	assert.Equal(t, "github.com/example/repo//module", result.Uri)
	assert.Equal(t, "v1.0.0", result.Version)
}

func TestParseIntPtr(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		key      string
		expected *int
	}{
		{
			name:     "Go int value",
			input:    map[string]any{"count": int(7)},
			key:      "count",
			expected: intPtr(7),
		},
		{
			name:     "float64 value",
			input:    map[string]any{"count": float64(5)},
			key:      "count",
			expected: intPtr(5),
		},
		{
			name:     "string value returns nil",
			input:    map[string]any{"count": "not-a-number"},
			key:      "count",
			expected: nil,
		},
		{
			name:     "key not found returns nil",
			input:    map[string]any{"other": 5},
			key:      "count",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIntPtr(tt.input, tt.key)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}

func TestParseFloat64Ptr(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		key      string
		expected *float64
	}{
		{
			name:     "float64 value",
			input:    map[string]any{"val": float64(3.14)},
			key:      "val",
			expected: float64Ptr(3.14),
		},
		{
			name:     "float32 value",
			input:    map[string]any{"val": float32(2.5)},
			key:      "val",
			expected: float64Ptr(2.5),
		},
		{
			name:     "int value",
			input:    map[string]any{"val": int(7)},
			key:      "val",
			expected: float64Ptr(7.0),
		},
		{
			name:     "int64 value",
			input:    map[string]any{"val": int64(42)},
			key:      "val",
			expected: float64Ptr(42.0),
		},
		{
			name:     "uint value",
			input:    map[string]any{"val": uint(10)},
			key:      "val",
			expected: float64Ptr(10.0),
		},
		{
			name:     "uint64 value",
			input:    map[string]any{"val": uint64(99)},
			key:      "val",
			expected: float64Ptr(99.0),
		},
		{
			name:     "string value returns nil",
			input:    map[string]any{"val": "not-a-number"},
			key:      "val",
			expected: nil,
		},
		{
			name:     "key not found returns nil",
			input:    map[string]any{"other": 5.0},
			key:      "val",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFloat64Ptr(tt.input, tt.key)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.InDelta(t, *tt.expected, *result, 0.001)
			}
		})
	}
}

func TestHasSource_NonStringNonMapType(t *testing.T) {
	// A source that is neither string nor map should return false.
	componentConfig := map[string]any{
		"source": 12345,
	}
	result := HasSource(componentConfig)
	assert.False(t, result)
}

func TestHasSource_NilSource(t *testing.T) {
	// Explicitly nil source value.
	componentConfig := map[string]any{
		"source": nil,
	}
	result := HasSource(componentConfig)
	assert.False(t, result)
}

func TestHasSource_MapWithEmptyURI(t *testing.T) {
	// Map source with empty uri should return false.
	componentConfig := map[string]any{
		"source": map[string]any{
			"uri": "",
		},
	}
	result := HasSource(componentConfig)
	assert.False(t, result)
}

func TestHasSource_MapWithoutURI(t *testing.T) {
	// Map source without uri field should return false.
	componentConfig := map[string]any{
		"source": map[string]any{
			"version": "v1.0.0",
		},
	}
	result := HasSource(componentConfig)
	assert.False(t, result)
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []any
		expected []string
	}{
		{
			name:     "all strings",
			input:    []any{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "mixed types skips non-strings",
			input:    []any{"a", 123, "b", true, "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "empty slice",
			input:    []any{},
			expected: []string{},
		},
		{
			name:     "no strings",
			input:    []any{1, 2, 3},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toStringSlice(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRetryConfig_FloatTypes(t *testing.T) {
	// Test parseFloat64Ptr with various numeric types via parseRetryConfig.
	tests := []struct {
		name             string
		input            map[string]any
		expectedJitter   *float64
		expectedMultiply *float64
	}{
		{
			name:             "float32 values",
			input:            map[string]any{"random_jitter": float32(0.5), "multiplier": float32(3.0)},
			expectedJitter:   float64Ptr(0.5),
			expectedMultiply: float64Ptr(3.0),
		},
		{
			name:             "int64 values",
			input:            map[string]any{"random_jitter": int64(1), "multiplier": int64(4)},
			expectedJitter:   float64Ptr(1.0),
			expectedMultiply: float64Ptr(4.0),
		},
		{
			name:             "uint values",
			input:            map[string]any{"random_jitter": uint(1), "multiplier": uint(2)},
			expectedJitter:   float64Ptr(1.0),
			expectedMultiply: float64Ptr(2.0),
		},
		{
			name:             "uint64 values",
			input:            map[string]any{"random_jitter": uint64(1), "multiplier": uint64(5)},
			expectedJitter:   float64Ptr(1.0),
			expectedMultiply: float64Ptr(5.0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseRetryConfig(tt.input)
			require.NotNil(t, cfg)
			if tt.expectedJitter != nil {
				require.NotNil(t, cfg.RandomJitter)
				assert.InDelta(t, *tt.expectedJitter, *cfg.RandomJitter, 0.01)
			}
			if tt.expectedMultiply != nil {
				require.NotNil(t, cfg.Multiplier)
				assert.InDelta(t, *tt.expectedMultiply, *cfg.Multiplier, 0.01)
			}
		})
	}
}

func TestExtractSource_MapWithEmptyURI(t *testing.T) {
	// Map with empty uri should return nil (no source configured).
	componentConfig := map[string]any{
		"source": map[string]any{
			"uri": "",
		},
	}
	result, err := ExtractSource(componentConfig)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func intPtr(i int) *int             { return &i }
func float64Ptr(f float64) *float64 { return &f }
