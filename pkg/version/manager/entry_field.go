package manager

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrUnsupportedEntryField is returned when Field is called with an unknown field name.
var ErrUnsupportedEntryField = errUtils.ErrUnsupportedVersionField

// entryFieldAccessors maps the yaml/json tag names of EffectiveEntry to accessor
// functions, keeping Field() a flat, low-complexity lookup rather than a large switch.
var entryFieldAccessors = map[string]func(*EffectiveEntry) any{
	"name":       func(e *EffectiveEntry) any { return e.Name },
	"ecosystem":  func(e *EffectiveEntry) any { return e.Ecosystem },
	"datasource": func(e *EffectiveEntry) any { return e.Datasource },
	"provider":   func(e *EffectiveEntry) any { return e.Provider },
	"package":    func(e *EffectiveEntry) any { return e.Package },
	"desired":    func(e *EffectiveEntry) any { return e.Desired },
	"group":      func(e *EffectiveEntry) any { return e.Group },
	"update":     func(e *EffectiveEntry) any { return e.Update },
	"include":    func(e *EffectiveEntry) any { return e.Include },
	"exclude":    func(e *EffectiveEntry) any { return e.Exclude },
	"prerelease": func(e *EffectiveEntry) any { return e.Prerelease },
	"labels":     func(e *EffectiveEntry) any { return e.Labels },
	"locked":     func(e *EffectiveEntry) any { return e.Locked },
}

// Field returns the value of a single field on the entry, keyed by the same
// names used in its yaml/json tags. Used by `version track get --show` to
// print one value instead of the full entry.
func (e *EffectiveEntry) Field(name string) (any, error) {
	defer perf.Track(nil, "manager.EffectiveEntry.Field")()

	accessor, ok := entryFieldAccessors[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedEntryField, name)
	}
	return accessor(e), nil
}
