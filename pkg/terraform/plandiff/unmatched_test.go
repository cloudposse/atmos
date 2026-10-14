package plandiff

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// planDiffTypedResource builds a resource record with explicit attribute sections.
func planDiffTypedResource(resourceType string, sections map[string]interface{}, change map[string]interface{}) map[string]interface{} {
	record := map[string]interface{}{"address": resourceType + ".this", "mode": "managed", "type": resourceType}
	for field, value := range sections {
		record[field] = value
	}
	if change != nil {
		record["change"] = change
	}
	return record
}

func TestUnmatchedPlanDiffIgnoreChanges(t *testing.T) {
	t.Parallel()
	dataValues := map[string]interface{}{"id": "x", "output": nil, "input": nil}
	tests := []struct {
		name      string
		entries   []string
		resources map[string]interface{}
		want      []string
	}{
		{
			name:      "resource address reported",
			entries:   []string{"terraform_data.db"},
			resources: map[string]interface{}{"a": planDiffTypedResource("terraform_data", map[string]interface{}{"values": dataValues}, nil)},
			want:      []string{"terraform_data.db"},
		},
		{
			name:      "typo reported",
			entries:   []string{"terraform_data.outptu", "terraform_data.output"},
			resources: map[string]interface{}{"a": planDiffTypedResource("terraform_data", map[string]interface{}{"values": dataValues}, nil)},
			want:      []string{"terraform_data.outptu"},
		},
		{
			name:    "attribute only in after_unknown is matched",
			entries: []string{"terraform_data.output"},
			resources: map[string]interface{}{"a": planDiffTypedResource("terraform_data", nil,
				map[string]interface{}{"after": map[string]interface{}{"id": "x"}, "after_unknown": map[string]interface{}{"output": true}})},
		},
		{
			name:    "attribute only in before is matched",
			entries: []string{"terraform_data.output"},
			resources: map[string]interface{}{"a": planDiffTypedResource("terraform_data", nil,
				map[string]interface{}{"before": map[string]interface{}{"output": "old"}})},
		},
		{
			name:      "attribute only in sensitive_values is matched",
			entries:   []string{"terraform_data.output"},
			resources: map[string]interface{}{"a": planDiffTypedResource("terraform_data", map[string]interface{}{"sensitive_values": map[string]interface{}{"output": true}}, nil)},
		},
		{
			name:      "absent type is not reported",
			entries:   []string{"aws_db_instance.latest_restorable_time", "aws_db_instance"},
			resources: map[string]interface{}{"a": planDiffTypedResource("terraform_data", map[string]interface{}{"values": dataValues}, nil)},
		},
		{
			name:      "duplicates reported once in config order",
			entries:   []string{"terraform_data.zzz", "terraform_data.aaa", "terraform_data.zzz", "terraform_data.aaa"},
			resources: map[string]interface{}{"a": planDiffTypedResource("terraform_data", map[string]interface{}{"values": dataValues}, nil)},
			want:      []string{"terraform_data.zzz", "terraform_data.aaa"},
		},
		{
			name:      "no resources reports nothing",
			entries:   []string{"terraform_data.db"},
			resources: map[string]interface{}{},
		},
		{
			name:    "any resource of the type matching is enough",
			entries: []string{"aws_instance.ami"},
			resources: map[string]interface{}{
				"a": planDiffTypedResource("aws_instance", map[string]interface{}{"values": map[string]interface{}{"id": "i-1"}}, nil),
				"b": planDiffTypedResource("aws_instance", map[string]interface{}{"values": map[string]interface{}{"ami": "ami-1"}}, nil),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := unmatchedIgnoreChanges(tc.entries, tc.resources)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUnmatchedPlanDiffIgnoreChangesSpansBothPlans(t *testing.T) {
	t.Parallel()
	orig := map[string]interface{}{"a": planDiffTypedResource("aws_instance", map[string]interface{}{"values": map[string]interface{}{"id": "i-1"}}, nil)}
	fresh := map[string]interface{}{"a": planDiffTypedResource("aws_instance", map[string]interface{}{"values": map[string]interface{}{"id": "i-1", "ami": "ami-1"}}, nil)}
	assert.Empty(t, unmatchedIgnoreChanges([]string{"aws_instance.ami"}, orig, fresh))
	assert.Equal(t, []string{"aws_instance.ami"}, unmatchedIgnoreChanges([]string{"aws_instance.ami"}, orig))
}

func TestWarnUnmatchedPlanDiffIgnoreChangesDoesNotChangeDiff(t *testing.T) {
	t.Parallel()
	orig := planDiffResourcePlan(t, planDiffTestResource("terraform_data", "output", "old", "planned_values"), "planned_values")
	fresh := planDiffResourcePlan(t, planDiffTestResource("terraform_data", "output", "new", "planned_values"), "planned_values")
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.PlanDiff.IgnoreChanges = []string{"terraform_data.db", "terraform_data.output"}
	policy, err := Policy(config)
	require.NoError(t, err)

	wantDiff, wantHasDiff := generatePlanDiff(orig, fresh, policy)
	require.Equal(t, []string{"terraform_data.db"}, unmatchedIgnoreChanges(
		config.Components.Terraform.PlanDiff.IgnoreChanges, getResources(orig), getResources(fresh),
	))
	WarnUnmatchedIgnoreChanges(config.Components.Terraform.PlanDiff.IgnoreChanges, orig, fresh)
	gotDiff, gotHasDiff := generatePlanDiff(orig, fresh, policy)

	assert.Equal(t, wantHasDiff, gotHasDiff)
	assert.Equal(t, wantDiff, gotDiff)
	assert.False(t, gotHasDiff, "terraform_data.output is excluded so the plans are equivalent")
}

func TestUnmatchedIgnoreChangesFromPlans(t *testing.T) {
	t.Parallel()
	orig := planDiffResourcePlan(t, planDiffTestResource("aws_instance", "ami", "old", "planned_values"), "planned_values")
	fresh := planDiffResourcePlan(t, planDiffTestResource("aws_instance", "ami", "new", "planned_values"), "planned_values")

	got := UnmatchedIgnoreChanges([]string{"aws_instance.ami", "aws_instance.amy", "aws_instance.amy", "aws_s3_bucket.acl"}, orig, fresh)

	assert.Equal(t, []string{"aws_instance.amy"}, got)
	assert.Empty(t, UnmatchedIgnoreChanges(nil, orig, fresh))
}
