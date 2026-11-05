package hooks

import (
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// stepDelimiterState caches the step template delimiters resolved for one hook invocation.
type stepDelimiterState struct {
	delims   delimiters.Delimiters
	err      error
	resolved bool
}

// stepDelimiters returns the delimiters that this hook's step templates use, resolving them from
// the Atmos configuration on first use (see delimiters.ForSteps) and reusing the result after
// that. Engines call it before they build step variables so an invalid edition pin or delimiter
// list fails the hook instead of selecting a pair silently.
func (c *ExecContext) stepDelimiters() (delimiters.Delimiters, error) {
	if !c.stepDelims.resolved {
		c.stepDelims.delims, c.stepDelims.err = delimiters.ForSteps(c.AtmosConfig)
		c.stepDelims.resolved = true
	}
	return c.stepDelims.delims, c.stepDelims.err
}
