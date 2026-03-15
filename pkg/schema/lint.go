package schema

// LintStacksConfig holds lint stacks-specific configuration from atmos.yaml under
// the `lint.stacks:` key.
type LintStacksConfig struct {
	// MaxImportDepth is the maximum allowed import depth (L-03 threshold). Default: 3.
	MaxImportDepth int `yaml:"max_import_depth,omitempty" json:"max_import_depth,omitempty" mapstructure:"max_import_depth"`
	// DRYThresholdPct is the percentage threshold for L-06 DRY extraction suggestions. Default: 80.
	DRYThresholdPct int `yaml:"dry_threshold_pct,omitempty" json:"dry_threshold_pct,omitempty" mapstructure:"dry_threshold_pct"`
	// SensitiveVarPatterns is a list of glob patterns for sensitive variable names (L-08).
	// Merged with built-in defaults.
	SensitiveVarPatterns []string `yaml:"sensitive_var_patterns,omitempty" json:"sensitive_var_patterns,omitempty" mapstructure:"sensitive_var_patterns"`
	// Rules maps rule IDs to their configured severity level (e.g. "L-03": "error").
	// Used to override default severities.
	Rules map[string]string `yaml:"rules,omitempty" json:"rules,omitempty" mapstructure:"rules"`
}

// LintConfig holds lint-specific configuration from atmos.yaml under the `lint:` key.
type LintConfig struct {
	// Stacks holds lint configuration specific to the `atmos lint stacks` command.
	Stacks LintStacksConfig `yaml:"stacks,omitempty" json:"stacks,omitempty" mapstructure:"stacks"`
}
