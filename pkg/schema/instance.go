package schema

// Instance represents an instance of a component for a specific stack.
type Instance struct {
	Component     string              `yaml:"component" json:"component" mapstructure:"component"`
	Stack         string              `yaml:"stack" json:"stack" mapstructure:"stack"`
	ComponentType string              `yaml:"component_type" json:"component_type" mapstructure:"component_type"`
	Settings      AtmosSectionMapType `yaml:"settings" json:"settings" mapstructure:"settings"`
	// Pro holds the top-level `pro:` component section. settings.pro is a deprecated alias
	// resolved by pro.ResolveSection.
	Pro      AtmosSectionMapType `yaml:"pro,omitempty" json:"pro,omitempty" mapstructure:"pro"`
	Vars     AtmosSectionMapType `yaml:"vars" json:"vars" mapstructure:"vars"`
	Env      AtmosSectionMapType `yaml:"env" json:"env" mapstructure:"env"`
	Backend  AtmosSectionMapType `yaml:"backend" json:"backend" mapstructure:"backend"`
	Source   AtmosSectionMapType `yaml:"source" json:"source" mapstructure:"source"`
	Metadata AtmosSectionMapType `yaml:"metadata" json:"metadata" mapstructure:"metadata"`
}
