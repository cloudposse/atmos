package plandiff

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetVariables(t *testing.T) {
	plan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "Stockholm",
			},
			"stage": map[string]interface{}{
				"value": "dev",
			},
		},
	}

	expected := map[string]interface{}{
		"location": "Stockholm",
		"stage":    "dev",
	}

	result := getVariables(plan)
	assert.Equal(t, expected, result)
}

func TestGetOutputs(t *testing.T) {
	plan := map[string]interface{}{
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com",
				},
			},
		},
		"output_changes": map[string]interface{}{
			"location": map[string]interface{}{
				"actions":          []interface{}{"create"},
				"before":           nil,
				"after":            "Stockholm",
				"after_unknown":    false,
				"before_sensitive": false,
				"after_sensitive":  false,
			},
		},
	}

	expected := map[string]interface{}{
		"url": map[string]interface{}{
			"sensitive": false,
			"value":     "https://example.com",
		},
		"location": map[string]interface{}{
			"actions":          []interface{}{"create"},
			"before":           nil,
			"after":            "Stockholm",
			"after_unknown":    false,
			"before_sensitive": false,
			"after_sensitive":  false,
		},
	}

	result := getOutputs(plan)
	assert.Equal(t, expected, result)
}

// TestGetResourceAttributes tests the getResourceAttributes function.
func TestGetResourceAttributes(t *testing.T) {
	// Test with values field
	t.Run("with_values_field", func(t *testing.T) {
		resource := map[string]interface{}{
			"address": "aws_instance.example",
			"values": map[string]interface{}{
				"id":   "i-12345",
				"name": "example",
			},
		}
		attrs := getResourceAttributes(resource)
		assert.Equal(t, "i-12345", attrs["id"])
		assert.Equal(t, "example", attrs["name"])
	})

	// Test with change.after field
	t.Run("with_change_after_field", func(t *testing.T) {
		resource := map[string]interface{}{
			"address": "aws_instance.example",
			"change": map[string]interface{}{
				"after": map[string]interface{}{
					"id":   "i-67890",
					"name": "changed",
				},
			},
		}
		attrs := getResourceAttributes(resource)
		assert.Equal(t, "i-67890", attrs["id"])
		assert.Equal(t, "changed", attrs["name"])
	})

	// Test with both fields (values should be overwritten by change.after)
	t.Run("with_both_fields", func(t *testing.T) {
		resource := map[string]interface{}{
			"address": "aws_instance.example",
			"values": map[string]interface{}{
				"id":   "i-12345",
				"name": "original",
			},
			"change": map[string]interface{}{
				"after": map[string]interface{}{
					"id":   "i-67890",
					"name": "changed",
				},
			},
		}
		attrs := getResourceAttributes(resource)
		assert.Equal(t, "i-67890", attrs["id"])
		assert.Equal(t, "changed", attrs["name"])
	})

	// Test with non-map resource
	t.Run("non_map_resource", func(t *testing.T) {
		attrs := getResourceAttributes("not a map")
		assert.Empty(t, attrs)
	})

	// Test with empty resource
	t.Run("empty_resource", func(t *testing.T) {
		attrs := getResourceAttributes(map[string]interface{}{})
		assert.Empty(t, attrs)
	})
}

// TestExtractValuesField tests the extractValuesField function.
func TestExtractValuesField(t *testing.T) {
	t.Run("with_values", func(t *testing.T) {
		resMap := map[string]interface{}{
			"values": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		}
		result := make(map[string]interface{})
		extractValuesField(resMap, result)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})

	t.Run("without_values", func(t *testing.T) {
		resMap := map[string]interface{}{
			"other": "data",
		}
		result := make(map[string]interface{})
		extractValuesField(resMap, result)
		assert.Empty(t, result)
	})

	t.Run("values_not_map", func(t *testing.T) {
		resMap := map[string]interface{}{
			"values": "not a map",
		}
		result := make(map[string]interface{})
		extractValuesField(resMap, result)
		assert.Empty(t, result)
	})
}

// TestExtractChangeAfterField tests the extractChangeAfterField function.
func TestExtractChangeAfterField(t *testing.T) {
	t.Run("with_change_after", func(t *testing.T) {
		resMap := map[string]interface{}{
			"change": map[string]interface{}{
				"after": map[string]interface{}{
					"key1": "value1",
					"key2": "value2",
				},
			},
		}
		result := make(map[string]interface{})
		extractChangeAfterField(resMap, result)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})

	t.Run("without_change", func(t *testing.T) {
		resMap := map[string]interface{}{
			"other": "data",
		}
		result := make(map[string]interface{})
		extractChangeAfterField(resMap, result)
		assert.Empty(t, result)
	})

	t.Run("change_not_map", func(t *testing.T) {
		resMap := map[string]interface{}{
			"change": "not a map",
		}
		result := make(map[string]interface{})
		extractChangeAfterField(resMap, result)
		assert.Empty(t, result)
	})

	t.Run("after_not_map", func(t *testing.T) {
		resMap := map[string]interface{}{
			"change": map[string]interface{}{
				"after": "not a map",
			},
		}
		result := make(map[string]interface{})
		extractChangeAfterField(resMap, result)
		assert.Empty(t, result)
	})
}

// TestProcessRootModuleResources tests the processRootModuleResources function.
func TestProcessRootModuleResources(t *testing.T) {
	t.Run("with_resources", func(t *testing.T) {
		rootModule := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"type":    "aws_instance",
					"name":    "example",
				},
				map[string]interface{}{
					"address": "aws_s3_bucket.data",
					"mode":    "managed",
					"type":    "aws_s3_bucket",
					"name":    "data",
				},
			},
		}
		result := make(map[string]interface{})
		processRootModuleResources(rootModule, result)
		assert.Len(t, result, 2)
		assert.NotNil(t, result["aws_instance.example"])
		assert.NotNil(t, result["aws_s3_bucket.data"])
	})

	t.Run("skip_data_resources", func(t *testing.T) {
		rootModule := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
				},
				map[string]interface{}{
					"address": "data.aws_ami.ubuntu",
					"mode":    "data",
				},
			},
		}
		result := make(map[string]interface{})
		processRootModuleResources(rootModule, result)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["aws_instance.example"])
		assert.Nil(t, result["data.aws_ami.ubuntu"])
	})

	t.Run("no_resources", func(t *testing.T) {
		rootModule := map[string]interface{}{}
		result := make(map[string]interface{})
		processRootModuleResources(rootModule, result)
		assert.Empty(t, result)
	})

	t.Run("invalid_resource_format", func(t *testing.T) {
		rootModule := map[string]interface{}{
			"resources": []interface{}{
				"not a map",
				map[string]interface{}{
					// missing address
					"mode": "managed",
				},
				map[string]interface{}{
					"address": "aws_instance.example",
					// missing mode
				},
			},
		}
		result := make(map[string]interface{})
		processRootModuleResources(rootModule, result)
		assert.Empty(t, result)
	})
}

// TestProcessResourceChanges tests the processResourceChanges function.
func TestProcessResourceChanges(t *testing.T) {
	t.Run("with_resource_changes", func(t *testing.T) {
		plan := map[string]interface{}{
			"resource_changes": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []interface{}{"update"},
					},
				},
				map[string]interface{}{
					"address": "aws_s3_bucket.data",
					"mode":    "managed",
					"change": map[string]interface{}{
						"actions": []interface{}{"create"},
					},
				},
			},
		}
		result := make(map[string]interface{})
		processResourceChanges(plan, result)
		assert.Len(t, result, 2)
		assert.NotNil(t, result["aws_instance.example"])
		assert.NotNil(t, result["aws_s3_bucket.data"])
	})

	t.Run("skip_data_resources", func(t *testing.T) {
		plan := map[string]interface{}{
			"resource_changes": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.example",
					"mode":    "managed",
				},
				map[string]interface{}{
					"address": "data.aws_ami.ubuntu",
					"mode":    "data",
				},
			},
		}
		result := make(map[string]interface{})
		processResourceChanges(plan, result)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["aws_instance.example"])
	})

	t.Run("no_resource_changes", func(t *testing.T) {
		plan := map[string]interface{}{}
		result := make(map[string]interface{})
		processResourceChanges(plan, result)
		assert.Empty(t, result)
	})
}

// TestProcessPriorStateResources tests the processPriorStateResources function.
func TestProcessPriorStateResources(t *testing.T) {
	t.Run("with_prior_state", func(t *testing.T) {
		plan := map[string]interface{}{
			"prior_state": map[string]interface{}{
				"values": map[string]interface{}{
					"root_module": map[string]interface{}{
						"resources": []interface{}{
							map[string]interface{}{
								"address": "aws_instance.example",
								"mode":    "managed",
							},
						},
					},
				},
			},
		}
		result := make(map[string]interface{})
		processPriorStateResources(plan, result)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["aws_instance.example"])
	})

	t.Run("no_prior_state", func(t *testing.T) {
		plan := map[string]interface{}{}
		result := make(map[string]interface{})
		processPriorStateResources(plan, result)
		assert.Empty(t, result)
	})

	t.Run("invalid_prior_state_structure", func(t *testing.T) {
		plan := map[string]interface{}{
			"prior_state": "not a map",
		}
		result := make(map[string]interface{})
		processPriorStateResources(plan, result)
		assert.Empty(t, result)
	})
}

// TestProcessPlannedValuesResources tests the processPlannedValuesResources function.
func TestProcessPlannedValuesResources(t *testing.T) {
	t.Run("with_planned_values", func(t *testing.T) {
		plan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.example",
							"mode":    "managed",
						},
					},
				},
			},
		}
		result := make(map[string]interface{})
		processPlannedValuesResources(plan, result)
		assert.Len(t, result, 1)
		assert.NotNil(t, result["aws_instance.example"])
	})

	t.Run("no_planned_values", func(t *testing.T) {
		plan := map[string]interface{}{}
		result := make(map[string]interface{})
		processPlannedValuesResources(plan, result)
		assert.Empty(t, result)
	})
}

// TestGetResources tests the getResources function.
func TestGetResources(t *testing.T) {
	plan := map[string]interface{}{
		"prior_state": map[string]interface{}{
			"values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.prior",
							"mode":    "managed",
						},
					},
				},
			},
		},
		"planned_values": map[string]interface{}{
			"root_module": map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"address": "aws_instance.planned",
						"mode":    "managed",
					},
				},
			},
		},
		"resource_changes": []interface{}{
			map[string]interface{}{
				"address": "aws_instance.changed",
				"mode":    "managed",
			},
		},
	}

	resources := getResources(plan)
	assert.NotNil(t, resources["aws_instance.prior"])
	assert.NotNil(t, resources["aws_instance.planned"])
	assert.NotNil(t, resources["aws_instance.changed"])
}
