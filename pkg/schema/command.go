package schema

// Custom CLI commands.

// Command defines a custom CLI command.
type Command struct {
	Name             string `yaml:"name" json:"name" mapstructure:"name"`
	Description      string `yaml:"description" json:"description" mapstructure:"description"`
	WorkingDirectory string `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	// Dependencies specifies external tool dependencies that must be installed before running this command.
	Dependencies    *Dependencies          `yaml:"dependencies,omitempty" json:"dependencies,omitempty" mapstructure:"dependencies"`
	Env             []CommandEnv           `yaml:"env" json:"env" mapstructure:"env"`
	Arguments       []CommandArgument      `yaml:"arguments" json:"arguments" mapstructure:"arguments"`
	Flags           []CommandFlag          `yaml:"flags" json:"flags" mapstructure:"flags"`
	Component       *CommandComponent      `yaml:"component,omitempty" json:"component,omitempty" mapstructure:"component"`
	ComponentConfig CommandComponentConfig `yaml:"component_config" json:"component_config" mapstructure:"component_config"`
	// Steps supports both simple string syntax and structured syntax.
	// Simple: ["echo hello", "echo world"]
	// Structured: [{name: step1, command: echo hello, timeout: 30s}]
	// Mixed: Both formats can be used in the same list.
	Steps    Tasks     `yaml:"steps" json:"steps" mapstructure:"steps"`
	Commands []Command `yaml:"commands" json:"commands" mapstructure:"commands"`
	Verbose  bool      `yaml:"verbose" json:"verbose" mapstructure:"verbose"`
	Identity string    `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}

// CommandArgument defines a positional argument for a custom command.
type CommandArgument struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
	Default     string `yaml:"default" json:"default" mapstructure:"default"`
	// Type specifies the semantic type of this argument: "component" or "stack".
	// When set, the argument value is used to resolve component configuration.
	Type string `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
}

// CommandFlag defines a flag for a custom command.
type CommandFlag struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Shorthand   string `yaml:"shorthand" json:"shorthand" mapstructure:"shorthand"`
	Type        string `yaml:"type" json:"type" mapstructure:"type"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
	Usage       string `yaml:"usage" json:"usage" mapstructure:"usage"`
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
	Default     any    `yaml:"default" json:"default" mapstructure:"default"`
	// SemanticType specifies the semantic type of this flag: "component" or "stack".
	// When set, the flag value is used to resolve component configuration.
	SemanticType string `yaml:"semantic_type,omitempty" json:"semantic_type,omitempty" mapstructure:"semantic_type"`
}

// CommandEnv defines an environment variable for a custom command.
type CommandEnv struct {
	Key          string `yaml:"key" json:"key" mapstructure:"key"`
	Value        string `yaml:"value" json:"value" mapstructure:"value"`
	ValueCommand string `yaml:"valueCommand" json:"valueCommand" mapstructure:"valueCommand"`
}

// CommandComponent defines a custom component type for a command.
// When specified, the command can access component configuration via {{ .Component.* }} templates.
type CommandComponent struct {
	// Type is the component type name (e.g., "script", "ansible", "manifest").
	Type string `yaml:"type" json:"type" mapstructure:"type"`
	// BasePath is the base directory for components of this type.
	// Defaults to "components/<type>" if not specified.
	BasePath string `yaml:"base_path,omitempty" json:"base_path,omitempty" mapstructure:"base_path"`
}

// CommandComponentConfig defines component configuration for a custom command (legacy).
type CommandComponentConfig struct {
	Component string `yaml:"component" json:"component" mapstructure:"component"`
	Stack     string `yaml:"stack" json:"stack" mapstructure:"stack"`
}

// CLI command aliases

type CommandAliases map[string]string
