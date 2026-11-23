package list

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInstanceEngineSelection(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Command = "tofu"
	for _, tc := range []struct {
		name, kind string
		component  map[string]any
		want       string
	}{
		{"configured tofu", "terraform", nil, "opentofu"},
		{"unit terraform overrides default", "terraform", map[string]any{"command": "terraform"}, "terraform"},
		{"versioned tofu binary", "terraform", map[string]any{"command": "tofu-1.8"}, "opentofu"},
		{"mixed-case OpenTofu binary", "terraform", map[string]any{"command": "OpenTofu.EXE"}, "opentofu"},
		{"absolute Windows binary", "terraform", map[string]any{"command": filepath.Join(t.TempDir(), "tofu.exe")}, "opentofu"},
		{"native HCL remains tofu", "terraform", map[string]any{"settings": map[string]any{"configuration": map[string]any{"provider": "terragrunt", "engine": "opentofu"}}}, "opentofu"},
		{"external engine metadata", "terraform", map[string]any{"settings": map[string]any{"configuration": map[string]any{"engine": "terragrunt"}}}, "terragrunt"},
		{"other component family", "helmfile", nil, "helmfile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, instanceEngine(config, tc.kind, tc.component))
		})
	}
	config.Components.Terraform.Command = ""
	require.Equal(t, "terraform", instanceEngine(config, "terraform", nil))
}

func TestInstanceDefaultColumnsRespectEditionsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		pin   string
		count int
	}{{"", 4}, {"2026-10-28", 2}, {"2026-10-29", 4}} {
		config := &schema.AtmosConfiguration{Edition: tc.pin}
		columns, err := getInstanceColumns(config, nil)
		require.NoError(t, err)
		require.Len(t, columns, tc.count)
		override, err := getInstanceColumns(config, []string{"Name={{ .component }}"})
		require.NoError(t, err)
		require.Len(t, override, 1)
	}
	_, err := getInstanceColumns(&schema.AtmosConfiguration{Edition: "bad"}, nil)
	require.Error(t, err)
}

func TestPopulateInstanceEnginesDoesNotEvaluateConfiguration(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Command = "tofu"
	instances := []schema.Instance{{Stack: "live", Component: "app", ComponentType: "terraform"}, {Stack: "missing", Component: "other", ComponentType: "helmfile"}}
	stacks := map[string]any{"live": map[string]any{"components": map[string]any{"terraform": map[string]any{"app": map[string]any{"command": "terraform", "vars": map[string]any{"secret": "!terraform.output other live password"}}}}}}
	populateInstanceEngines(config, stacks, instances)
	require.Equal(t, "terraform", instances[0].Engine)
	require.Equal(t, "helmfile", instances[1].Engine)
}

func TestInstanceEngineReachesRenderedColumns(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Command = "tofu"
	stacks := map[string]any{
		"live": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"hcl": map[string]any{
						"settings": map[string]any{
							"configuration": map[string]any{"provider": "terragrunt"},
						},
					},
					"yaml": map[string]any{"command": "terraform"},
				},
			},
		},
	}
	processor := e.NewMockStacksProcessor(gomock.NewController(t))
	processor.EXPECT().
		ExecuteDescribeStacks(config, "", nil, nil, nil, false, true, false, false, nil, nil).
		Return(stacks, nil)

	instances, _, err := processInstancesWithDeps(config, processor, nil, true, false, nil, "", false, nil, nil, nil)
	require.NoError(t, err)
	columns, err := getInstanceColumns(config, nil)
	require.NoError(t, err)
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	require.NoError(t, err)

	// The CLI uses metadata extraction, including for JSON/YAML row output.
	// Testing only extract.Instances misses fields dropped on this path.
	headers, rows, err := selector.Extract(extract.Metadata(instances))
	require.NoError(t, err)
	require.Equal(t, []string{"Component", "Stack", "Type", "Engine"}, headers)
	require.Equal(t, [][]string{
		{"hcl", "live", "terraform", "opentofu"},
		{"yaml", "live", "terraform", "terraform"},
	}, rows)
}

// TestEngineColumnEvaluatesItsInputs verifies that selecting the Engine column evaluates the
// fields the engine is derived from, so a templated command decides it only after rendering,
// while the narrowed evaluation still leaves unrelated fields alone.
func TestEngineColumnEvaluatesItsInputs(t *testing.T) {
	columns := []column.Config{{Name: "Engine", Value: "{{ .engine }}"}}
	sections, ok := column.RequiredSections(columns)
	require.True(t, ok)
	require.Empty(t, sections, "the inputs are narrowed to fields, not whole sections")
	require.ElementsMatch(t, [][]string{{"settings", "configuration", "engine"}, {"command"}}, column.RequiredPaths(columns))

	config := &schema.AtmosConfiguration{}
	paths := resolveInstancesEvaluationPaths(config, columns, &InstancesCommandOptions{})
	require.Contains(t, paths, []string{"command"})
	require.Contains(t, paths, []string{"settings", "configuration", "engine"})
	require.NotContains(t, paths, []string{"settings"}, "settings are not evaluated wholesale for the engine")

	component := map[string]any{
		"command":  "{{ .settings.tool }}",
		"settings": map[string]any{"tool": "tofu", "configuration": map[string]any{"engine": ""}},
		"vars":     map[string]any{"secret": "!terraform.output other live password"},
	}
	expanded := deferred.ExpandEvaluationPaths(component, paths)
	selected, _ := deferred.SplitEvaluationFields(component, expanded)
	require.Contains(t, selected, "command", "the templated command must be evaluated before the engine is derived")
	require.Contains(t, expanded, []string{"settings", "tool"}, "the command's own template inputs are evaluated too")
	require.NotContains(t, selected, "vars", "unrelated sections stay unevaluated")

	// Other derived columns still read nothing.
	require.Empty(t, column.RequiredPaths([]column.Config{{Name: "Component", Value: "{{ .component }}"}}))
}
