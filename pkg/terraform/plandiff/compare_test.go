package plandiff

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortMapKeys(t *testing.T) {
	// Test with a simple map
	input := map[string]interface{}{
		"c": 3,
		"a": 1,
		"b": 2,
	}

	expected := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": 3,
	}

	result := sortMapKeys(input)
	assert.Equal(t, expected, result)

	// Test with nested maps
	nestedInput := map[string]interface{}{
		"z": map[string]interface{}{
			"y": 2,
			"x": 1,
		},
		"a": 1,
	}

	nestedExpected := map[string]interface{}{
		"a": 1,
		"z": map[string]interface{}{
			"x": 1,
			"y": 2,
		},
	}

	nestedResult := sortMapKeys(nestedInput)
	assert.Equal(t, nestedExpected, nestedResult)

	// Test with arrays of maps
	arrayInput := map[string]interface{}{
		"arr": []interface{}{
			map[string]interface{}{
				"c": 3,
				"a": 1,
			},
			map[string]interface{}{
				"z": 26,
				"y": 25,
			},
		},
	}

	arrayExpected := map[string]interface{}{
		"arr": []interface{}{
			map[string]interface{}{
				"a": 1,
				"c": 3,
			},
			map[string]interface{}{
				"y": 25,
				"z": 26,
			},
		},
	}

	arrayResult := sortMapKeys(arrayInput)
	assert.Equal(t, arrayExpected, arrayResult)
}

func TestGeneratePlanDiff(t *testing.T) {
	// Test with identical plans
	origPlan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "Stockholm",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/Stockholm",
				},
			},
		},
	}

	diff, hasDiff := generatePlanDiff(origPlan, origPlan)
	assert.False(t, hasDiff)
	assert.Empty(t, diff)

	// Test with different plans
	newPlan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "New York",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/New+York",
				},
			},
		},
	}

	diff, hasDiff = generatePlanDiff(origPlan, newPlan)
	assert.True(t, hasDiff)
	assert.Contains(t, diff, "~ location: Stockholm => New York")
	assert.Contains(t, diff, "~ url: https://example.com/Stockholm => https://example.com/New+York")

	// Test with sensitive output values (Terraform marks outputs as sensitive)
	sensitiveOutputPlan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "Stockholm",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/Stockholm",
				},
				"api_key": map[string]interface{}{
					"sensitive": true,
					"value":     "api_key_12345",
				},
			},
		},
	}

	newSensitiveOutputPlan := map[string]interface{}{
		"variables": map[string]interface{}{
			"location": map[string]interface{}{
				"value": "New York",
			},
		},
		"planned_values": map[string]interface{}{
			"outputs": map[string]interface{}{
				"url": map[string]interface{}{
					"sensitive": false,
					"value":     "https://example.com/New+York",
				},
				"api_key": map[string]interface{}{
					"sensitive": true,
					"value":     "api_key_67890",
				},
			},
		},
	}

	diff, hasDiff = generatePlanDiff(sensitiveOutputPlan, newSensitiveOutputPlan)
	assert.True(t, hasDiff)
	assert.Contains(t, diff, "~ location: Stockholm => New York")
	assert.Contains(t, diff, "~ url: https://example.com/Stockholm => https://example.com/New+York")
	assert.Contains(t, diff, "~ api_key: (sensitive value) => (sensitive value)")
	assert.NotContains(t, diff, "api_key_12345")
	assert.NotContains(t, diff, "api_key_67890")
}

// TestCompareResources tests the compareResources function.
func TestCompareResources(t *testing.T) {
	t.Run("added_resources", func(t *testing.T) {
		origResources := map[string]interface{}{}
		newResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
			},
		}
		result := compareResources(origResources, newResources)
		assert.Contains(t, result, "+ aws_instance.example")
	})

	t.Run("removed_resources", func(t *testing.T) {
		origResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
			},
		}
		newResources := map[string]interface{}{}
		result := compareResources(origResources, newResources)
		assert.Contains(t, result, "- aws_instance.example")
	})

	t.Run("changed_resources", func(t *testing.T) {
		origResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
				"values": map[string]interface{}{
					"instance_type": "t2.micro",
				},
			},
		}
		newResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
				"values": map[string]interface{}{
					"instance_type": "t2.large",
				},
			},
		}
		result := compareResources(origResources, newResources)
		assert.Contains(t, result, "aws_instance.example")
		assert.Contains(t, result, "instance_type")
	})

	t.Run("no_changes", func(t *testing.T) {
		resources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"address": "aws_instance.example",
			},
		}
		result := compareResources(resources, resources)
		assert.Empty(t, result)
	})
}

// TestProcessResourceAdditionsAndRemovals tests the processResourceAdditionsAndRemovals function.
func TestProcessResourceAdditionsAndRemovals(t *testing.T) {
	t.Run("additions_and_removals", func(t *testing.T) {
		var diff strings.Builder
		origResources := map[string]interface{}{
			"aws_instance.old": map[string]interface{}{},
		}
		newResources := map[string]interface{}{
			"aws_instance.new": map[string]interface{}{},
		}
		processResourceAdditionsAndRemovals(&diff, origResources, newResources)
		result := diff.String()
		assert.Contains(t, result, "+ aws_instance.new")
		assert.Contains(t, result, "- aws_instance.old")
	})
}

// TestProcessChangedResources tests the processChangedResources function.
func TestProcessChangedResources(t *testing.T) {
	t.Run("with_attribute_changes", func(t *testing.T) {
		var diff strings.Builder
		origResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"values": map[string]interface{}{
					"instance_type": "t2.micro",
					"ami":           "ami-12345",
				},
			},
		}
		newResources := map[string]interface{}{
			"aws_instance.example": map[string]interface{}{
				"values": map[string]interface{}{
					"instance_type": "t2.large",
					"ami":           "ami-12345",
				},
			},
		}
		processChangedResources(&diff, origResources, newResources)
		result := diff.String()
		assert.Contains(t, result, "aws_instance.example")
		assert.Contains(t, result, "instance_type")
	})
}

// TestProcessAttributeDifferences tests the processAttributeDifferences function.
func TestProcessAttributeDifferences(t *testing.T) {
	t.Run("priority_attributes", func(t *testing.T) {
		var diff strings.Builder
		origAttrs := map[string]interface{}{
			"id":  "old-id",
			"url": "http://old.com",
		}
		newAttrs := map[string]interface{}{
			"id":  "new-id",
			"url": "http://new.com",
		}
		processAttributeDifferences(&diff, origAttrs, newAttrs)
		result := diff.String()
		// Priority attributes (id, url) should be shown
		assert.Contains(t, result, "id")
		assert.Contains(t, result, "url")
	})

	t.Run("formatter_does_not_hide_unfiltered_attributes", func(t *testing.T) {
		var diff strings.Builder
		origAttrs := map[string]interface{}{
			"content":              "old content",
			"content_md5":          "old-hash",
			"content_sha256":       "old-sha",
			"response_body_base64": "old-body",
		}
		newAttrs := map[string]interface{}{
			"content":              "new content",
			"content_md5":          "new-hash",
			"content_sha256":       "new-sha",
			"response_body_base64": "new-body",
		}
		processAttributeDifferences(&diff, origAttrs, newAttrs)
		result := diff.String()
		// content is a priority attribute, so it should be shown
		assert.Contains(t, result, "content")
		// Filtering belongs to the comparator; the formatter displays all remaining attributes.
		assert.Contains(t, result, "content_md5")
		assert.Contains(t, result, "content_sha256")
		assert.Contains(t, result, "response_body_base64")
	})

	t.Run("added_and_removed_attributes", func(t *testing.T) {
		var diff strings.Builder
		origAttrs := map[string]interface{}{
			"old_attr": "old_value",
		}
		newAttrs := map[string]interface{}{
			"new_attr": "new_value",
		}
		processAttributeDifferences(&diff, origAttrs, newAttrs)
		result := diff.String()
		assert.Contains(t, result, "- old_attr")
		assert.Contains(t, result, "+ new_attr")
	})
}

// TestCompareVariables tests the compareVariables function.
func TestCompareVariables(t *testing.T) {
	t.Run("no_difference", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "Stockholm"},
			},
		}
		diff, hasDiff := compareVariables(origPlan, origPlan)
		assert.False(t, hasDiff)
		assert.Empty(t, diff)
	})

	t.Run("added_variable", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"variables": map[string]interface{}{},
		}
		newPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "Stockholm"},
			},
		}
		diff, hasDiff := compareVariables(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "+ location")
	})

	t.Run("removed_variable", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "Stockholm"},
			},
		}
		newPlan := map[string]interface{}{
			"variables": map[string]interface{}{},
		}
		diff, hasDiff := compareVariables(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "- location")
	})

	t.Run("changed_variable", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "Stockholm"},
			},
		}
		newPlan := map[string]interface{}{
			"variables": map[string]interface{}{
				"location": map[string]interface{}{"value": "New York"},
			},
		}
		diff, hasDiff := compareVariables(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "~ location")
	})
}

// TestCompareOutputSections tests the compareOutputSections function.
func TestCompareOutputSections(t *testing.T) {
	t.Run("no_difference", func(t *testing.T) {
		plan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"outputs": map[string]interface{}{
					"url": map[string]interface{}{"value": "http://example.com"},
				},
			},
		}
		diff, hasDiff := compareOutputSections(plan, plan)
		assert.False(t, hasDiff)
		assert.Empty(t, diff)
	})

	t.Run("with_difference", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"outputs": map[string]interface{}{
					"url": map[string]interface{}{"value": "http://old.com"},
				},
			},
		}
		newPlan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"outputs": map[string]interface{}{
					"url": map[string]interface{}{"value": "http://new.com"},
				},
			},
		}
		diff, hasDiff := compareOutputSections(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "Outputs:")
	})
}

// TestCompareResourceSections tests the compareResourceSections function.
func TestCompareResourceSections(t *testing.T) {
	t.Run("no_difference", func(t *testing.T) {
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
		diff, hasDiff := compareResourceSections(plan, plan)
		assert.False(t, hasDiff)
		assert.Empty(t, diff)
	})

	t.Run("with_difference", func(t *testing.T) {
		origPlan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.old",
							"mode":    "managed",
						},
					},
				},
			},
		}
		newPlan := map[string]interface{}{
			"planned_values": map[string]interface{}{
				"root_module": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"address": "aws_instance.new",
							"mode":    "managed",
						},
					},
				},
			},
		}
		diff, hasDiff := compareResourceSections(origPlan, newPlan)
		assert.True(t, hasDiff)
		assert.Contains(t, diff, "Resources:")
	})
}

// TestCompareOutputs tests the compareOutputs function.
func TestCompareOutputs(t *testing.T) {
	t.Run("added_output", func(t *testing.T) {
		origOutputs := map[string]interface{}{}
		newOutputs := map[string]interface{}{
			"url": map[string]interface{}{"value": "http://example.com"},
		}
		result := compareOutputs(origOutputs, newOutputs)
		assert.Contains(t, result, "+ url")
	})

	t.Run("removed_output", func(t *testing.T) {
		origOutputs := map[string]interface{}{
			"url": map[string]interface{}{"value": "http://example.com"},
		}
		newOutputs := map[string]interface{}{}
		result := compareOutputs(origOutputs, newOutputs)
		assert.Contains(t, result, "- url")
	})

	t.Run("changed_output", func(t *testing.T) {
		origOutputs := map[string]interface{}{
			"url": map[string]interface{}{"value": "http://old.com"},
		}
		newOutputs := map[string]interface{}{
			"url": map[string]interface{}{"value": "http://new.com"},
		}
		result := compareOutputs(origOutputs, newOutputs)
		assert.Contains(t, result, "~ url")
	})
}

// TestCompareAppliesExclusionsToVerdictAndDisplay verifies the exported entry point end to end.
func TestCompareAppliesExclusionsToVerdictAndDisplay(t *testing.T) {
	t.Parallel()
	orig := planDiffResourcePlan(t, planDiffTestResource("aws_db_instance", "latest_restorable_time", "old", "resource_changes"), "resource_changes")
	fresh := planDiffResourcePlan(t, planDiffTestResource("aws_db_instance", "latest_restorable_time", "new", "resource_changes"), "resource_changes")

	diff, hasDiff := Compare(orig, fresh)
	assert.True(t, hasDiff)
	assert.Contains(t, diff, "latest_restorable_time: old => new")

	diff, hasDiff = Compare(orig, fresh, WithIgnoreChanges([]string{"aws_db_instance.latest_restorable_time"}))
	assert.False(t, hasDiff)
	assert.Empty(t, diff)

	// Other resource types are unaffected by an exclusion for a different type.
	diff, hasDiff = Compare(orig, fresh, WithIgnoreChanges([]string{"aws_s3_bucket.latest_restorable_time"}))
	assert.True(t, hasDiff)
	assert.Contains(t, diff, "latest_restorable_time: old => new")
}

// TestCompareNormalizesKeysWithoutMutatingInputs verifies callers do not need to sort the plans.
func TestCompareNormalizesKeysWithoutMutatingInputs(t *testing.T) {
	t.Parallel()
	orig := map[string]interface{}{"variables": map[string]interface{}{"a": map[string]interface{}{"value": "1"}}}
	fresh := map[string]interface{}{"variables": map[string]interface{}{"a": map[string]interface{}{"value": "2"}}}

	wantDiff, wantHasDiff := generatePlanDiff(sortMapKeys(orig), sortMapKeys(fresh))
	gotDiff, gotHasDiff := Compare(orig, fresh)

	assert.Equal(t, wantHasDiff, gotHasDiff)
	assert.Equal(t, wantDiff, gotDiff)
	assert.Equal(t, "1", orig["variables"].(map[string]interface{})["a"].(map[string]interface{})["value"])
	assert.Equal(t, "2", fresh["variables"].(map[string]interface{})["a"].(map[string]interface{})["value"])

	_, hasDiff := Compare(orig, orig)
	assert.False(t, hasDiff)
}
