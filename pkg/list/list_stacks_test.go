package list

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestListStacks(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil,
		nil, false, true, true, false, nil, nil)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, "")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.NotEmpty(t, dependentsYaml)
}

func TestListStacksWithComponent(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	assert.Nil(t, err)

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil,
		nil, false, true, true, false, nil, nil)
	assert.Nil(t, err)

	output, err := FilterAndListStacks(stacksMap, "eks-blue/cluster")
	assert.Nil(t, err)
	dependentsYaml, err := u.ConvertToYAML(output)
	assert.Nil(t, err)

	// Verify the output structure
	assert.NotEmpty(t, dependentsYaml)
	// Verify that only stacks with the specified component are included
	assert.Contains(t, dependentsYaml, "tenant1-uw1-test-1")
	assert.Contains(t, dependentsYaml, "tenant1-uw2-test-1")
}

func TestFilterAndListStacks_NoMatchingComponent(t *testing.T) {
	stacksMap := map[string]any{
		"stack1": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"existing": map[string]any{},
				},
			},
		},
	}

	output, err := FilterAndListStacks(stacksMap, "missing")
	assert.NoError(t, err)
	assert.Nil(t, output)
}

// TestFilterAndListStacks_NonTerraformComponentType is a regression test: this
// function is shared by non-terraform completion/prompt callers (e.g.
// cmd/container/completions.go, cmd/ansible/completions.go, cmd/cmd_utils.go's
// describe-stack completion), so it must find a component defined under any
// component type section, not only "terraform" -- otherwise it always returns
// zero matches for every other component type.
func TestFilterAndListStacks_NonTerraformComponentType(t *testing.T) {
	stacksMap := map[string]any{
		"prod": map[string]any{
			"components": map[string]any{
				"aws/cloudformation": map[string]any{
					"vpc": map[string]any{"stack_name": "prod-vpc"},
				},
			},
		},
		"dev": map[string]any{
			"components": map[string]any{
				"container": map[string]any{
					"api": map[string]any{},
				},
			},
		},
		"unrelated": map[string]any{
			"components": map[string]any{
				"aws/cloudformation": map[string]any{
					"rds": map[string]any{},
				},
			},
		},
	}

	output, err := FilterAndListStacks(stacksMap, "vpc")
	assert.NoError(t, err)
	assert.Equal(t, []string{"prod"}, output)

	output, err = FilterAndListStacks(stacksMap, "api")
	assert.NoError(t, err)
	assert.Equal(t, []string{"dev"}, output)
}

// TestStackContainsAnyTypeComponent covers the malformed-data guard clauses in
// stackContainsAnyTypeComponent: a stack value that isn't a map, a stack map
// missing (or with a malformed) "components" section, and a component-type
// section whose value isn't a map (which must be skipped, not treated as a
// match or an error), alongside the successful match path.
func TestStackContainsAnyTypeComponent(t *testing.T) {
	tests := []struct {
		name      string
		stackData any
		component string
		want      bool
	}{
		{
			name:      "stack data is not a map",
			stackData: "not-a-map",
			component: "vpc",
			want:      false,
		},
		{
			name: "components key missing",
			stackData: map[string]any{
				"vars": map[string]any{"foo": "bar"},
			},
			component: "vpc",
			want:      false,
		},
		{
			name: "components value is not a map",
			stackData: map[string]any{
				"components": "not-a-map",
			},
			component: "vpc",
			want:      false,
		},
		{
			name: "type section value is not a map is skipped, later valid section still matches",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": "not-a-map",
					"container": map[string]any{
						"vpc": map[string]any{},
					},
				},
			},
			component: "vpc",
			want:      true,
		},
		{
			name: "type section value is not a map and no other section matches",
			stackData: map[string]any{
				"components": map[string]any{
					"terraform": "not-a-map",
				},
			},
			component: "vpc",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stackContainsAnyTypeComponent(tt.stackData, tt.component)
			assert.Equal(t, tt.want, got)
		})
	}
}
