package config

// CoverageConfig represents the coverage configuration structure.
type CoverageConfig struct {
	Enabled    bool               `mapstructure:"enabled" yaml:"enabled"`
	Profile    string             `mapstructure:"profile" yaml:"profile"`
	Packages   []string           `mapstructure:"packages" yaml:"packages"`
	Analysis   CoverageAnalysis   `mapstructure:"analysis" yaml:"analysis"`
	Output     CoverageOutput     `mapstructure:"output" yaml:"output"`
	Thresholds CoverageThresholds `mapstructure:"thresholds" yaml:"thresholds"`
}

// CoverageAnalysis defines what analysis to perform on coverage data.
type CoverageAnalysis struct {
	Functions  bool     `mapstructure:"functions" yaml:"functions"`
	Statements bool     `mapstructure:"statements" yaml:"statements"`
	Uncovered  bool     `mapstructure:"uncovered" yaml:"uncovered"`
	Exclude    []string `mapstructure:"exclude" yaml:"exclude"`
}

// CoverageOutput defines how to output coverage information.
type CoverageOutput struct {
	Terminal TerminalOutput `mapstructure:"terminal" yaml:"terminal"`
	// Future: HTML, JSON output options can be added here
}

// TerminalOutput defines terminal-specific output options.
type TerminalOutput struct {
	Format        string `mapstructure:"format" yaml:"format"` // summary, detailed, none
	ShowUncovered int    `mapstructure:"show_uncovered" yaml:"show_uncovered"`
}

// CoverageThresholds defines coverage threshold requirements.
type CoverageThresholds struct {
	Total     float64             `mapstructure:"total" yaml:"total"`
	Packages  []PackageThreshold  `mapstructure:"packages" yaml:"packages"`
	FailUnder bool               `mapstructure:"fail_under" yaml:"fail_under"`
}

// PackageThreshold defines a coverage threshold for a specific package pattern.
type PackageThreshold struct {
	Pattern   string  `mapstructure:"pattern" yaml:"pattern"`
	Threshold float64 `mapstructure:"threshold" yaml:"threshold"`
}