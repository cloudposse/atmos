// Package freshness computes the checksum.changed/timestamp.changed/precondition.success/
// sources/artifacts facts consumed via the existing `when:` condition engine for a step's
// `inputs:`/`artifacts:`/`precondition:`, and persists sources-hash state across runs for
// checksum comparison. See pkg/condition for how these facts are exposed to `when:`.
package freshness

import "errors"

// ErrGlobInvalid is returned when an inputs.sources/artifacts.paths glob pattern is malformed
// (distinct from "matched nothing," which is not an error).
var ErrGlobInvalid = errors.New("invalid sources/artifacts glob pattern")
