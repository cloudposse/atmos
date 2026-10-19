package deferred

import (
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/degradation"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CanRecover adds credential unavailability to a caller's existing value policy.
// Providers classify their errors; this package does not interpret SDK error codes.
func CanRecover(err error, enabled bool, existing func(error) bool) bool {
	defer perf.Track(nil, "deferred.CanRecover")()
	return existing(err) || (enabled && errors.Is(err, errUtils.ErrAuthenticationUnavailable))
}

// ValueProcessor applies recovery at an individual YAML value boundary.
type ValueProcessor struct {
	Inner       m.YAMLFunctionProcessor
	Recoverable func(error) bool
	Warn        func(string, error)
}

func (p *ValueProcessor) ProcessYAMLFunctionString(value string) (any, error) {
	defer perf.Track(nil, "deferred.ValueProcessor.ProcessYAMLFunctionString")()
	result, err := p.Inner.ProcessYAMLFunctionString(value)
	if err != nil && p.Warn != nil && p.Recoverable(err) {
		p.Warn(value, err)
		return degradation.AtmosComputedValue{}, nil
	}
	return result, err
}

// ExcludeComputedFields prevents a second substitution during deferred merging.
func ExcludeComputedFields(dctx *m.DeferredMergeContext, section map[string]any) {
	defer perf.Track(nil, "deferred.ExcludeComputedFields")()
	for key, values := range dctx.GetDeferredValues() {
		if len(values) == 0 {
			continue
		}
		var value any = section
		for _, part := range values[0].Path {
			nested, ok := value.(map[string]any)
			if !ok {
				break
			}
			value = nested[part]
		}
		if _, computed := value.(degradation.AtmosComputedValue); computed {
			delete(dctx.GetDeferredValues(), key)
		}
	}
}
