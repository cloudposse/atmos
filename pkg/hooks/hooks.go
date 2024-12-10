package hooks

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
	Name   string                 `yaml:"name,omitempty"`   // for store command
	Values map[string]interface{} `yaml:"values,omitempty"` // for store command
}

type Hooks struct {
	Hooks map[string]Hook `yaml:"hooks"`
}
