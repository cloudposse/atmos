package values

// ValueExtractor handles the extraction of values from stack configurations.
type ValueExtractor interface {
	// ExtractStackValues extracts values from stack configurations for a given component.
	ExtractStackValues(stacksMap map[string]interface{}, component string, includeAbstract bool) (map[string]interface{}, error)

	// ApplyValueQuery applies a query to extracted values and returns the filtered results.
	ApplyValueQuery(values map[string]interface{}, query string) (map[string]interface{}, error)
}

// StackValue represents a value extracted from a stack.
type StackValue struct {
	Value      interface{}
	IsAbstract bool
	Stack      string
}

// QueryResult represents the result of applying a query to stack values.
type QueryResult struct {
	Values map[string]interface{}
	Query  string
}

// ExtractOptions contains options for value extraction.
type ExtractOptions struct {
	Component       string
	IncludeAbstract bool
	StackPattern    string
}

// DefaultExtractor provides a default implementation of ValueExtractor.
type DefaultExtractor struct{}

// NewDefaultExtractor creates a new DefaultExtractor.
func NewDefaultExtractor() *DefaultExtractor {
	return &DefaultExtractor{}
}
