package edition

import (
	"fmt"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/perf"
)

// behaviorIndex maps each behavior key to its first transition in Journal order, for one journal.
type behaviorIndex struct {
	source      []Entry
	transitions map[string]Entry
}

// behaviorIndexCache holds the index for the current journal. BehaviorChanged runs for every
// evaluated YAML value, so copying and sorting the journal on each call dominated stack
// processing time. The journal is immutable in production; tests that swap it get a new index.
var behaviorIndexCache atomic.Pointer[behaviorIndex]

func sameEntries(a, b []Entry) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func behaviorTransitions() map[string]Entry {
	if cached := behaviorIndexCache.Load(); cached != nil && sameEntries(cached.source, journal) {
		return cached.transitions
	}
	index := &behaviorIndex{source: journal, transitions: map[string]Entry{}}
	for _, entry := range Journal() {
		if _, seen := index.transitions[entry.Key]; entry.Kind == KindBehavior && !seen {
			index.transitions[entry.Key] = entry
		}
	}
	behaviorIndexCache.Store(index)
	return index.transitions
}

// BehaviorChanged reports whether a named behavior transition applies at raw's
// edition pin. An empty pin selects the new behavior; a dated pin selects it on
// or after the journal entry's date, including end-of-period rounding.
// Each behavior key identifies exactly one transition, not a configuration key.
// Unknown keys and malformed pins return errors rather than silently selecting
// a code path. Callers pass the effective pin from AtmosConfiguration.Edition.
func BehaviorChanged(raw, key string) (bool, error) {
	defer perf.Track(nil, "edition.BehaviorChanged")()

	var anchor *Anchor
	if raw != "" {
		parsed, err := ParseAnchor(raw)
		if err != nil {
			return false, err
		}
		anchor = &parsed
	}
	entry, ok := behaviorTransitions()[key]
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnknownBehavior, key)
	}
	return anchor == nil || !entryDate(&entry).After(anchor.Date), nil
}
