package exec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// planDiffResourcePlan exercises the same JSON decoding and normalization as the command.
func planDiffResourcePlan(t *testing.T, resource map[string]interface{}, section string) map[string]interface{} {
	t.Helper()
	var plan map[string]interface{}
	if section == "resource_changes" {
		plan = map[string]interface{}{"resource_changes": []interface{}{resource}}
	} else {
		values := map[string]interface{}{"root_module": map[string]interface{}{"resources": []interface{}{resource}}}
		if section == "prior_state" {
			values = map[string]interface{}{"values": values}
		}
		plan = map[string]interface{}{section: values}
	}
	encoded, err := json.Marshal(plan)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	return sortMapKeys(decoded)
}

func planDiffTestResource(resourceType, attribute, value, section string) map[string]interface{} {
	values := map[string]interface{}{"id": "example", attribute: value}
	resource := map[string]interface{}{
		"address": "module.example." + resourceType + ".default[0]",
		"mode":    "managed",
		"type":    resourceType,
	}
	if section == "resource_changes" {
		resource["change"] = map[string]interface{}{
			"actions": []interface{}{"no-op"}, "before": values, "after": values,
		}
	} else {
		resource["values"] = values
	}
	return resource
}

func TestPlanDiffAttributeDisplayEditions(t *testing.T) {
	t.Parallel()
	for _, section := range []string{"planned_values", "prior_state", "resource_changes"} {
		for _, attribute := range []string{
			"response_body_base64", "content_base64sha256", "content_base64sha512",
			"content_md5", "content_sha1", "content_sha256", "content_sha512",
		} {
			t.Run(section+"/"+attribute, func(t *testing.T) {
				t.Parallel()
				orig := planDiffResourcePlan(t, planDiffTestResource("aws_s3_object", attribute, "old", section), section)
				fresh := planDiffResourcePlan(t, planDiffTestResource("aws_s3_object", attribute, "new", section), section)
				for _, pin := range []string{"", "2026-10-12", "2026-10-13", "2026-09", "2026-10", "2026"} {
					config := &schema.AtmosConfiguration{Edition: pin}
					policy, err := planDiffPolicy(config)
					require.NoError(t, err)
					diff, hasDiff := generatePlanDiff(orig, fresh, policy)
					assert.True(t, hasDiff, "hash changes must affect the verdict under edition %q", pin)
					if pin == "2026-10-12" || pin == "2026-09" {
						assert.NotContains(t, diff, attribute)
						assert.Contains(t, diff, "module.example.aws_s3_object.default[0]")
					} else {
						assert.Contains(t, diff, attribute+": old => new")
					}

					config.Components.Terraform.PlanDiff.IgnoreChanges = []string{"aws_s3_object." + attribute}
					policy, err = planDiffPolicy(config)
					require.NoError(t, err)
					diff, hasDiff = generatePlanDiff(orig, fresh, policy)
					assert.False(t, hasDiff, "explicit exclusion must work under edition %q", pin)
					assert.Empty(t, diff)
				}
			})
		}
	}
}

func TestPlanDiffRDSExclusions(t *testing.T) {
	t.Parallel()
	for _, section := range []string{"planned_values", "prior_state", "resource_changes"} {
		t.Run(section, func(t *testing.T) {
			t.Parallel()
			origResource := planDiffTestResource("aws_db_instance", "latest_restorable_time", "2026-02-17T22:02:50Z", section)
			newResource := planDiffTestResource("aws_db_instance", "latest_restorable_time", "2026-02-17T22:07:50Z", section)
			orig := planDiffResourcePlan(t, origResource, section)
			fresh := planDiffResourcePlan(t, newResource, section)
			origSnapshot, err := json.Marshal(orig)
			require.NoError(t, err)
			freshSnapshot, err := json.Marshal(fresh)
			require.NoError(t, err)

			diff, hasDiff := generatePlanDiff(orig, fresh)
			assert.True(t, hasDiff)
			assert.Contains(t, diff, "latest_restorable_time: 2026-02-17T22:02:50Z => 2026-02-17T22:07:50Z")

			option := withPlanDiffIgnoreChanges([]string{"aws_db_instance.latest_restorable_time"})
			diff, hasDiff = generatePlanDiff(orig, fresh, option)
			assert.False(t, hasDiff)
			assert.Empty(t, diff)
			origAfter, err := json.Marshal(orig)
			require.NoError(t, err)
			freshAfter, err := json.Marshal(fresh)
			require.NoError(t, err)
			assert.Equal(t, string(origSnapshot), string(origAfter))
			assert.Equal(t, string(freshSnapshot), string(freshAfter))

			// An exclusion for another resource type must not suppress this timestamp.
			_, hasDiff = generatePlanDiff(orig, fresh, withPlanDiffIgnoreChanges([]string{"aws_rds_cluster.latest_restorable_time"}))
			assert.True(t, hasDiff)

			if section == "resource_changes" {
				newResource["change"].(map[string]interface{})["after"].(map[string]interface{})["instance_class"] = "db.t3.small"
			} else {
				newResource["values"].(map[string]interface{})["instance_class"] = "db.t3.small"
			}
			diff, hasDiff = generatePlanDiff(orig, planDiffResourcePlan(t, newResource, section), option)
			assert.True(t, hasDiff)
			assert.Contains(t, diff, "instance_class")
			assert.NotContains(t, diff, "latest_restorable_time")
		})
	}
}

func TestPlanDiffExclusionsAttributeMaps(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"values", "sensitive_values", "before", "after", "after_unknown", "before_sensitive", "after_sensitive"} {
		for _, change := range []string{"changed", "added", "removed"} {
			t.Run(field+"/"+change, func(t *testing.T) {
				t.Parallel()
				origResource := planDiffTestResource("aws_db_instance", "name", "unchanged", "resource_changes")
				newResource := planDiffTestResource("aws_db_instance", "name", "unchanged", "resource_changes")
				origAttrs := map[string]interface{}{"latest_restorable_time": "old"}
				newAttrs := map[string]interface{}{"latest_restorable_time": "new"}
				if field != "values" && field != "before" && field != "after" {
					origAttrs["latest_restorable_time"] = false
					newAttrs["latest_restorable_time"] = true
				}
				if change == "added" {
					delete(origAttrs, "latest_restorable_time")
				}
				if change == "removed" {
					delete(newAttrs, "latest_restorable_time")
				}
				if field == "values" || field == "sensitive_values" {
					origResource[field], newResource[field] = origAttrs, newAttrs
				} else {
					origResource["change"].(map[string]interface{})[field] = origAttrs
					newResource["change"].(map[string]interface{})[field] = newAttrs
				}
				orig := planDiffResourcePlan(t, origResource, "resource_changes")
				fresh := planDiffResourcePlan(t, newResource, "resource_changes")
				_, hasDiff := generatePlanDiff(orig, fresh)
				assert.True(t, hasDiff, "control must detect the unfiltered difference")
				diff, hasDiff := generatePlanDiff(orig, fresh, withPlanDiffIgnoreChanges([]string{"aws_db_instance.latest_restorable_time"}))
				assert.False(t, hasDiff)
				assert.Empty(t, diff)
			})
		}
	}
}

func TestPlanDiffExclusionsPreserveMeaningfulChanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{"actions", func(resource map[string]interface{}) {
			resource["change"].(map[string]interface{})["actions"] = []interface{}{"delete", "create"}
		}},
		{"replacement paths", func(resource map[string]interface{}) {
			resource["change"].(map[string]interface{})["replace_paths"] = []interface{}{[]interface{}{"latest_restorable_time"}}
		}},
		{"null before", func(resource map[string]interface{}) { resource["change"].(map[string]interface{})["before"] = nil }},
		{"null after", func(resource map[string]interface{}) { resource["change"].(map[string]interface{})["after"] = nil }},
		{"nested same name", func(resource map[string]interface{}) {
			resource["change"].(map[string]interface{})["after"] = map[string]interface{}{
				"settings": map[string]interface{}{"latest_restorable_time": "must remain"},
			}
		}},
		{"resource identity", func(resource map[string]interface{}) { resource["address"] = "aws_db_instance.another" }},
		{"resource type", func(resource map[string]interface{}) { resource["type"] = "another_type" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orig := planDiffResourcePlan(t, planDiffTestResource("aws_db_instance", "latest_restorable_time", "old", "resource_changes"), "resource_changes")
			resource := planDiffTestResource("aws_db_instance", "latest_restorable_time", "new", "resource_changes")
			tc.mutate(resource)
			_, hasDiff := generatePlanDiff(orig, planDiffResourcePlan(t, resource, "resource_changes"),
				withPlanDiffIgnoreChanges([]string{"aws_db_instance.latest_restorable_time"}))
			assert.True(t, hasDiff)
		})
	}
}

func TestPlanDiffExclusionsPreserveOtherSections(t *testing.T) {
	t.Parallel()
	option := withPlanDiffIgnoreChanges([]string{"aws_db_instance.latest_restorable_time"})
	resourcePlan := planDiffResourcePlan(t, planDiffTestResource("aws_db_instance", "latest_restorable_time", "old", "resource_changes"), "resource_changes")
	for _, plans := range [][2]map[string]interface{}{
		{{}, resourcePlan},
		{resourcePlan, {}},
		{{}, {"variables": map[string]interface{}{"latest_restorable_time": map[string]interface{}{"value": "new"}}}},
		{{}, {"output_changes": map[string]interface{}{"latest_restorable_time": map[string]interface{}{"after": "new"}}}},
	} {
		_, hasDiff := generatePlanDiff(plans[0], plans[1], option)
		assert.True(t, hasDiff)
	}
}

func TestPlanDiffConfiguredExclusionsDoNotSuppressHashes(t *testing.T) {
	t.Parallel()
	resource := planDiffTestResource("aws_db_instance", "latest_restorable_time", "old", "planned_values")
	resource["values"].(map[string]interface{})["content_md5"] = "old hash"
	orig := planDiffResourcePlan(t, resource, "planned_values")
	resource["values"].(map[string]interface{})["content_md5"] = "new hash"
	resource["values"].(map[string]interface{})["latest_restorable_time"] = "new"
	fresh := planDiffResourcePlan(t, resource, "planned_values")
	diff, hasDiff := generatePlanDiff(orig, fresh, withPlanDiffIgnoreChanges([]string{"aws_db_instance.latest_restorable_time"}))
	assert.True(t, hasDiff)
	assert.Contains(t, diff, "content_md5: old hash => new hash")
	assert.NotContains(t, diff, "latest_restorable_time")
}

func TestPlanDiffLegacyDisplayAttributePresence(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"added", "removed"} {
		for _, pin := range []string{"2026-10-12", "2026-10-13"} {
			t.Run(change+"/"+pin, func(t *testing.T) {
				t.Parallel()
				orig := planDiffTestResource("local_file", "content_sha256", "old", "planned_values")
				fresh := planDiffTestResource("local_file", "content_sha256", "new", "planned_values")
				if change == "added" {
					delete(orig["values"].(map[string]interface{}), "content_sha256")
				} else {
					delete(fresh["values"].(map[string]interface{}), "content_sha256")
				}
				fresh["values"].(map[string]interface{})["filename"] = "changed.txt"
				policy, err := planDiffPolicy(&schema.AtmosConfiguration{Edition: pin})
				require.NoError(t, err)
				diff, hasDiff := generatePlanDiff(planDiffResourcePlan(t, orig, "planned_values"),
					planDiffResourcePlan(t, fresh, "planned_values"), policy)
				assert.True(t, hasDiff)
				assert.Contains(t, diff, "filename: changed.txt")
				if pin == "2026-10-12" {
					assert.NotContains(t, diff, "content_sha256")
				} else {
					assert.Contains(t, diff, "content_sha256")
				}
			})
		}
	}
}
