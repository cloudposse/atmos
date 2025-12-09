package schema

// Custom CLI commands

type Command struct {
	Name            string                 `yaml:"name" json:"name" mapstructure:"name"`
	Description     string                 `yaml:"description" json:"description" mapstructure:"description"`
	Env             []CommandEnv           `yaml:"env" json:"env" mapstructure:"env"`
	Arguments       []CommandArgument      `yaml:"arguments" json:"arguments" mapstructure:"arguments"`
	Flags           []CommandFlag          `yaml:"flags" json:"flags" mapstructure:"flags"`
	ComponentConfig CommandComponentConfig `yaml:"component_config" json:"component_config" mapstructure:"component_config"`
	Steps           []string               `yaml:"steps" json:"steps" mapstructure:"steps"`
	Commands        []Command              `yaml:"commands" json:"commands" mapstructure:"commands"`
	Verbose         bool                   `yaml:"verbose" json:"verbose" mapstructure:"verbose"`
	Identity        string                 `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}

type CommandArgument struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
	Default     string `yaml:"default" json:"default" mapstructure:"default"`
}

type CommandFlag struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Shorthand   string `yaml:"shorthand" json:"shorthand" mapstructure:"shorthand"`
	Type        string `yaml:"type" json:"type" mapstructure:"type"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
	Usage       string `yaml:"usage" json:"usage" mapstructure:"usage"`
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
	Default     any    `yaml:"default" json:"default" mapstructure:"default"`
}

type CommandEnv struct {
	Key          string `yaml:"key" json:"key" mapstructure:"key"`
	Value        string `yaml:"value" json:"value" mapstructure:"value"`
	ValueCommand string `yaml:"valueCommand" json:"valueCommand" mapstructure:"valueCommand"`
}

type CommandComponentConfig struct {
	Component string `yaml:"component" json:"component" mapstructure:"component"`
	Stack     string `yaml:"stack" json:"stack" mapstructure:"stack"`
}

// CLI command aliases

type CommandAliases map[string]string
