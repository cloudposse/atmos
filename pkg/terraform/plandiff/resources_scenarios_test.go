package plandiff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessRootModuleResources_DataResourcesSkipped(t *testing.T) {
	t.Parallel()

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
			t.Parallel()
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
	t.Parallel()

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
			t.Parallel()
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
	t.Parallel()

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
			t.Parallel()
			result := getResources(tc.plan)

			assert.Equal(t, tc.expectedResult, result, tc.description)
		})
	}
}
