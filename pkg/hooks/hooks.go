package hooks

import (
	"fmt"

	"gopkg.in/yaml.v2"
)

type HookType string

const (
	AfterTerraformApply  HookType = "after.terraform.apply"
	BeforeTerraformApply HookType = "before.terraform.apply"
	AfterTerraformPlan   HookType = "after.terraform.plan"
	BeforeTerraformPlan  HookType = "before.terraform.plan"
)

type Command string

const (
	Store Command = "store"
)

// Hook defines the structure of a hook
type Hook struct {
	Events  []string `yaml:"events"`
	Command string   `yaml:"command"`

	// Dynamic command-specific properties

	// store command
	Name    string            `yaml:"name,omitempty"`    // for store command
	Outputs map[string]string `yaml:"outputs,omitempty"` // for store command
}

type Hooks map[string]Hook

func (h Hooks) ConvertToHooks(input map[string]any) (Hooks, error) {
	yamlData, err := yaml.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	var hooks Hooks
	err = yaml.Unmarshal(yamlData, &hooks)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to Hooks: %w", err)
	}

	return hooks, nil
}
