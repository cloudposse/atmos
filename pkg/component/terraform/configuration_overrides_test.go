package terraform

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestOverlayStackVarsDeepMergesStackOverProviderInputs(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	inputs := map[string]any{"name": "from-hcl", "tags": map[string]any{"team": "hcl", "env": "dev"}, "keep": 1, "dropped": "hcl"}
	stack := map[string]any{"name": "from-stack", "tags": map[string]any{"team": "stack", "extra": "x"}, "added": true, "dropped": nil}

	merged, err := overlayStackVars(config, inputs, stack)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"name":    "from-stack",
		"tags":    map[string]any{"team": "stack", "env": "dev", "extra": "x"},
		"keep":    1,
		"added":   true,
		"dropped": nil,
	}, merged, "maps deep-merge, scalars are replaced, and null replaces the provider input")

	// Result to source: mutating the result leaves both layers alone.
	merged["tags"].(map[string]any)["team"] = "mutated"
	assert.Equal(t, "hcl", inputs["tags"].(map[string]any)["team"])
	assert.Equal(t, "stack", stack["tags"].(map[string]any)["team"])
	// Source to result: mutating a source after the merge leaves the result alone.
	merged, err = overlayStackVars(config, inputs, stack)
	require.NoError(t, err)
	inputs["tags"].(map[string]any)["env"] = "mutated"
	stack["tags"].(map[string]any)["extra"] = "mutated"
	assert.Equal(t, map[string]any{"team": "stack", "env": "dev", "extra": "x"}, merged["tags"])
}

func TestOverlayStackVarsHonorsTheListMergeStrategy(t *testing.T) {
	inputs := map[string]any{"zones": []any{"a", "b"}}
	stack := map[string]any{"zones": []any{"c"}}

	replaced, err := overlayStackVars(&schema.AtmosConfiguration{}, inputs, stack)
	require.NoError(t, err)
	assert.Equal(t, []any{"c"}, replaced["zones"])

	config := &schema.AtmosConfiguration{}
	config.Settings.ListMergeStrategy = "append"
	appended, err := overlayStackVars(config, inputs, stack)
	require.NoError(t, err)
	assert.Equal(t, []any{"a", "b", "c"}, appended["zones"])

	config.Settings.ListMergeStrategy = "bogus"
	_, err = overlayStackVars(config, inputs, stack)
	require.Error(t, err)
}

func TestOverlayStackVarsWithoutStackVarsKeepsProviderInputs(t *testing.T) {
	inputs := map[string]any{"name": "from-hcl"}
	for _, stack := range []map[string]any{nil, {}} {
		merged, err := overlayStackVars(&schema.AtmosConfiguration{}, inputs, stack)
		require.NoError(t, err)
		assert.Equal(t, inputs, merged)
	}
}

func TestWithoutStackEnvironmentLetsTheStackWin(t *testing.T) {
	execution := &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{
		{Commands: []string{"plan"}, Environment: map[string]string{"TF_LOG": "TRACE", "FROM_UNIT": "1"}},
		{Commands: []string{"apply"}},
	}}

	result := withoutStackEnvironment(execution, map[string]any{"TF_LOG": "ERROR"})

	assert.Equal(t, map[string]string{"FROM_UNIT": "1"}, result.ExtraArguments[0].Environment)
	assert.Empty(t, result.ExtraArguments[1].Environment)
	assert.Equal(t, []string{"plan"}, result.ExtraArguments[0].Commands)
	assert.Equal(t, map[string]string{"TF_LOG": "TRACE", "FROM_UNIT": "1"}, execution.ExtraArguments[0].Environment, "the provider's data is not modified")

	assert.Same(t, execution, withoutStackEnvironment(execution, nil), "no stack env, nothing to drop")
	assert.Nil(t, withoutStackEnvironment(nil, map[string]any{"A": "b"}))
}

func TestCheckStackOverrides(t *testing.T) {
	owned := func(section map[string]any, metadata map[string]any) *schema.ConfigAndStacksInfo {
		return &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev", ComponentSection: section, ComponentMetadataSection: metadata}
	}
	for _, test := range []struct {
		name    string
		info    *schema.ConfigAndStacksInfo
		wantErr string
	}{
		{name: "placeholder only", info: owned(map[string]any{"backend_type": "local", "backend": map[string]any{"local": map[string]any{}}}, nil)},
		{name: "no backend at all", info: owned(map[string]any{}, nil)},
		{name: "global backend is shielded by the placeholder", info: owned(map[string]any{"backend_type": "local", "backend": map[string]any{"local": map[string]any{}, "s3": map[string]any{"bucket": "b"}}}, nil)},
		{name: "backend type", info: owned(map[string]any{"backend_type": "s3"}, nil), wantErr: "backend_type: s3"},
		{name: "backend options", info: owned(map[string]any{"backend_type": "local", "backend": map[string]any{"local": map[string]any{"path": "x"}}}, nil), wantErr: "backend.local"},
		{name: "module", info: owned(map[string]any{}, map[string]any{"component": "other"}), wantErr: "metadata.component: other"},
		{name: "metadata that is not owned", info: owned(map[string]any{}, map[string]any{"enabled": true, "terraform_workspace": "blue", "inherits": []any{"base"}})},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := checkStackOverrides(test.info)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
			require.ErrorContains(t, err, test.wantErr)
			require.ErrorContains(t, err, `"vpc"`)
		})
	}
}

func overrideInfo(t *testing.T) *schema.ConfigAndStacksInfo {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	return &schema.ConfigAndStacksInfo{
		Command: binary, SubCommand: "plan", Stack: "dev", ComponentFromArg: "vpc",
		ComponentSection:         map[string]any{},
		ComponentSettingsSection: map[string]any{"configuration": true},
		ComponentVarsSection:     map[string]any{"name": "from-stack", "tags": map[string]any{"team": "stack"}},
		ComponentEnvSection:      map[string]any{"TF_LOG": "ERROR"},
	}
}

func TestPrepareConfigurationAppliesStackOverrides(t *testing.T) {
	runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	info := overrideInfo(t)
	prepared := &schema.PreparedComponentConfiguration{
		Engine: "terraform", Source: "./module", BackendType: "local", Backend: map[string]any{"path": "state"}, Workspace: "default",
		Inputs: map[string]any{"name": "from-hcl", "tags": map[string]any{"team": "hcl", "env": "dev"}, "region": "us-east-1"},
		Execution: &schema.ComponentExecution{ExtraArguments: []schema.ComponentExtraArguments{
			{Commands: []string{"plan"}, Environment: map[string]string{"TF_LOG": "TRACE", "UNIT": "1"}},
		}},
	}
	runtime.EXPECT().Prepare(gomock.Any(), info).Return(prepared, nil)

	_, err := prepareConfiguration(t.Context(), config, info)

	require.NoError(t, err)
	want := map[string]any{"name": "from-stack", "tags": map[string]any{"team": "stack", "env": "dev"}, "region": "us-east-1"}
	assert.Equal(t, want, info.ComponentVarsSection)
	assert.Equal(t, want, info.ComponentSection["vars"], "the generated varfile reads the merged component section")
	assert.Equal(t, map[string]string{"UNIT": "1"}, info.ComponentExecution.ExtraArguments[0].Environment)
}

func TestPrepareConfigurationRefusesProviderOwnedOverridesBeforeEvaluating(t *testing.T) {
	// The mock has no expectation: evaluating the unit would fail the test.
	runtime := NewMockComponentConfigurationRuntime(gomock.NewController(t))
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	info := overrideInfo(t)
	info.ComponentSection["backend_type"] = "s3"

	_, err := prepareConfiguration(t.Context(), config, info)

	require.ErrorIs(t, err, errUtils.ErrConfigurationOverrideUnsupported)
}

func TestApplyStackOverridesKeepsSavedPlanInputs(t *testing.T) {
	info := overrideInfo(t)
	info.UseTerraformPlan = true
	prepared := &schema.PreparedComponentConfiguration{Inputs: map[string]any{}}

	require.NoError(t, applyStackOverrides(&schema.AtmosConfiguration{}, info, prepared))

	assert.Empty(t, prepared.Inputs, "a reviewed plan embodies its inputs; current stack vars must not leak into the apply")
}

func TestApplyStackOverridesReportsMergeFailures(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Settings.ListMergeStrategy = "bogus"
	err := applyStackOverrides(config, overrideInfo(t), &schema.PreparedComponentConfiguration{Inputs: map[string]any{"a": 1}})
	require.Error(t, err)
}
