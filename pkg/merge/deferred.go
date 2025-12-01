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
