package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRegisteredEvents(t *testing.T) {
	for _, event := range []string{
		"after.terraform.apply", "before.terraform.deploy", "after-terraform-apply",
		"before-terraform-deploy", "after.terraform.destroy.aggregate",
		"before.terraform.destroy.aggregate", "before.helm.delete", "after.helm.delete",
		"before.helmfile.destroy", "after.helmfile.destroy", "after.helmfile.version",
		"after.helmfile.sync", "before.kubernetes.plan", "after.aws/cloudformation.deploy",
		"before.aws/cloudformation.changeset-create", "after.aws/cloudformation.drift-detect",
		"before.scaffold.generate", "after.scaffold.generate", "after.helm.apply.aggregate",
	} {
		t.Run(event, func(t *testing.T) {
			require.True(t, DefaultRegistry.Contains(event))
			require.NoError(t, DefaultRegistry.Validate("", map[string][]string{"hook": {event}}))
		})
	}
	for _, event := range []string{"", "*", "after.terraform.aply", "before.helm.destroy", "after.widgets.deploy", "before.terraform.destroy", "Before.terraform.plan"} {
		t.Run("unsupported="+event, func(t *testing.T) {
			require.False(t, DefaultRegistry.Contains(event))
			err := DefaultRegistry.Validate("", map[string][]string{"hook": {event}})
			require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
			require.Contains(t, err.Error(), event)
		})
	}
}

func TestRegistryRegistrationAndIsolation(t *testing.T) {
	r := NewRegistry(AfterTerraformPlan)
	require.ErrorIs(t, r.Validate("", map[string][]string{"custom": {"after.widgets.deploy"}}), errUtils.ErrUnsupportedHookEvent)
	r.Register("after.widgets.deploy", "after.widgets.deploy")
	require.NoError(t, r.Validate("", map[string][]string{"custom": {"after.widgets.deploy"}}))
	names := r.Names()
	assert.Equal(t, []HookEvent{AfterTerraformPlan, "after.widgets.deploy"}, names)
	names[0] = "changed"
	assert.True(t, r.Contains(string(AfterTerraformPlan)))
	assert.False(t, DefaultRegistry.Contains("after.widgets.deploy"))
}

func TestValidateEditionBoundary(t *testing.T) {
	for _, tc := range []struct {
		pin    string
		strict bool
	}{
		{"2026-10-18", false},
		{"2026-09", false},
		{"2025", false},
		{"2026-10-19", true},
		{"2026-10-20", true},
		{"2026-10", true},
		{"2026", true},
		{"", true},
	} {
		t.Run(tc.pin, func(t *testing.T) {
			declarations := map[string][]string{"typo": {"after.terraform.aply"}}
			err := DefaultRegistry.Validate(tc.pin, declarations)
			if tc.strict {
				require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, DefaultRegistry.Validate(tc.pin, map[string][]string{"all": nil, "also-all": {}}))
		})
	}
	require.Error(t, DefaultRegistry.Validate("2026-02-30", nil))
	require.Error(t, ValidateSection("invalid", nil))
}

func TestValidateSectionReportsAllEventsInStableOrder(t *testing.T) {
	section := map[string]any{
		"z-hook": map[string]any{"events": []string{"after.terraform.aply", "after.terraform.plan"}},
		"a-hook": map[string]any{"events": []string{"before.helm.destroy"}},
	}
	err := ValidateSection("", section)
	require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
	require.Contains(t, err.Error(), `hook "a-hook" declares "before.helm.destroy"`)
	require.Contains(t, err.Error(), `hook "z-hook" declares "after.terraform.aply"`)
	require.NotContains(t, err.Error(), "after.terraform.plan")
	require.NoError(t, ValidateSection("2026-10-18", section))
	require.NoError(t, ValidateSection("", nil))
	// Scope-specific structural validation retains its existing error types.
	require.NoError(t, ValidateSection("", "not a hook map"))
	require.ErrorIs(t, ValidateSection("", map[string]any{"hook": map[string]any{"events": []any{"after.terraform.aply"}}}), errUtils.ErrUnsupportedHookEvent)
	require.ErrorIs(t, ValidateSection("", map[string]any{"hook": map[string]any{"events": []any{42}}}), errUtils.ErrInvalidHooksSection)
}
