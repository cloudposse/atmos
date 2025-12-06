package exec

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecStackLoader implements the component.StackLoader interface.
// This allows internal/exec to provide stack loading functionality to pkg/component.
// This avoids a circular dependency.
type ExecStackLoader struct{}

// NewStackLoader creates a new stack loader.
func NewStackLoader() *ExecStackLoader {
	defer perf.Track(nil, "exec.NewStackLoader")()

	return &ExecStackLoader{}
}

// FindStacksMap implements component.StackLoader.
func (l *ExecStackLoader) FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	defer perf.Track(atmosConfig, "exec.ExecStackLoader.FindStacksMap")()

	return FindStacksMap(atmosConfig, ignoreMissingFiles)
}
