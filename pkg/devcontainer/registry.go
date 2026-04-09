package devcontainer

// Registry manages devcontainer configurations.
type Registry interface {
	// Register a named devcontainer configuration
	Register(name string, config *Config) error

	// Get a devcontainer configuration by name
	Get(name string) (*Config, error)

	// List all registered devcontainer configurations
	List() ([]Info, error)

	// Exists checks if a devcontainer configuration exists
	Exists(name string) bool
}

// Info represents metadata about a registered devcontainer.
type Info struct {
	Name        string
	Image       string
	Description string
}

// Config represents a complete devcontainer configuration.
type Config struct {
	Name            string                    `yaml:"name" json:"name" mapstructure:"name"`
	Image           string                    `yaml:"image" json:"image" mapstructure:"image"`
	Build           *Build                    `yaml:"build,omitempty" json:"build,omitempty" mapstructure:"build"`
	WorkspaceFolder string                    `yaml:"workspaceFolder,omitempty" json:"workspaceFolder,omitempty" mapstructure:"workspacefolder"`
	WorkspaceMount  string                    `yaml:"workspaceMount,omitempty" json:"workspaceMount,omitempty" mapstructure:"workspacemount"`
	Mounts          []string                  `yaml:"mounts,omitempty" json:"mounts,omitempty" mapstructure:"mounts"`
	ForwardPorts    []interface{}             `yaml:"forwardPorts,omitempty" json:"forwardPorts,omitempty" mapstructure:"forwardports"`
	PortsAttributes map[string]PortAttributes `yaml:"portsAttributes,omitempty" json:"portsAttributes,omitempty" mapstructure:"portsattributes"`
	ContainerEnv    map[string]string         `yaml:"containerEnv,omitempty" json:"containerEnv,omitempty" mapstructure:"containerenv"`
	RemoteUser      string                    `yaml:"remoteUser,omitempty" json:"remoteUser,omitempty" mapstructure:"remoteuser"`

	// Runtime configuration
	RunArgs         []string `yaml:"runArgs,omitempty" json:"runArgs,omitempty" mapstructure:"runargs"`
	OverrideCommand bool     `yaml:"overrideCommand,omitempty" json:"overrideCommand,omitempty" mapstructure:"overridecommand"`
	Init            bool     `yaml:"init,omitempty" json:"init,omitempty" mapstructure:"init"`
	Privileged      bool     `yaml:"privileged,omitempty" json:"privileged,omitempty" mapstructure:"privileged"`
	CapAdd          []string `yaml:"capAdd,omitempty" json:"capAdd,omitempty" mapstructure:"capadd"`
	SecurityOpt     []string `yaml:"securityOpt,omitempty" json:"securityOpt,omitempty" mapstructure:"securityopt"`
	UserEnvProbe    string   `yaml:"userEnvProbe,omitempty" json:"userEnvProbe,omitempty" mapstructure:"userenvprobe"`
}

// Build represents build configuration for a devcontainer.
type Build struct {
	Dockerfile string            `yaml:"dockerfile" json:"dockerfile" mapstructure:"dockerfile"`
	Context    string            `yaml:"context,omitempty" json:"context,omitempty" mapstructure:"context"`
	Args       map[string]string `yaml:"args,omitempty" json:"args,omitempty" mapstructure:"args"`
}

// PortAttributes represents metadata for a forwarded port.
type PortAttributes struct {
	Label    string `yaml:"label,omitempty" json:"label,omitempty" mapstructure:"label"`
	Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty" mapstructure:"protocol"`
}
