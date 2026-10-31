package report

import (
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/perf"
)

//go:generate mockgen -destination=mock_collector.go -package=report github.com/cloudposse/atmos/pkg/artifact/report Collector

// Entry is one completed or failed operation attempt, recorded at an execution boundary.
type Entry struct {
	// Step is the name of the step that ran the operation, or empty outside a step.
	Step string

	// OperationID is the logical operation identifier. Every retry attempt of one operation
	// carries the same ID, so a renderer deduplicates by it and keeps the final attempt.
	OperationID string

	// Attempt counts the attempts of this logical operation, starting at one.
	Attempt int

	// Outcome is the typed outcome of single-artifact actions. It is nil for list and search.
	Outcome *invoke.Outcome

	// Err is the operation error, or nil on success. A failed attempt still carries the partial
	// result in Outcome.
	Err error
}

// Collector receives the entries that artifact operations produce. Implementations must be safe
// for concurrent use: steps in a parallel group record concurrently.
type Collector interface {
	// Record stores one entry. It must not block the operation and must not fail it: reporting
	// problems are the collector's own to log.
	Record(entry *Entry)
}

// Discard is the default collector. It ignores every entry, so an unconfigured runner pays nothing.
type Discard struct{}

// Record implements [Collector].
func (Discard) Record(*Entry) {
	defer perf.Track(nil, "report.Discard.Record")()
}

var _ Collector = Discard{}
