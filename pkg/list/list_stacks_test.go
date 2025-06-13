package list

import (
	"strings"
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

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, true, true, false, nil)
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

	stacksMap, err := e.ExecuteDescribeStacks(atmosConfig, "", nil, nil,
		nil, false, true, true, false, nil)
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

// TestListStacksWithColumns tests the custom columns functionality for stacks
func TestListStacksWithColumns(t *testing.T) {
	stacksMap := map[string]any{
		"tenant1-ue2-dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
					"eks": map[string]any{},
				},
			},
		},
		"tenant1-ue2-staging": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
				},
			},
		},
		"tenant1-ue2-prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{},
					"eks": map[string]any{},
					"rds": map[string]any{},
				},
			},
		},
	}

	atmosConfig := schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
	}

	tests := []struct {
		name        string
		component   string
		listConfig  schema.ListConfig
		format      string
		delimiter   string
		validate    func(t *testing.T, output string)
		description string
	}{
		{
			name:      "default columns with JSON format",
			component: "",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Stack", Value: "{{ .stack_name }}"},
					{Name: "Path", Value: "{{ .stack_path }}"},
				},
			},
			format:    "json",
			delimiter: "\t",
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "\"Stack\":")
				assert.Contains(t, output, "\"Path\":")
				assert.Contains(t, output, "tenant1-ue2-dev")
				assert.Contains(t, output, "tenant1-ue2-staging")
				assert.Contains(t, output, "tenant1-ue2-prod")
				assert.Contains(t, output, "stacks/")
			},
			description: "should output JSON with default columns",
		},
		{
			name:      "custom columns with templates",
			component: "vpc",
			listConfig: schema.ListConfig{
				Columns: []schema.ListColumnConfig{
					{Name: "Name", Value: "{{ .stack_name }}"},
					{Name: "Environment", Value: "{{ .stack_name }}"},
					{Name: "Status", Value: "Active"},
				},
			},
			format:    "csv",
			delimiter: ",",
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), u.GetLineEnding())
				assert.GreaterOrEqual(t, len(lines), 2)
				assert.Equal(t, "Name,Environment,Status", lines[0])
				for i := 1; i < len(lines); i++ {
					fields := strings.Split(lines[i], ",")
					if len(fields) == 3 {
						assert.Equal(t, "Active", fields[2])
						assert.Contains(t, fields[0], "tenant1-ue2")
					}
				}
			},
			description: "should handle custom columns with templates and static values",
		},
		{
			name:       "default simple list format",
			component:  "eks",
			listConfig: schema.ListConfig{},
			format:     "",
			delimiter:  "\t",
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), u.GetLineEnding())
				assert.Equal(t, 2, len(lines))
				assert.Contains(t, output, "tenant1-ue2-dev")
				assert.Contains(t, output, "tenant1-ue2-prod")
				assert.NotContains(t, output, "tenant1-ue2-staging")
			},
			description: "should return simple list format when no format specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := FilterAndListStacksWithColumns(stacksMap, tt.component, tt.listConfig, tt.format, tt.delimiter, atmosConfig)
			assert.NoError(t, err)
			tt.validate(t, output)
		})
	}
}
