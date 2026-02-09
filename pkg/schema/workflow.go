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

// ShowConfig configures automatic display features for workflows.
// All fields use *bool to enable tri-state logic: nil (inherit), true, false.
// This allows step-level settings to override workflow-level defaults via deep merge.
type ShowConfig struct {
	// Header auto-displays workflow description as styled header before first step.
	Header *bool `yaml:"header,omitempty" json:"header,omitempty" mapstructure:"header"`
	// Flags displays workflow-level flag values under header (e.g., "stack: dev").
	Flags *bool `yaml:"flags,omitempty" json:"flags,omitempty" mapstructure:"flags"`
	// Command shows step command before execution (with $ prefix).
	Command *bool `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
	// Count shows step count prefix (e.g., "[1/3]").
	Count *bool `yaml:"count,omitempty" json:"count,omitempty" mapstructure:"count"`
	// Progress shows progress bar pinned to bottom (Docker-build style, TTY only).
	Progress *bool `yaml:"progress,omitempty" json:"progress,omitempty" mapstructure:"progress"`
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
	Content   string           `yaml:"content,omitempty" json:"content,omitempty" mapstructure:"content"`       // Content for output types (supports templates).
	Title     string           `yaml:"title,omitempty" json:"title,omitempty" mapstructure:"title"`             // Title for spin/pager.
	Data      []map[string]any `yaml:"data,omitempty" json:"data,omitempty" mapstructure:"data"`                // Data for table type.
	Columns   []string         `yaml:"columns,omitempty" json:"columns,omitempty" mapstructure:"columns"`       // Columns for table.
	Separator string           `yaml:"separator,omitempty" json:"separator,omitempty" mapstructure:"separator"` // Separator for join type (default: newline).

	// File picker fields.
	Path       string   `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`                   // Starting path for file picker.
	Extensions []string `yaml:"extensions,omitempty" json:"extensions,omitempty" mapstructure:"extensions"` // File extensions filter.

	// Display configuration.
	Output   string          `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`       // Output mode: viewport, raw, log, none.
	Height   int             `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"`       // Height for write type (editor lines).
	Viewport *ViewportConfig `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"` // Viewport settings for output mode.
	Timeout  string          `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`    // Timeout duration.
	Count    int             `yaml:"count,omitempty" json:"count,omitempty" mapstructure:"count"`          // Count for linebreak type.

	// Style step fields (like gum style).
	Foreground       string `yaml:"foreground,omitempty" json:"foreground,omitempty" mapstructure:"foreground"`                      // Foreground color.
	Background       string `yaml:"background,omitempty" json:"background,omitempty" mapstructure:"background"`                      // Background color.
	Border           string `yaml:"border,omitempty" json:"border,omitempty" mapstructure:"border"`                                  // Border style: none, hidden, normal, rounded, thick, double.
	BorderForeground string `yaml:"border_foreground,omitempty" json:"border_foreground,omitempty" mapstructure:"border_foreground"` // Border foreground color.
	BorderBackground string `yaml:"border_background,omitempty" json:"border_background,omitempty" mapstructure:"border_background"` // Border background color.
	Padding          string `yaml:"padding,omitempty" json:"padding,omitempty" mapstructure:"padding"`                               // Padding: "1" or "1 2" or "1 2 1 2" (top, right, bottom, left).
	Margin           string `yaml:"margin,omitempty" json:"margin,omitempty" mapstructure:"margin"`                                  // Margin: "1" or "1 2" or "1 2 1 2" (top, right, bottom, left).
	Width            int    `yaml:"width,omitempty" json:"width,omitempty" mapstructure:"width"`                                     // Fixed width.
	Align            string `yaml:"align,omitempty" json:"align,omitempty" mapstructure:"align"`                                     // Text alignment: left, center, right.
	Bold             bool   `yaml:"bold,omitempty" json:"bold,omitempty" mapstructure:"bold"`                                        // Bold text.
	Italic           bool   `yaml:"italic,omitempty" json:"italic,omitempty" mapstructure:"italic"`                                  // Italic text.
	Underline        bool   `yaml:"underline,omitempty" json:"underline,omitempty" mapstructure:"underline"`                         // Underline text.
	Strikethrough    bool   `yaml:"strikethrough,omitempty" json:"strikethrough,omitempty" mapstructure:"strikethrough"`             // Strikethrough text.
	Faint            bool   `yaml:"faint,omitempty" json:"faint,omitempty" mapstructure:"faint"`                                     // Faint/dim text.
	Markdown         bool   `yaml:"markdown,omitempty" json:"markdown,omitempty" mapstructure:"markdown"`                            // Render content as markdown.

	// Log step fields.
	Level  string            `yaml:"level,omitempty" json:"level,omitempty" mapstructure:"level"`    // Log level: trace, debug, info, warn, error.
	Fields map[string]string `yaml:"fields,omitempty" json:"fields,omitempty" mapstructure:"fields"` // Structured log fields (key-value pairs).

	// Environment variables (supports templates).
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`

	// Env step type fields.
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty" mapstructure:"vars"` // Variables to set for env step type.

	// Exit step type fields.
	Code int `yaml:"code,omitempty" json:"code,omitempty" mapstructure:"code"` // Exit code for exit step type.

	// Show configuration for this step (overrides workflow-level show settings).
	Show *ShowConfig `yaml:"show,omitempty" json:"show,omitempty" mapstructure:"show"`
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
	Show     *ShowConfig     `yaml:"show,omitempty" json:"show,omitempty" mapstructure:"show"`             // Default show settings for steps.
}

type WorkflowConfig map[string]WorkflowDefinition

type WorkflowManifest struct {
	Name        string         `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Workflows   WorkflowConfig `yaml:"workflows" json:"workflows" mapstructure:"workflows"`
}
