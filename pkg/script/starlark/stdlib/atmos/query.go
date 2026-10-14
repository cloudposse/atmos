package atmos

import (
	"strings"

	"go.starlark.net/starlark"
)

// queryDefaults applies only to data-reading wrappers. The atmos.run function retains literal
// CLI behavior, and mutations such as config set retain streaming output.
func (s *binding) queryDefaults(argv, commandPath []string, opts *Options, kwargs []starlark.Tuple) []string {
	path := s.catalog.CommandPath(commandPath)
	if !isQuery(path) {
		return argv
	}
	explicitOutput := false
	for _, kwarg := range kwargs {
		if kwarg[0] == starlark.String("output") {
			explicitOutput = true
		}
	}
	if !explicitOutput {
		opts.Output = "capture"
	}
	return s.catalog.WithDefaultFlag(argv, path, "format", "json")
}

func isQuery(path []string) bool {
	if len(path) == 0 {
		return false
	}
	if path[0] == "list" || path[0] == "describe" {
		return true
	}
	switch strings.Join(path, " ") {
	case "config get", "stack config get", "stack get":
		return true
	default:
		return false
	}
}
