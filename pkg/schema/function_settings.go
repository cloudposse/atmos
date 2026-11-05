package schema

// FunctionSettings configures YAML function evaluation.
type FunctionSettings struct {
	Exec ExecFunctionSettings `yaml:"exec,omitempty" json:"exec,omitempty" mapstructure:"exec"`
}

// ExecFunctionSettings controls the number of simultaneous !exec calls in one document evaluation.
type ExecFunctionSettings struct {
	MaxConcurrency *int `yaml:"max_concurrency,omitempty" json:"max_concurrency,omitempty" mapstructure:"max_concurrency" jsonschema:"minimum=1,default=1"`
}

// Concurrency returns the explicit limit, or the serial default when omitted.
func (s ExecFunctionSettings) Concurrency() int {
	if s.MaxConcurrency == nil {
		return 1
	}
	return *s.MaxConcurrency
}
