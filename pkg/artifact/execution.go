package artifact

import "time"

// Execution carries the per-call execution settings that a wrapper (a workflow runner, a custom
// command, or a hook) supplies when it owns the retry policy. The zero value is the behavior of a
// standalone CLI call: the service generates a fresh operation identity and applies its own
// transport retry budget.
//
// A wrapper that retries a whole operation captures one Execution before the first attempt and
// passes the same value on every attempt. The operation ID and the UTC preparation time then stay
// stable, so a publication keeps its creation time and a report can deduplicate attempts by the
// logical operation ID. The wrapper also sets [Execution.MaxAttempts] to 1, so the wrapper's retry
// budget and the service's transport retries never multiply.
type Execution struct {
	// OperationID is the logical operation identifier. Empty means the service generates one.
	OperationID string

	// PreparedAt is the single UTC preparation time. Zero means the service reads its clock.
	PreparedAt time.Time

	// MaxAttempts overrides the number of transport attempts for this call. Zero keeps the
	// service default. One disables transport retries.
	MaxAttempts int
}
