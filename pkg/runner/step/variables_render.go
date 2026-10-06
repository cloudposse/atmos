package step

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// ResolveWithData renders input with the same renderer, template functions, and pass count that
// Resolve uses, against the supplied data instead of the step variables. Parallel and matrix
// children use it so their template fields behave exactly like those of a sequential step in the
// same context. The name labels the template in render errors.
func (v *Variables) ResolveWithData(name, input string, data map[string]any) (string, error) {
	defer perf.Track(nil, "step.Variables.ResolveWithData")()

	if input == "" {
		return "", nil
	}
	return v.resolveTemplate(name, input, data)
}
