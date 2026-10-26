package compat

import (
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	globalSeparatedArgs []string
	separatedMu         sync.RWMutex
)

// SetSeparated stores the separated args (pass-through flags for terraform, etc.).
// Called once during preprocessing in Execute() before Cobra parses.
//
// Separated args are flags that should be passed through to the underlying command,
// (e.g., terraform -out=/tmp/plan, -var=foo=bar) rather than being parsed by Atmos.
// These are identified by the CompatibilityFlagTranslator during preprocessing.
//
// Note: passing an empty (non-nil) slice is equivalent to passing nil — the global
// state will be nil and GetSeparated() will return nil. This is intentional: callers
// that range over the result are unaffected, and it avoids spurious "no args" vs
// "zero-length args" ambiguity. This contract is tested and must be preserved.
func SetSeparated(separatedArgs []string) {
	defer perf.Track(nil, "compat.SetSeparated")()

	separatedMu.Lock()
	defer separatedMu.Unlock()
	// Defensive copy to prevent callers from mutating the global state.
	globalSeparatedArgs = append([]string(nil), separatedArgs...)
}

// GetSeparated returns the separated args (terraform pass-through flags like -out, -var).
// Returns nil if no separated args were set during preprocessing.
//
// Usage in RunE.
//
//	separatedArgs := compat.GetSeparated()
func GetSeparated() []string {
	defer perf.Track(nil, "compat.GetSeparated")()

	separatedMu.RLock()
	defer separatedMu.RUnlock()
	if globalSeparatedArgs == nil {
		return nil
	}
	// Return a defensive copy to prevent data races.
	return append([]string{}, globalSeparatedArgs...)
}

// ResetSeparated clears the separated args. Used for testing.
func ResetSeparated() {
	defer perf.Track(nil, "compat.ResetSeparated")()

	separatedMu.Lock()
	defer separatedMu.Unlock()
	globalSeparatedArgs = nil
}
