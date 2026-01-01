package schema

type DescribeWorkflowsItem struct {
	File     string `yaml:"file" json:"file" mapstructure:"file"`
	Workflow string `yaml:"workflow" json:"workflow" mapstructure:"workflow"`
}
type WorkflowStep struct {
	Name             string       `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Command          string       `yaml:"command" json:"command" mapstructure:"command"`
	Stack            string       `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	Type             string       `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
	WorkingDirectory string       `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	Retry            *RetryConfig `yaml:"retry,omitempty" json:"retry,omitempty" mapstructure:"retry"`
	Identity         string       `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}

type WorkflowDefinition struct {
	Description      string `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	WorkingDirectory string `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	// Dependencies lists external tools required for this workflow to execute successfully.
	Dependencies *Dependencies  `yaml:"dependencies,omitempty" json:"dependencies,omitempty" mapstructure:"dependencies"`
	Steps        []WorkflowStep `yaml:"steps" json:"steps" mapstructure:"steps"`
	Stack        string         `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
}

type WorkflowConfig map[string]WorkflowDefinition

type WorkflowManifest struct {
	Name        string         `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Workflows   WorkflowConfig `yaml:"workflows" json:"workflows" mapstructure:"workflows"`
}
