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

// makeVariablesMap converts a map of values to a terraform variables map format.
func makeVariablesMap(vars map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range vars {
		result[k] = map[string]interface{}{
			"value": v,
		}
	}
	return result
}

func TestProcessRootModuleResources_DataResourcesSkipped(t *testing.T) {
	tests := []struct {
		name           string
		resources      []interface{}
		expectedResult map[string]interface{}
		description    string
	}{
		{
			name: "only data resources - all skipped",
			resources: []interface{}{
				map[string]interface{}{
					"address": "data.aws_ami.example",
					"mode":    "data",
					"values": map[string]interface{}{
						"id": "ami-12345",
					},
				},
				map[string]interface{}{
					"address": "data.aws_vpc.main",
					"mode":    "data",
					"values": map[string]interface{}{
						"id": "vpc-12345",
					},
				},
			},
			expectedResult: map[string]interface{}{},
			description:    "Data resources should be completely skipped",
		},
		{
			name: "mixed managed and data resources - only managed included",
			resources: []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"values": map[string]interface{}{
						"instance_type": "t2.micro",
					},
				},
				map[string]interface{}{
					"address": "data.aws_ami.example",
					"mode":    "data",
					"values": map[string]interface{}{
						"id": "ami-12345",
					},
				},
				map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"values": map[string]interface{}{
						"bucket": "my-logs",
					},
				},
			},
			expectedResult: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"values": map[string]interface{}{
						"instance_type": "t2.micro",
					},
				},
				"aws_s3_bucket.logs": map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"values": map[string]interface{}{
						"bucket": "my-logs",
					},
				},
			},
			description: "Only managed resources should be included, data resources should be skipped",
		},
		{
			name: "only managed resources - all included",
			resources: []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"values": map[string]interface{}{
						"instance_type": "t2.micro",
					},
				},
				map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"values": map[string]interface{}{
						"bucket": "my-logs",
					},
				},
			},
			expectedResult: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"values": map[string]interface{}{
						"instance_type": "t2.micro",
					},
				},
				"aws_s3_bucket.logs": map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"values": map[string]interface{}{
						"bucket": "my-logs",
					},
				},
			},
			description: "All managed resources should be included",
		},
		{
			name: "resources without mode field - skipped",
			resources: []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"values": map[string]interface{}{
						"instance_type": "t2.micro",
					},
				},
			},
			expectedResult: map[string]interface{}{},
			description:    "Resources without mode field should be skipped (current implementation behavior)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootModule := map[string]interface{}{
				"resources": tc.resources,
			}
			result := make(map[string]interface{})

			processRootModuleResources(rootModule, result)

			assert.Equal(t, tc.expectedResult, result, tc.description)
		})
	}
}

func TestProcessResourceChanges_DataResourcesSkipped(t *testing.T) {
	tests := []struct {
		name            string
		resourceChanges []interface{}
		expectedResult  map[string]interface{}
		description     string
	}{
		{
			name: "only data resource changes - all skipped",
			resourceChanges: []interface{}{
				map[string]interface{}{
					"address": "data.aws_ami.example",
					"mode":    "data",
					"change": map[string]interface{}{
						"actions": []string{"read"},
					},
				},
				map[string]interface{}{
					"address": "data.aws_vpc.main",
					"mode":    "data",
					"change": map[string]interface{}{
						"actions": []string{"read"},
					},
				},
			},
			expectedResult: map[string]interface{}{},
			description:    "Data resource changes should be completely skipped",
		},
		{
			name: "mixed managed and data resource changes - only managed included",
			resourceChanges: []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"update"},
					},
				},
				map[string]interface{}{
					"address": "data.aws_ami.example",
					"mode":    "data",
					"change": map[string]interface{}{
						"actions": []string{"read"},
					},
				},
				map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"create"},
					},
				},
			},
			expectedResult: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"update"},
					},
				},
				"aws_s3_bucket.logs": map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"create"},
					},
				},
			},
			description: "Only managed resource changes should be included, data resource changes should be skipped",
		},
		{
			name: "only managed resource changes - all included",
			resourceChanges: []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"update"},
					},
				},
				map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"create"},
					},
				},
			},
			expectedResult: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"update"},
					},
				},
				"aws_s3_bucket.logs": map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"create"},
					},
				},
			},
			description: "All managed resource changes should be included",
		},
		{
			name: "resource changes without mode field - skipped",
			resourceChanges: []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"change": map[string]interface{}{
						"actions": []string{"update"},
					},
				},
			},
			expectedResult: map[string]interface{}{},
			description:    "Resource changes without mode field should be skipped (current implementation behavior)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := map[string]interface{}{
				"resource_changes": tc.resourceChanges,
			}
			result := make(map[string]interface{})

			processResourceChanges(plan, result)

			assert.Equal(t, tc.expectedResult, result, tc.description)
		})
	}
}

func TestGetResources_DataResourcesSkipped(t *testing.T) {
	tests := []struct {
		name           string
		plan           map[string]interface{}
		expectedResult map[string]interface{}
		description    string
	}{
		{
			name: "data resources in prior_state - skipped",
			plan: map[string]interface{}{
				"prior_state": map[string]interface{}{
					"values": map[string]interface{}{
						"root_module": map[string]interface{}{
							"resources": []interface{}{
								map[string]interface{}{
									"address": "data.aws_ami.example",
									"mode":    "data",
									"values": map[string]interface{}{
										"id": "ami-12345",
									},
								},
								map[string]interface{}{
									"address": "aws_instance.example",
									"mode":    "managed",
									"values": map[string]interface{}{
										"instance_type": "t2.micro",
									},
								},
							},
						},
					},
				},
			},
			expectedResult: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"values": map[string]interface{}{
						"instance_type": "t2.micro",
					},
				},
			},
			description: "Data resources in prior_state should be skipped, only managed resources included",
		},
		{
			name: "data resources in planned_values - skipped",
			plan: map[string]interface{}{
				"planned_values": map[string]interface{}{
					"root_module": map[string]interface{}{
						"resources": []interface{}{
							map[string]interface{}{
								"address": "data.aws_vpc.main",
								"mode":    "data",
								"values": map[string]interface{}{
									"id": "vpc-12345",
								},
							},
							map[string]interface{}{
								"address": "aws_s3_bucket.logs",
								"mode":    "managed",
								"values": map[string]interface{}{
									"bucket": "my-logs",
								},
							},
						},
					},
				},
			},
			expectedResult: map[string]interface{}{
				"aws_s3_bucket.logs": map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"values": map[string]interface{}{
						"bucket": "my-logs",
					},
				},
			},
			description: "Data resources in planned_values should be skipped, only managed resources included",
		},
		{
			name: "data resources in resource_changes - skipped",
			plan: map[string]interface{}{
				"resource_changes": []interface{}{
					map[string]interface{}{
						"address": "data.aws_ami.example",
						"mode":    "data",
						"change": map[string]interface{}{
							"actions": []string{"read"},
						},
					},
					map[string]interface{}{
						"address": "aws_instance.example",
						"mode":    "managed",
						"change": map[string]interface{}{
							"actions": []string{"update"},
						},
					},
				},
			},
			expectedResult: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []string{"update"},
					},
				},
			},
			description: "Data resources in resource_changes should be skipped, only managed resources included",
		},
		{
			name: "data resources in all sections - all skipped",
			plan: map[string]interface{}{
				"prior_state": map[string]interface{}{
					"values": map[string]interface{}{
						"root_module": map[string]interface{}{
							"resources": []interface{}{
								map[string]interface{}{
									"address": "data.aws_ami.example",
									"mode":    "data",
									"values": map[string]interface{}{
										"id": "ami-12345",
									},
								},
								map[string]interface{}{
									"address": "aws_instance.example",
									"mode":    "managed",
									"values": map[string]interface{}{
										"instance_type": "t2.micro",
									},
								},
							},
						},
					},
				},
				"planned_values": map[string]interface{}{
					"root_module": map[string]interface{}{
						"resources": []interface{}{
							map[string]interface{}{
								"address": "data.aws_vpc.main",
								"mode":    "data",
								"values": map[string]interface{}{
									"id": "vpc-12345",
								},
							},
							map[string]interface{}{
								"address": "aws_s3_bucket.logs",
								"mode":    "managed",
								"values": map[string]interface{}{
									"bucket": "my-logs",
								},
							},
						},
					},
				},
				"resource_changes": []interface{}{
					map[string]interface{}{
						"address": "data.aws_subnet.main",
						"mode":    "data",
						"change": map[string]interface{}{
							"actions": []string{"read"},
						},
					},
					map[string]interface{}{
						"address": "aws_instance.example",
						"mode":    "managed",
						"change": map[string]interface{}{
							"actions": []string{"update"},
						},
					},
				},
			},
			expectedResult: map[string]interface{}{
				"aws_instance.example": map[string]interface{}{
					"address": "aws_instance.example",
					"change": map[string]interface{}{
						"actions": []string{"update"},
					},
					"mode": "managed",
				},
				"aws_s3_bucket.logs": map[string]interface{}{
					"address": "aws_s3_bucket.logs",
					"mode":    "managed",
					"values": map[string]interface{}{
						"bucket": "my-logs",
					},
				},
			},
			description: "Data resources in all sections should be skipped, only managed resources included",
		},
		{
			name: "only data resources in all sections - empty result",
			plan: map[string]interface{}{
				"prior_state": map[string]interface{}{
					"values": map[string]interface{}{
						"root_module": map[string]interface{}{
							"resources": []interface{}{
								map[string]interface{}{
									"address": "data.aws_ami.example",
									"mode":    "data",
									"values": map[string]interface{}{
										"id": "ami-12345",
									},
								},
							},
						},
					},
				},
				"planned_values": map[string]interface{}{
					"root_module": map[string]interface{}{
						"resources": []interface{}{
							map[string]interface{}{
								"address": "data.aws_vpc.main",
								"mode":    "data",
								"values": map[string]interface{}{
									"id": "vpc-12345",
								},
							},
						},
					},
				},
				"resource_changes": []interface{}{
					map[string]interface{}{
						"address": "data.aws_subnet.main",
						"mode":    "data",
						"change": map[string]interface{}{
							"actions": []string{"read"},
						},
					},
				},
			},
			expectedResult: map[string]interface{}{},
			description:    "When only data resources exist, result should be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getResources(tc.plan)

			assert.Equal(t, tc.expectedResult, result, tc.description)
		})
	}
}
