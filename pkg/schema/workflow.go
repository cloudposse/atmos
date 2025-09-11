package schema

type DescribeWorkflowsItem struct {
	File     string `yaml:"file" json:"file" mapstructure:"file"`
	Workflow string `yaml:"workflow" json:"workflow" mapstructure:"workflow"`
}
type WorkflowStep struct {
	Name    string       `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Command string       `yaml:"command" json:"command" mapstructure:"command"`
	Stack   string       `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	Type    string       `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
	Retry   *RetryConfig `yaml:"retry,omitempty" json:"retry,omitempty" mapstructure:"retry"`
}

type WorkflowDefinition struct {
	Description string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Steps       []WorkflowStep `yaml:"steps" json:"steps" mapstructure:"steps"`
	Stack       string         `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
}

type WorkflowConfig map[string]WorkflowDefinition

type WorkflowManifest struct {
	Name        string         `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Workflows   WorkflowConfig `yaml:"workflows" json:"workflows" mapstructure:"workflows"`
}
