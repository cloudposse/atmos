package schema

// AWSSecuritySettings contains configuration for AWS security and compliance features.
type AWSSecuritySettings struct {
	Enabled         bool                  `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Identity        string                `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"` // Atmos Auth identity for AWS credential injection.
	Region          string                `yaml:"region,omitempty" json:"region,omitempty" mapstructure:"region"`       // Default AWS region (e.g., Security Hub aggregation region).
	Sources         AWSSecuritySources    `yaml:"sources,omitempty" json:"sources,omitempty" mapstructure:"sources"`
	DefaultSeverity []string              `yaml:"default_severity,omitempty" json:"default_severity,omitempty" mapstructure:"default_severity"` // Default severity filter (e.g., ["CRITICAL", "HIGH"]).
	MaxFindings     int                   `yaml:"max_findings,omitempty" json:"max_findings,omitempty" mapstructure:"max_findings"`             // Maximum findings per analysis run (controls AI costs).
	TagMapping      AWSSecurityTagMapping `yaml:"tag_mapping,omitempty" json:"tag_mapping,omitempty" mapstructure:"tag_mapping"`
	AccountMap      map[string]string     `yaml:"account_map,omitempty" json:"account_map,omitempty" mapstructure:"account_map"` // Account ID → name for account-level findings.
	Frameworks      []string              `yaml:"frameworks,omitempty" json:"frameworks,omitempty" mapstructure:"frameworks"`    // Compliance frameworks to track (e.g., ["cis-aws", "pci-dss"]).
}

// AWSSecuritySources controls which AWS security services to query.
type AWSSecuritySources struct {
	SecurityHub    bool `yaml:"security_hub,omitempty" json:"security_hub,omitempty" mapstructure:"security_hub"`
	Config         bool `yaml:"config,omitempty" json:"config,omitempty" mapstructure:"config"`
	Inspector      bool `yaml:"inspector,omitempty" json:"inspector,omitempty" mapstructure:"inspector"`
	GuardDuty      bool `yaml:"guardduty,omitempty" json:"guardduty,omitempty" mapstructure:"guardduty"`
	Macie          bool `yaml:"macie,omitempty" json:"macie,omitempty" mapstructure:"macie"`
	AccessAnalyzer bool `yaml:"access_analyzer,omitempty" json:"access_analyzer,omitempty" mapstructure:"access_analyzer"`
}

// AWSSecurityTagMapping configures the tag keys used for finding-to-code mapping.
// Only two tags are needed: one for the stack name and one for the component name.
// These are configurable so organizations can use their own tagging standards.
type AWSSecurityTagMapping struct {
	StackTag     string `yaml:"stack_tag,omitempty" json:"stack_tag,omitempty" mapstructure:"stack_tag"`             // Default: "atmos:stack".
	ComponentTag string `yaml:"component_tag,omitempty" json:"component_tag,omitempty" mapstructure:"component_tag"` // Default: "atmos:component".
}

// DefaultAWSSecurityTagMapping returns the default tag mapping for finding-to-code resolution.
func DefaultAWSSecurityTagMapping() AWSSecurityTagMapping {
	return AWSSecurityTagMapping{
		StackTag:     "atmos:stack",
		ComponentTag: "atmos:component",
	}
}

// DefaultAWSSecuritySources returns default AWS security sources (Security Hub primary).
func DefaultAWSSecuritySources() AWSSecuritySources {
	return AWSSecuritySources{
		SecurityHub:    true,
		Config:         true,
		Inspector:      true,
		GuardDuty:      true,
		Macie:          false,
		AccessAnalyzer: false,
	}
}
