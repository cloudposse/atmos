package schema

// ScaffoldConfiguration contains the effective catalog of named scaffolds.
// Templates retain their original names and fields, including case and dots.
type ScaffoldConfiguration struct {
	Templates map[string]any `yaml:"templates,omitempty" json:"templates,omitempty" mapstructure:"templates"`
	// SourceDirs anchors each effective source to its declaring configuration file.
	SourceDirs map[string]string `yaml:"-" json:"-" mapstructure:"-"`
}
