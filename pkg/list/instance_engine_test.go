package list

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	e "github.com/cloudposse/atmos/internal/exec"
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
