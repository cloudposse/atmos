package schema

// AWSSettings contains configuration for AWS-specific features.
type AWSSettings struct {
	Security AWSSecuritySettings `yaml:"security,omitempty" json:"security,omitempty" mapstructure:"security"`
}
