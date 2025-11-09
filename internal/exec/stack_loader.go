package exec

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecStackLoader implements the component.StackLoader interface.
// This allows internal/exec to provide stack loading functionality to pkg/component
// without creating a circular dependency.
type ExecStackLoader struct{}

// NewStackLoader creates a new stack loader.
func NewStackLoader() *ExecStackLoader {
	return &ExecStackLoader{}
}

// FindStacksMap implements component.StackLoader.
func (l *ExecStackLoader) FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	return FindStacksMap(atmosConfig, ignoreMissingFiles)
}
