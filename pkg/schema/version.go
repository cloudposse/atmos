package schema

type VersionCheck struct {
	Enabled   bool   `yaml:"enabled,omitempty" mapstructure:"enabled"`
	Timeout   int    `yaml:"timeout,omitempty" mapstructure:"timeout"`
	Frequency string `yaml:"frequency,omitempty" mapstructure:"frequency"`
}

type Version struct {
	Check VersionCheck `yaml:"check,omitempty" mapstructure:"check"`
}
