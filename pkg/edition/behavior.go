package edition

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
)

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
	for _, entry := range Journal() {
		if entry.Kind == KindBehavior && entry.Key == key {
			return anchor == nil || !entryDate(&entry).After(anchor.Date), nil
		}
	}
	return false, fmt.Errorf("%w: %q", ErrUnknownBehavior, key)
}
