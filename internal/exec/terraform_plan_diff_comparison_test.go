package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareOutputs_AllScenarios(t *testing.T) {
	tests := []struct {
		name        string
		origOutput  map[string]interface{}
		newOutput   map[string]interface{}
		expectDiff  bool
		contains    []string
		notContains []string
	}{
		{
			name: "identical outputs",
			origOutput: map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com",
				},
			},
			newOutput: map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com",
				},
			},
			expectDiff: false,
		},
		{
			name: "different output values",
			origOutput: map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/old",
				},
			},
			newOutput: map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/new",
				},
			},
			expectDiff: true,
			contains:   []string{"~ url:"},
		},
		{
			name: "added output",
			origOutput: map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com",
				},
			},
			newOutput: map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com",
				},
				"api_url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://api.example.com",
				},
			},
			expectDiff: true,
			contains:   []string{"+ api_url:"},
		},
		{
			name: "removed output",
			origOutput: map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com",
				},
				"api_url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://api.example.com",
				},
			},
			newOutput: map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com",
				},
			},
			expectDiff: true,
			contains:   []string{"- api_url:"},
		},
		{
			name: "sensitive value change",
			origOutput: map[string]interface{}{
				"secret": map[string]interface{}{
					"sensitive": true,
					"value":     "old-secret",
				},
			},
			newOutput: map[string]interface{}{
				"secret": map[string]interface{}{
					"sensitive": true,
					"value":     "new-secret",
				},
			},
			expectDiff:  true,
			contains:    []string{"~ secret:", "(sensitive value)"},
			notContains: []string{"old-secret", "new-secret"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diff := compareOutputs(tc.origOutput, tc.newOutput)

			if tc.expectDiff {
				assert.NotEmpty(t, diff, "Expected non-empty diff for different outputs")
				for _, expected := range tc.contains {
					assert.Contains(t, diff, expected, "Expected diff to contain %q", expected)
				}

				for _, notExpected := range tc.notContains {
					assert.NotContains(t, diff, notExpected, "Expected diff not to contain %q", notExpected)
				}
			} else {
				assert.Empty(t, diff, "Expected empty diff for identical outputs")
			}
		})
	}
}

func TestCompareVariables_AllScenarios(t *testing.T) {
	tests := []struct {
		name       string
		origVars   map[string]interface{}
		newVars    map[string]interface{}
		expectDiff bool
		contains   []string
	}{
		{
			name: "identical variables",
			origVars: map[string]interface{}{
				"region": "us-west-2",
				"stage":  "dev",
			},
			newVars: map[string]interface{}{
				"region": "us-west-2",
				"stage":  "dev",
			},
			expectDiff: false,
		},
		{
			name: "changed variable value",
			origVars: map[string]interface{}{
				"region": "us-west-2",
				"stage":  "dev",
			},
			newVars: map[string]interface{}{
				"region": "us-west-2",
				"stage":  "prod",
			},
			expectDiff: true,
			contains:   []string{"~ stage:", "dev => prod"},
		},
		{
			name: "added variable",
			origVars: map[string]interface{}{
				"region": "us-west-2",
			},
			newVars: map[string]interface{}{
				"region": "us-west-2",
				"stage":  "dev",
			},
			expectDiff: true,
			contains:   []string{"+ stage:", "dev"},
		},
		{
			name: "removed variable",
			origVars: map[string]interface{}{
				"region": "us-west-2",
				"stage":  "dev",
			},
			newVars: map[string]interface{}{
				"region": "us-west-2",
			},
			expectDiff: true,
			contains:   []string{"- stage:", "dev"},
		},
		{
			name: "multiple variable changes",
			origVars: map[string]interface{}{
				"region": "us-west-2",
				"stage":  "dev",
				"zone":   "a",
			},
			newVars: map[string]interface{}{
				"region":    "us-east-1",
				"stage":     "prod",
				"namespace": "example",
			},
			expectDiff: true,
			contains: []string{
				"~ region:", "us-west-2 => us-east-1",
				"~ stage:", "dev => prod",
				"- zone:", "a",
				"+ namespace:", "example",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create mock plans with variables
			origPlan := map[string]interface{}{
				"variables": makeVariablesMap(tc.origVars),
			}
			newPlan := map[string]interface{}{
				"variables": makeVariablesMap(tc.newVars),
			}

			diff, hasDiff := compareVariables(origPlan, newPlan)

			assert.Equal(t, tc.expectDiff, hasDiff, "Expected hasDiff to be %v", tc.expectDiff)

			if tc.expectDiff {
				assert.NotEmpty(t, diff, "Expected non-empty diff for different variables")
				for _, expected := range tc.contains {
					assert.Contains(t, diff, expected, "Expected diff to contain %q", expected)
				}
			} else {
				assert.Empty(t, diff, "Expected empty diff for identical variables")
			}
		})
	}
}

func TestCompareResources_AllScenarios(t *testing.T) {
	tests := []struct {
		name       string
		origRes    map[string]interface{}
		newRes     map[string]interface{}
		expectDiff bool
		contains   []string
	}{
		{
			name: "identical resources",
			origRes: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"instance_type": "t2.micro",
					"ami":           "ami-12345",
				},
			},
			newRes: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"instance_type": "t2.micro",
					"ami":           "ami-12345",
				},
			},
			expectDiff: false,
		},
		{
			name: "changed resource attribute",
			origRes: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"instance_type": "t2.micro",
					"ami":           "ami-12345",
				},
			},
			newRes: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"instance_type": "t2.small",
					"ami":           "ami-12345",
				},
			},
			expectDiff: true,
			contains: []string{
				"aws_instance.example",
				"instance_type",
				"t2.micro",
				"t2.small",
			},
		},
		{
			name: "added resource",
			origRes: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"instance_type": "t2.micro",
				},
			},
			newRes: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"instance_type": "t2.micro",
				},
				"aws_s3_bucket.logs": map[string]interface{}{
					"bucket": "my-logs",
				},
			},
			expectDiff: true,
			contains: []string{
				"+ aws_s3_bucket.logs",
			},
		},
		{
			name: "removed resource",
			origRes: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"instance_type": "t2.micro",
				},
				"aws_s3_bucket.logs": map[string]interface{}{
					"bucket": "my-logs",
				},
			},
			newRes: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"instance_type": "t2.micro",
				},
			},
			expectDiff: true,
			contains: []string{
				"- aws_s3_bucket.logs",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diff := compareResources(tc.origRes, tc.newRes)

			if tc.expectDiff {
				assert.NotEmpty(t, diff, "Expected non-empty diff for different resources")
				for _, expected := range tc.contains {
					assert.Contains(t, diff, expected, "Expected diff to contain %q", expected)
				}
			} else {
				assert.Empty(t, diff, "Expected empty diff for identical resources")
			}
		})
	}
}

// makeVariablesMap converts a map of values to a terraform variables map format
func makeVariablesMap(vars map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range vars {
		result[k] = map[string]interface{}{
			"value": v,
		}
	}
	return result
}
