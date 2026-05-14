package schema

// ComponentDependency represents a dependency entry in dependencies.components.
type ComponentDependency struct {
	// Component instance name (required for component-type dependencies).
	// This is the name under components.<kind>.<name>, not the Terraform module path.
	Component string `yaml:"component,omitempty" json:"component,omitempty" mapstructure:"component"`
	// Stack name (optional, defaults to current stack). Supports Go templates.
	Stack string `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	// Kind specifies the dependency type: terraform, helmfile, packer, file, folder, or plugin type.
	// Defaults to the declaring component's type for component dependencies.
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty" mapstructure:"kind"`
	// Path for file or folder dependencies (required when kind is "file" or "folder").
	Path string `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`

	// Legacy context fields from settings.depends_on format.
	// These are only populated when reading from the deprecated settings.depends_on format.
	// For new dependencies.components format, use the stack field with templates instead.
	Namespace   string `yaml:"-" json:"-" mapstructure:"namespace"`
	Tenant      string `yaml:"-" json:"-" mapstructure:"tenant"`
	Environment string `yaml:"-" json:"-" mapstructure:"environment"`
	Stage       string `yaml:"-" json:"-" mapstructure:"stage"`
}

// IsFileDependency returns true if this is a file dependency.
func (d *ComponentDependency) IsFileDependency() bool {
	return d.Kind == "file"
}

// IsFolderDependency returns true if this is a folder dependency.
func (d *ComponentDependency) IsFolderDependency() bool {
	return d.Kind == "folder"
}

// IsComponentDependency returns true if this is a component dependency (not file or folder).
func (d *ComponentDependency) IsComponentDependency() bool {
	return d.Kind != "file" && d.Kind != "folder"
}

// Dependencies declares required tools and component dependencies.
type Dependencies struct {
	// Tools maps tool names to version constraints (e.g., "terraform": "1.5.0" or "latest").
	Tools map[string]string `yaml:"tools,omitempty" json:"tools,omitempty" mapstructure:"tools"`
	// Components lists component dependencies that must be applied before this component.
	// Uses list format with always-append merge behavior (child lists extend parent lists).
	Components []ComponentDependency `yaml:"components,omitempty" json:"components,omitempty" mapstructure:"components"`
}
