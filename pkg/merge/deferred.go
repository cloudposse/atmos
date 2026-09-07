package merge

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DeferredValue represents a value that contains a YAML function and needs
// to be processed after the initial merge.
type DeferredValue struct {
	Path       []string    // Field path (e.g., ["components", "terraform", "vpc", "vars", "config"]).
	Value      interface{} // The YAML function string or the final processed value.
	Precedence int         // Merge precedence (higher = later in import chain = higher priority).
	IsFunction bool        // True if Value is still a YAML function string, false if processed.
}

// DeferredMergeContext tracks all deferred values during the merge process.
type DeferredMergeContext struct {
	deferredValues map[string][]*DeferredValue // Key is path joined with ".".
	precedence     int                         // Current precedence counter.
}

// NewDeferredMergeContext creates a new deferred merge context for tracking deferred values.
func NewDeferredMergeContext() *DeferredMergeContext {
	defer perf.Track(nil, "merge.NewDeferredMergeContext")()

	return &DeferredMergeContext{
		deferredValues: make(map[string][]*DeferredValue),
		precedence:     0,
	}
}

// AddDeferred adds a deferred value to the context.
func (dmc *DeferredMergeContext) AddDeferred(path []string, value interface{}) {
	key := strings.Join(path, ".")
	dmc.deferredValues[key] = append(dmc.deferredValues[key], &DeferredValue{
		Path:       path,
		Value:      value,
		Precedence: dmc.precedence,
		IsFunction: true,
	})
}

// IncrementPrecedence increases the precedence counter (call after each import).
func (dmc *DeferredMergeContext) IncrementPrecedence() {
	dmc.precedence++
}

// GetDeferredValues returns all deferred values.
// The returned map is the internal storage and can be modified by the caller.
func (dmc *DeferredMergeContext) GetDeferredValues() map[string][]*DeferredValue {
	return dmc.deferredValues
}

// HasDeferredValues returns true if there are any deferred values.
func (dmc *DeferredMergeContext) HasDeferredValues() bool {
	return len(dmc.deferredValues) > 0
}

// Clone creates a deep copy of the deferred merge context, including new
// *DeferredValue instances for every entry. ApplyDeferredMerges mutates the
// DeferredValue.Value/IsFunction fields of the context it's given in place;
// callers that recover a context from shared/cached storage (e.g. a
// FindStacksMap cache entry reused across concurrent invocations) must clone
// before resolving, so that resolving one caller's copy can't corrupt another
// concurrent caller's unresolved view of the same context.
func (dmc *DeferredMergeContext) Clone() *DeferredMergeContext {
	defer perf.Track(nil, "merge.DeferredMergeContext.Clone")()

	if dmc == nil {
		return nil
	}

	newDmc := &DeferredMergeContext{
		deferredValues: make(map[string][]*DeferredValue, len(dmc.deferredValues)),
		precedence:     dmc.precedence,
	}

	for key, values := range dmc.deferredValues {
		clonedValues := make([]*DeferredValue, len(values))
		for i, dv := range values {
			clonedPath := make([]string, len(dv.Path))
			copy(clonedPath, dv.Path)
			clonedValues[i] = &DeferredValue{
				Path:       clonedPath,
				Value:      dv.Value,
				Precedence: dv.Precedence,
				IsFunction: dv.IsFunction,
			}
		}
		newDmc.deferredValues[key] = clonedValues
	}

	return newDmc
}
