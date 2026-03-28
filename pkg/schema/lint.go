package schema

// LintStacksConfig holds lint stacks-specific configuration from atmos.yaml under
// the `lint.stacks:` key.
type LintStacksConfig struct {
	// MaxImportDepth is the maximum allowed import depth (L-03 threshold). Default: 3.
	MaxImportDepth int `yaml:"max_import_depth,omitempty" json:"max_import_depth,omitempty" mapstructure:"max_import_depth"`
	// DRYThresholdPct is the percentage threshold for L-06 DRY extraction suggestions. Default: 80.
	DRYThresholdPct int `yaml:"dry_threshold_pct,omitempty" json:"dry_threshold_pct,omitempty" mapstructure:"dry_threshold_pct"`
	// CohesionMaxGroups is the maximum number of concern groups allowed per catalog file
	// before L-05 triggers a finding. Default: 3.
	CohesionMaxGroups int `yaml:"cohesion_max_groups,omitempty" json:"cohesion_max_groups,omitempty" mapstructure:"cohesion_max_groups"`
	// SensitiveVarPatterns is a list of glob patterns for sensitive variable names (L-08).
	// User-provided patterns are merged with built-in defaults (*password*, *secret*, etc.)
	// so common sensitive names are always checked. When empty, patterns from
	// settings.terminal.mask.sensitive_key_patterns are used as the base before merging defaults.
	SensitiveVarPatterns []string `yaml:"sensitive_var_patterns,omitempty" json:"sensitive_var_patterns,omitempty" mapstructure:"sensitive_var_patterns"`
	// Rules maps rule IDs to their configured severity level (e.g. "L-03": "error").
	// User overrides are merged with defaults: unspecified rules retain their default severity.
	Rules map[string]string `yaml:"rules,omitempty" json:"rules,omitempty" mapstructure:"rules"`
}

// LintConfig holds lint-specific configuration from atmos.yaml under the `lint:` key.
type LintConfig struct {
	// Stacks holds lint configuration specific to the `atmos lint stacks` command.
	Stacks LintStacksConfig `yaml:"stacks,omitempty" json:"stacks,omitempty" mapstructure:"stacks"`
}
