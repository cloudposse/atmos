package schema

// DescribeWorkflowsItem represents a workflow item in the describe workflows output.
type DescribeWorkflowsItem struct {
	File     string `yaml:"file" json:"file" mapstructure:"file"`
	Workflow string `yaml:"workflow" json:"workflow" mapstructure:"workflow"`
}

// ViewportConfig configures viewport display settings.
type ViewportConfig struct {
	Height int `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"` // Lines.
	Width  int `yaml:"width,omitempty" json:"width,omitempty" mapstructure:"width"`    // Columns.
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	// Existing fields.
	Name             string       `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Command          string       `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
	Stack            string       `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	Type             string       `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
	WorkingDirectory string       `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	Retry            *RetryConfig `yaml:"retry,omitempty" json:"retry,omitempty" mapstructure:"retry"`
	Identity         string       `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`

	// Interactive step fields.
	Prompt      string   `yaml:"prompt,omitempty" json:"prompt,omitempty" mapstructure:"prompt"`                // Prompt text for interactive types.
	Options     []string `yaml:"options,omitempty" json:"options,omitempty" mapstructure:"options"`             // Options for choose/filter.
	Default     string   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`             // Default value.
	Placeholder string   `yaml:"placeholder,omitempty" json:"placeholder,omitempty" mapstructure:"placeholder"` // Input placeholder.
	Password    bool     `yaml:"password,omitempty" json:"password,omitempty" mapstructure:"password"`          // Mask input.
	Multiple    bool     `yaml:"multiple,omitempty" json:"multiple,omitempty" mapstructure:"multiple"`          // Allow multiple selection.
	Limit       int      `yaml:"limit,omitempty" json:"limit,omitempty" mapstructure:"limit"`                   // Selection limit.

	// Output/UI step fields.
	Content string           `yaml:"content,omitempty" json:"content,omitempty" mapstructure:"content"` // Content for output types (supports templates).
	Title   string           `yaml:"title,omitempty" json:"title,omitempty" mapstructure:"title"`       // Title for spin/pager.
	Data    []map[string]any `yaml:"data,omitempty" json:"data,omitempty" mapstructure:"data"`          // Data for table type.
	Columns []string         `yaml:"columns,omitempty" json:"columns,omitempty" mapstructure:"columns"` // Columns for table.

	// File picker fields.
	Path       string   `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`                   // Starting path for file picker.
	Extensions []string `yaml:"extensions,omitempty" json:"extensions,omitempty" mapstructure:"extensions"` // File extensions filter.

	// Display configuration.
	Output   string          `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`       // Output mode: viewport, raw, log, none.
	Height   int             `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"`       // Height for write type (editor lines).
	Viewport *ViewportConfig `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"` // Viewport settings for output mode.
	Timeout  string          `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`    // Timeout duration.

	// Environment variables (supports templates).
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
}

// WorkflowDefinition represents a complete workflow with steps.
type WorkflowDefinition struct {
	Description      string `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	WorkingDirectory string `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	// Dependencies lists external tools required for this workflow to execute successfully.
	Dependencies *Dependencies  `yaml:"dependencies,omitempty" json:"dependencies,omitempty" mapstructure:"dependencies"`
	Steps        []WorkflowStep `yaml:"steps" json:"steps" mapstructure:"steps"`
	Stack        string         `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`

	// Output mode fields.
	Output   string          `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`       // Default output mode for steps.
	Viewport *ViewportConfig `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"` // Default viewport settings.
}

type WorkflowConfig map[string]WorkflowDefinition

type WorkflowManifest struct {
	Name        string         `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Workflows   WorkflowConfig `yaml:"workflows" json:"workflows" mapstructure:"workflows"`
}
