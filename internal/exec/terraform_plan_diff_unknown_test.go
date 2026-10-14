package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// unknownTestPlan builds a resource_changes plan for a local_file with explicit after and after_unknown maps.
func unknownTestPlan(t *testing.T, after, afterUnknown map[string]interface{}) map[string]interface{} {
	t.Helper()
	change := map[string]interface{}{"actions": []interface{}{"update"}, "before": after, "after": after}
	if afterUnknown != nil {
		change["after_unknown"] = afterUnknown
	}
	resource := map[string]interface{}{
		"address": "local_file.example",
		"mode":    "managed",
		"type":    "local_file",
		"change":  change,
	}
	return planDiffResourcePlan(t, resource, "resource_changes")
}

func TestPlanDiffKnownAfterApply(t *testing.T) {
	t.Parallel()
	legacy, err := planDiffPolicy(&schema.AtmosConfiguration{Edition: "2026-10-12"})
	require.NoError(t, err)

	tests := []struct {
		name         string
		origAfter    map[string]interface{}
		origUnknown  map[string]interface{}
		newAfter     map[string]interface{}
		newUnknown   map[string]interface{}
		opts         []planDiffOption
		contains     []string
		notContains  []string
		wantHasDiffs bool
	}{
		{
			name:        "orig known and new unknown",
			origAfter:   map[string]interface{}{"filename": "a.txt", "content_md5": "5d41"},
			newAfter:    map[string]interface{}{"filename": "b.txt"},
			newUnknown:  map[string]interface{}{"content_md5": true},
			contains:    []string{"~ content_md5: 5d41 => (known after apply)", "~ filename: a.txt => b.txt"},
			notContains: []string{"- content_md5:", "+ content_md5:"},
		},
		{
			name:        "orig unknown and new known",
			origAfter:   map[string]interface{}{"filename": "a.txt"},
			origUnknown: map[string]interface{}{"content_md5": true},
			newAfter:    map[string]interface{}{"filename": "b.txt", "content_md5": "abcd"},
			contains:    []string{"~ content_md5: (known after apply) => abcd", "~ filename: a.txt => b.txt"},
			notContains: []string{"- content_md5:", "+ content_md5:"},
		},
		{
			name:        "unknown on both sides prints nothing",
			origAfter:   map[string]interface{}{"filename": "a.txt"},
			origUnknown: map[string]interface{}{"content_md5": true},
			newAfter:    map[string]interface{}{"filename": "b.txt"},
			newUnknown:  map[string]interface{}{"content_md5": true},
			contains:    []string{"~ filename: a.txt => b.txt"},
			notContains: []string{"content_md5"},
		},
		{
			name:        "priority id known to unknown",
			origAfter:   map[string]interface{}{"id": "abc123", "filename": "a.txt"},
			newAfter:    map[string]interface{}{"filename": "b.txt"},
			newUnknown:  map[string]interface{}{"id": true},
			contains:    []string{"~ id: abc123 => (known after apply)"},
			notContains: []string{"- id:", "+ id:"},
		},
		{
			name:        "priority id unknown to known",
			origAfter:   map[string]interface{}{"filename": "a.txt"},
			origUnknown: map[string]interface{}{"id": true},
			newAfter:    map[string]interface{}{"id": "def456", "filename": "b.txt"},
			contains:    []string{"~ id: (known after apply) => def456"},
			notContains: []string{"- id:", "+ id:"},
		},
		{
			name:        "legacy display still hides content hashes",
			origAfter:   map[string]interface{}{"filename": "a.txt", "content_md5": "5d41", "content_sha256": "aaaa"},
			origUnknown: map[string]interface{}{"content_sha1": true},
			newAfter:    map[string]interface{}{"filename": "b.txt", "content_sha1": "bbbb"},
			newUnknown:  map[string]interface{}{"content_md5": true, "content_sha256": true},
			opts:        []planDiffOption{legacy},
			contains:    []string{"~ filename: a.txt => b.txt"},
			notContains: []string{"content_md5", "content_sha256", "content_sha1", "known after apply"},
		},
		{
			name:        "non-true unknown markers do not count",
			origAfter:   map[string]interface{}{"filename": "a.txt", "content_md5": "5d41", "sensitive_content": "x"},
			newAfter:    map[string]interface{}{"filename": "b.txt"},
			newUnknown:  map[string]interface{}{"content_md5": false, "sensitive_content": map[string]interface{}{"nested": true}},
			contains:    []string{"- content_md5: 5d41", "- sensitive_content: x"},
			notContains: []string{"known after apply"},
		},
		{
			name:        "unknown only counts when attribute is missing from that side",
			origAfter:   map[string]interface{}{"filename": "a.txt", "content_md5": "old"},
			newAfter:    map[string]interface{}{"filename": "a.txt", "content_md5": "new"},
			newUnknown:  map[string]interface{}{"content_md5": true},
			contains:    []string{"~ content_md5: old => new"},
			notContains: []string{"known after apply"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			orig := unknownTestPlan(t, tc.origAfter, tc.origUnknown)
			fresh := unknownTestPlan(t, tc.newAfter, tc.newUnknown)
			diff, hasDiff := generatePlanDiff(orig, fresh, tc.opts...)
			assert.True(t, hasDiff)
			for _, want := range tc.contains {
				assert.Contains(t, diff, want)
			}
			for _, notWant := range tc.notContains {
				assert.NotContains(t, diff, notWant)
			}
		})
	}
}

func TestPlanDiffKnownAfterApplyDoesNotChangeVerdict(t *testing.T) {
	t.Parallel()
	// The after_unknown record is part of the compared resource, so a known-to-unknown
	// transition keeps the verdict a difference even with no other attribute changes.
	orig := unknownTestPlan(t, map[string]interface{}{"filename": "a.txt", "content_md5": "5d41"}, nil)
	fresh := unknownTestPlan(t, map[string]interface{}{"filename": "a.txt"}, map[string]interface{}{"content_md5": true})
	diff, hasDiff := generatePlanDiff(orig, fresh)
	assert.True(t, hasDiff)
	assert.Contains(t, diff, "~ content_md5: 5d41 => (known after apply)")

	same := unknownTestPlan(t, map[string]interface{}{"filename": "a.txt"}, map[string]interface{}{"content_md5": true})
	_, hasDiff = generatePlanDiff(same, same)
	assert.False(t, hasDiff)
}
