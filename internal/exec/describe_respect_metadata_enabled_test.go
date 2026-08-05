package exec

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDisabledComponentTerraformSkip verifies the skip-list augmentation used to suppress
// !terraform.* resolution for disabled components: the terraform state/output functions are added
// (as bare names matching skipFunc), the base skip is preserved, and the base slice is not mutated.
func TestDisabledComponentTerraformSkip(t *testing.T) {
	t.Parallel()

	base := []string{"existing.func"}
	got := disabledComponentTerraformSkip(base)

	assert.Contains(t, got, "terraform.state", "must add the terraform.state skip (bare name)")
	assert.Contains(t, got, "terraform.output", "must add the terraform.output skip (bare name)")
	assert.Contains(t, got, "existing.func", "must preserve the base skip entries")
	assert.Equal(t, []string{"existing.func"}, base, "must not mutate the caller's base skip slice")

	gotNil := disabledComponentTerraformSkip(nil)
	assert.Contains(t, gotNil, "terraform.state")
	assert.Contains(t, gotNil, "terraform.output")
}

// TestEnclosingComponentDisabled verifies the gate used by atmos.Component: it reports disabled only
// when the enclosing component's metadata.enabled is explicitly false. nil info, a missing
// ComponentSection/metadata, or a vars.enabled flag must all be treated as enabled (no skip).
func TestEnclosingComponentDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		info *schema.ConfigAndStacksInfo
		want bool
	}{
		{"nil_info", nil, false},
		{"no_component_section", &schema.ConfigAndStacksInfo{}, false},
		{"empty_component_section", &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}, false},
		{
			"metadata_without_enabled",
			&schema.ConfigAndStacksInfo{ComponentSection: map[string]any{cfg.MetadataSectionName: map[string]any{}}},
			false,
		},
		{
			"metadata_enabled_true",
			&schema.ConfigAndStacksInfo{ComponentSection: map[string]any{cfg.MetadataSectionName: map[string]any{"enabled": true}}},
			false,
		},
		{
			"metadata_enabled_false",
			&schema.ConfigAndStacksInfo{ComponentSection: map[string]any{cfg.MetadataSectionName: map[string]any{"enabled": false}}},
			true,
		},
		{
			// vars.enabled must NOT gate atmos.Component; only metadata.enabled does.
			"vars_enabled_false_metadata_absent",
			&schema.ConfigAndStacksInfo{ComponentSection: map[string]any{cfg.VarsSectionName: map[string]any{"enabled": false}}},
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, enclosingComponentDisabled(tc.info))
		})
	}
}

// TestComponentFunc_DisabledEnclosing_ReturnsEmptySections verifies that atmos.Component invoked from
// a disabled enclosing component returns structurally-valid empty sections (including an empty
// outputs map) without describing the target or reading state — so no Executor needs to be set.
func TestComponentFunc_DisabledEnclosing_ReturnsEmptySections(t *testing.T) {
	t.Parallel()

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "enclosing",
		ComponentSection: map[string]any{
			cfg.MetadataSectionName: map[string]any{"enabled": false},
		},
	}

	got, err := componentFunc(&schema.AtmosConfiguration{}, info, "target", "some-stack")
	require.NoError(t, err)

	sections, ok := got.(map[string]any)
	require.True(t, ok, "result must be a sections map")

	outputs, ok := sections[cfg.OutputsSectionName].(map[string]any)
	require.True(t, ok, "result must include an outputs section")
	assert.Empty(t, outputs, "outputs must be empty for a disabled enclosing component")

	vars, ok := sections[cfg.VarsSectionName].(map[string]any)
	require.True(t, ok, "result must include a vars section so .vars.x stays nil-safe")
	assert.Empty(t, vars)
}

// countingStateGetter is a TerraformStateGetter test double that records GetState invocations.
type countingStateGetter struct {
	calls atomic.Int64
	ret   any
}

func (c *countingStateGetter) GetState(
	_ *schema.AtmosConfiguration,
	_ string,
	_ string,
	_ string,
	_ string,
	_ bool,
	_ *schema.AuthContext,
	_ any,
) (any, error) {
	c.calls.Add(1)
	return c.ret, nil
}

// TestProcessComponentEntry_DisabledComponentSkipsTerraformState verifies the end-to-end gate: when a
// component is disabled via metadata.enabled, processComponentEntry does not resolve its
// !terraform.state (GetState is never called); enabled components — and components disabled only via
// vars.enabled — still resolve it.
func TestProcessComponentEntry_DisabledComponentSkipsTerraformState(t *testing.T) {
	tests := []struct {
		name           string
		metadata       map[string]any
		varsEnabled    *bool
		expectResolved bool
	}{
		{"metadata_enabled_false_skips", map[string]any{"enabled": false}, nil, false},
		{"metadata_enabled_true_resolves", map[string]any{"enabled": true}, nil, true},
		{"no_metadata_enabled_resolves", map[string]any{}, nil, true},
		{"vars_enabled_false_still_resolves", map[string]any{}, boolPtr(false), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spy := &countingStateGetter{ret: "resolved-value"}
			orig := stateGetter
			stateGetter = spy
			t.Cleanup(func() { stateGetter = orig })
			ClearResolutionContext()
			t.Cleanup(ClearResolutionContext)

			vars := map[string]any{"x": "!terraform.state some/component someoutput"}
			if tc.varsEnabled != nil {
				vars["enabled"] = *tc.varsEnabled
			}
			componentSection := map[string]any{
				cfg.VarsSectionName:     vars,
				cfg.MetadataSectionName: tc.metadata,
			}
			allTypeComponents := map[string]any{"test-component": componentSection}

			p := &describeStacksProcessor{
				atmosConfig:          &schema.AtmosConfiguration{},
				processYamlFunctions: true,
				finalStacksMap:       make(map[string]any),
			}

			err := p.processComponentEntry(
				"test-stack", "", cfg.HelmfileSectionName,
				"test-component", componentSection, allTypeComponents,
				processComponentTypeOpts{},
			)
			require.NoError(t, err)

			if tc.expectResolved {
				require.Positive(t, spy.calls.Load(), "enabled component must resolve !terraform.state")
			} else {
				require.Zero(t, spy.calls.Load(), "disabled component must not resolve !terraform.state")
			}
		})
	}
}
