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
// This returns stacks keyed by their LOGICAL names (based on name_pattern/name_template)
// not by their manifest file names. This is important for component resolution since
// users specify stacks by logical name (e.g., "test") not by file name (e.g., "test-stack").
func (l *ExecStackLoader) FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	defer perf.Track(atmosConfig, "exec.ExecStackLoader.FindStacksMap")()

	// Use ExecuteDescribeStacks instead of FindStacksMap to get stacks keyed by logical name.
	// FindStacksMap returns stacks by manifest file name, but we need logical stack names
	// since users specify stacks by logical name (e.g., "-s test" not "-s test-stack").
	stacksMap, err := ExecuteDescribeStacks(
		atmosConfig,
		"",  // filterByStack - empty to get all stacks
		nil, // components - nil to get all
		nil, // componentTypes - nil to get all
		nil, // sections - nil to get all
		ignoreMissingFiles,
		false, // processTemplates - false for performance, we only need structure
		false, // processYamlFunctions - false for performance
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager - not needed for component resolution
	)
	if err != nil {
		return nil, nil, err
	}

	// Return the processed stacks map with logical stack names.
	// The second return value (rawStackConfigs) is not available from ExecuteDescribeStacks,
	// but it's not used by the component resolver.
	return stacksMap, nil, nil
}
