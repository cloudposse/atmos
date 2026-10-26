package schema

// LSPSettings contains configuration for Language Server Protocol integration.
type LSPSettings struct {
	Enabled bool                  `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Servers map[string]*LSPServer `yaml:"servers,omitempty" json:"servers,omitempty" mapstructure:"servers"`
}

// LSPServer contains configuration for a single LSP server.
type LSPServer struct {
	Command               string                 `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`                                              // Command to run (e.g., "yaml-language-server")
	Args                  []string               `yaml:"args,omitempty" json:"args,omitempty" mapstructure:"args"`                                                       // Command arguments (e.g., ["--stdio"])
	FileTypes             []string               `yaml:"filetypes,omitempty" json:"filetypes,omitempty" mapstructure:"filetypes"`                                        // Supported file types (e.g., ["yaml", "yml"])
	RootPatterns          []string               `yaml:"root_patterns,omitempty" json:"root_patterns,omitempty" mapstructure:"root_patterns"`                            // Workspace root patterns (e.g., ["atmos.yaml", ".git"])
	InitializationOptions map[string]interface{} `yaml:"initialization_options,omitempty" json:"initialization_options,omitempty" mapstructure:"initialization_options"` // Custom initialization options
}
