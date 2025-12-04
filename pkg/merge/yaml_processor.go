package merge

// YAMLFunctionProcessor is an interface for processing YAML functions.
// This allows the merge package to process YAML functions without depending on internal/exec.
type YAMLFunctionProcessor interface {
	// ProcessYAMLFunctionString processes a single YAML function string and returns the processed value.
	// The value parameter is the YAML function string (e.g., "!template '{{ .settings.vpc_cidr }}'").
	// Returns the processed value (which may be any type) or an error.
	ProcessYAMLFunctionString(value string) (any, error)
}
