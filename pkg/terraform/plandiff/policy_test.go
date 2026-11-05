package plandiff

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPolicyRejectsInvalidIgnoreChanges(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.PlanDiff.IgnoreChanges = []string{"*.latest_restorable_time"}
	policy, err := Policy(config)
	assert.ErrorIs(t, err, schema.ErrInvalidPlanDiffIgnoreChange)
	assert.Nil(t, policy)
}

func TestPolicyRejectsInvalidEdition(t *testing.T) {
	t.Parallel()
	policy, err := Policy(&schema.AtmosConfiguration{Edition: "not-a-date"})
	assert.ErrorIs(t, err, edition.ErrInvalidEdition)
	assert.Nil(t, policy)
}

func TestPolicyOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		pin        string
		entries    []string
		wantLegacy bool
	}{
		{name: "unconfigured shows computed attributes"},
		{name: "older edition keeps legacy display", pin: "2026-10-12", wantLegacy: true},
		{name: "current edition with entries", pin: "2026-10-13", entries: []string{"aws_db_instance.latest_restorable_time"}},
		{name: "older edition with entries", pin: "2026-10-12", entries: []string{"aws_db_instance.latest_restorable_time"}, wantLegacy: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			config := &schema.AtmosConfiguration{Edition: tc.pin}
			config.Components.Terraform.PlanDiff.IgnoreChanges = tc.entries
			policy, err := Policy(config)
			require.NoError(t, err)
			got := newOptions(policy)
			assert.Equal(t, tc.wantLegacy, got.legacyDisplay)
			assert.Equal(t, tc.entries, got.ignoreChanges)
		})
	}
}

func TestOptionsCompose(t *testing.T) {
	t.Parallel()
	got := newOptions(
		WithIgnoreChanges([]string{"a.b"}),
		WithIgnoreChanges([]string{"c.d"}),
		WithLegacyDisplay(true),
	)
	assert.Equal(t, []string{"a.b", "c.d"}, got.ignoreChanges)
	assert.True(t, got.legacyDisplay)
	assert.True(t, got.ignores("c", "d"))
	assert.False(t, got.ignores("a", "d"))
}
