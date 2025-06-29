package schema

// Deployment represents a deployment of a component for a specific stack.
type Deployment struct {
	Component     string              `yaml:"component" json:"component" mapstructure:"component"`
	Stack         string              `yaml:"stack" json:"stack" mapstructure:"stack"`
	ComponentType string              `yaml:"component_type" json:"component_type" mapstructure:"component_type"`
	Settings      AtmosSectionMapType `yaml:"settings" json:"settings" mapstructure:"settings"`
	Vars          AtmosSectionMapType `yaml:"vars" json:"vars" mapstructure:"vars"`
	Env           AtmosSectionMapType `yaml:"env" json:"env" mapstructure:"env"`
	Backend       AtmosSectionMapType `yaml:"backend" json:"backend" mapstructure:"backend"`
	Metadata      AtmosSectionMapType `yaml:"metadata" json:"metadata" mapstructure:"metadata"`
}
