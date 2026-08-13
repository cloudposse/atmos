package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestResolveDeferredYamlFunctions_NoDeferredContexts verifies Stage 3 is a safe no-op when the
// caller never populated DeferredMergeContexts (e.g. a non-FindStacksMap-sourced stacksMap, or a
// component whose merge produced no deferred YAML functions at all) — it must not panic or error,
// and must leave ComponentSection completely untouched.
func TestResolveDeferredYamlFunctions_NoDeferredContexts(t *testing.T) {
	tests := []struct {
		name                  string
		deferredMergeContexts any
	}{
		{name: "nil (zero value)", deferredMergeContexts: nil},
		{name: "wrong type", deferredMergeContexts: "not-a-ComponentDeferredContexts"},
		{name: "empty ComponentDeferredContexts map", deferredMergeContexts: ComponentDeferredContexts{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := templatingEnabledConfig()
			info := &schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{
					cfg.VarsSectionName: map[string]any{"greeting": "unchanged"},
				},
				DeferredMergeContexts: tt.deferredMergeContexts,
			}
			settingsStruct := &schema.Settings{}

			err := resolveDeferredYamlFunctions(atmosConfig, info, settingsStruct, nil, nil)

			require.NoError(t, err)
			vars, ok := info.ComponentSection[cfg.VarsSectionName].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "unchanged", vars["greeting"], "ComponentSection must be untouched when there is nothing to resolve")
		})
	}
}

// TestResolveDeferredYamlFunctions_SkipsSectionMissingFromComponentSection verifies that a
// section present in the deferred-contexts bundle but absent (or not a map) in
// configAndStacksInfo.ComponentSection is safely skipped rather than causing a panic or an
// error — while a sibling section that IS present still gets resolved. Custom component types
// pass through mergeComponentConfigurations for some but not all sections, so this mismatch is a
// real, reachable case, not just a defensive guard.
func TestResolveDeferredYamlFunctions_SkipsSectionMissingFromComponentSection(t *testing.T) {
	atmosConfig := templatingEnabledConfig()

	varsDctx := m.NewDeferredMergeContext()
	varsDctx.AddDeferred([]string{"greeting"}, "!template 'hello'")

	settingsDctx := m.NewDeferredMergeContext()
	settingsDctx.AddDeferred([]string{"missing"}, "!template 'unused'")

	info := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			// The vars section survived Stage 2 with the deferred placeholder (nil).
			cfg.VarsSectionName: map[string]any{"greeting": nil},
			// The settings section is deliberately absent: settingsDctx must be skipped, not error.
		},
		DeferredMergeContexts: ComponentDeferredContexts{
			cfg.VarsSectionName:     varsDctx,
			cfg.SettingsSectionName: settingsDctx,
		},
	}
	settingsStruct := &schema.Settings{}

	err := resolveDeferredYamlFunctions(atmosConfig, info, settingsStruct, map[string]any{}, nil)
	require.NoError(t, err)

	vars, ok := info.ComponentSection[cfg.VarsSectionName].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "'hello'", vars["greeting"], "the present vars section must still be resolved even though settings was skipped")
	_, hasSettings := info.ComponentSection[cfg.SettingsSectionName]
	assert.False(t, hasSettings, "the missing settings section must not be fabricated")
}

// TestProcessStacks_DeferredYamlFunctionResolutionError_PropagatesFromProcessStacks is an
// end-to-end regression test proving that Stage 3 (resolveDeferredYamlFunctions, called from
// processStacks) surfaces a real resolution failure as an error, rather than silently swallowing
// it. It uses `test-yaml-to-concrete`, an existing fixture component whose base layer sets
// `vars.template_config: !template '{{ toJson .settings.base_config }}'` — a deferred function
// with a `{{ }}` expression. With processTemplates=false, ProcessStacks never builds a component
// template context, so when processYamlFunctions=true asks Stage 3 to resolve that deferred value,
// it must fail loudly (errUtils.ErrDeferredTemplateContextMissing) instead of leaving a nil/unset
// placeholder in the final vars.
func TestProcessStacks_DeferredYamlFunctionResolutionError_PropagatesFromProcessStacks(t *testing.T) {
	t.Chdir(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "atmos-yaml-functions-merge"))
	t.Setenv("ATMOS_STAGE", "test")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "test-yaml-to-concrete",
		Stack:            "test",
		ComponentType:    cfg.TerraformComponentType,
	}

	// checkStack=true, processTemplates=false, processYamlFunctions=true.
	_, err = ProcessStacks(&atmosConfig, info, true, false, true, nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDeferredTemplateContextMissing)
}
