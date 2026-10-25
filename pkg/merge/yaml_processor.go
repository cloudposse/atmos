package merge

// YAMLFunctionProcessor is an interface for processing YAML functions.
// This allows the merge package to process YAML functions without depending on internal/exec.
type YAMLFunctionProcessor interface {
	// ProcessYAMLFunctionString processes a single YAML function string and returns the processed value.
	// The value parameter is the YAML function string (e.g., "!template '{{ .settings.vpc_cidr }}'").
	// Returns the processed value (which may be any type) or an error.
	ProcessYAMLFunctionString(value string) (any, error)
}

// ConcreteValueProcessor optionally defers rendering inherited strings until after
// their containers have merged. Function results are already resolved and must not
// be prepared or evaluated a second time.
type ConcreteValueProcessor interface {
	YAMLFunctionProcessor
	// DeferConcreteValue returns a copy of a concrete container with lazy leaves.
	DeferConcreteValue(value any) any
	// ResolveConcreteValue evaluates only lazy leaves surviving the merge.
	ResolveConcreteValue(value any) (any, error)
}
