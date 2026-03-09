package schema

// AISecuritySettings contains configuration for AI security and compliance features.
type AISecuritySettings struct {
	Enabled         bool                 `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Sources         AISecuritySources    `yaml:"sources,omitempty" json:"sources,omitempty" mapstructure:"sources"`
	DefaultSeverity []string             `yaml:"default_severity,omitempty" json:"default_severity,omitempty" mapstructure:"default_severity"` // Default severity filter (e.g., ["CRITICAL", "HIGH"]).
	MaxFindings     int                  `yaml:"max_findings,omitempty" json:"max_findings,omitempty" mapstructure:"max_findings"`             // Maximum findings per analysis run (controls AI costs).
	TagMapping      AISecurityTagMapping `yaml:"tag_mapping,omitempty" json:"tag_mapping,omitempty" mapstructure:"tag_mapping"`
	Frameworks      []string             `yaml:"frameworks,omitempty" json:"frameworks,omitempty" mapstructure:"frameworks"` // Compliance frameworks to track (e.g., ["cis-aws", "pci-dss"]).
}

// AISecuritySources controls which AWS security services to query.
type AISecuritySources struct {
	SecurityHub    bool `yaml:"security_hub,omitempty" json:"security_hub,omitempty" mapstructure:"security_hub"`
	Config         bool `yaml:"config,omitempty" json:"config,omitempty" mapstructure:"config"`
	Inspector      bool `yaml:"inspector,omitempty" json:"inspector,omitempty" mapstructure:"inspector"`
	GuardDuty      bool `yaml:"guardduty,omitempty" json:"guardduty,omitempty" mapstructure:"guardduty"`
	Macie          bool `yaml:"macie,omitempty" json:"macie,omitempty" mapstructure:"macie"`
	AccessAnalyzer bool `yaml:"access_analyzer,omitempty" json:"access_analyzer,omitempty" mapstructure:"access_analyzer"`
}

// AISecurityTagMapping configures the tag keys used for finding-to-code mapping.
type AISecurityTagMapping struct {
	StackTag       string `yaml:"stack_tag,omitempty" json:"stack_tag,omitempty" mapstructure:"stack_tag"`                   // Default: "atmos:stack".
	ComponentTag   string `yaml:"component_tag,omitempty" json:"component_tag,omitempty" mapstructure:"component_tag"`       // Default: "atmos:component".
	TenantTag      string `yaml:"tenant_tag,omitempty" json:"tenant_tag,omitempty" mapstructure:"tenant_tag"`                // Default: "atmos:tenant".
	EnvironmentTag string `yaml:"environment_tag,omitempty" json:"environment_tag,omitempty" mapstructure:"environment_tag"` // Default: "atmos:environment".
	StageTag       string `yaml:"stage_tag,omitempty" json:"stage_tag,omitempty" mapstructure:"stage_tag"`                   // Default: "atmos:stage".
}

// DefaultAISecurityTagMapping returns the default tag mapping for finding-to-code resolution.
func DefaultAISecurityTagMapping() AISecurityTagMapping {
	return AISecurityTagMapping{
		StackTag:       "atmos:stack",
		ComponentTag:   "atmos:component",
		TenantTag:      "atmos:tenant",
		EnvironmentTag: "atmos:environment",
		StageTag:       "atmos:stage",
	}
}

// DefaultAISecuritySources returns default security sources (Security Hub primary).
func DefaultAISecuritySources() AISecuritySources {
	return AISecuritySources{
		SecurityHub:    true,
		Config:         true,
		Inspector:      true,
		GuardDuty:      true,
		Macie:          false,
		AccessAnalyzer: false,
	}
}
