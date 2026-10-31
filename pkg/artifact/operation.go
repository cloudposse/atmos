package artifact

import (
	"time"

	"github.com/opencontainers/go-digest"
)

// Action names an artifact operation.
type Action string

// Artifact operations.
const (
	ActionPublish Action = "publish"
	ActionPull    Action = "pull"
	ActionCopy    Action = "copy"
	ActionInspect Action = "inspect"
	ActionList    Action = "list"
	ActionSearch  Action = "search"
	ActionDelete  Action = "delete"
	ActionShow    Action = "show"
	ActionSign    Action = "sign"
	ActionVerify  Action = "verify"
	ActionPin     Action = "pin"
)

// Outcome is the result classification of an operation.
type Outcome string

// Operation outcomes. A no-op returns the existing Record unchanged.
const (
	// OutcomeCreated means new state was written.
	OutcomeCreated Outcome = "created"

	// OutcomeUnchanged means the same intent already existed and nothing was written.
	OutcomeUnchanged Outcome = "unchanged"

	// OutcomeReplaced means existing state was replaced under force.
	OutcomeReplaced Outcome = "replaced"

	// OutcomeDeleted means state was removed.
	OutcomeDeleted Outcome = "deleted"

	// OutcomeRead means state was read or materialized without mutation.
	OutcomeRead Outcome = "read"

	// OutcomeDryRun means the operation was validated and planned but nothing was written.
	OutcomeDryRun Outcome = "dry-run"

	// OutcomeFailed means the operation did not complete. The service leaves the outcome empty on
	// failure so that a failure is never mistaken for success; command envelopes report this value.
	OutcomeFailed Outcome = "failed"
)

// Severity classifies a diagnostic.
type Severity string

// Diagnostic severities.
const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Diagnostic is a structured, non-fatal message attached to a result.
type Diagnostic struct {
	// Severity is info, warning, or error.
	Severity Severity `json:"severity"`

	// Code is a stable machine-readable identifier.
	Code string `json:"code,omitempty"`

	// Message is the human-readable text. It never contains credentials.
	Message string `json:"message"`
}

// OperationResult is the typed result of one operation. Its JSON field names match the named step
// outputs and structured CLI output; optional values are omitted rather than filled with
// misleading empty strings.
type OperationResult struct {
	// OperationID is the logical operation identifier.
	OperationID string `json:"operation_id"`

	// Action is the operation performed.
	Action Action `json:"action"`

	// Outcome classifies the result.
	Outcome Outcome `json:"outcome"`

	// Repository is the configured repository alias the operation targeted.
	Repository string `json:"repository,omitempty"`

	// Requested is the reference as requested by the caller.
	Requested string `json:"requested_ref,omitempty"`

	// Ref is the resolved reference.
	Ref string `json:"ref,omitempty"`

	// Location is the primary resolved storage location.
	Location string `json:"location,omitempty"`

	// Aliases lists additional locations or tags that resolve to the same content.
	Aliases []string `json:"aliases,omitempty"`

	// ResolvedDigest is the OCI manifest/index digest, or the record digest for local and S3.
	ResolvedDigest digest.Digest `json:"resolved_digest,omitempty"`

	// RecordDigest is the digest of the canonical Record bytes.
	RecordDigest digest.Digest `json:"record_digest,omitempty"`

	// ContentDigest is the portable content digest from the Record.
	ContentDigest digest.Digest `json:"content_digest,omitempty"`

	// RepresentationDigest is the checksum of the transferred payload representation.
	RepresentationDigest digest.Digest `json:"representation_digest,omitempty"`

	// Size is the transferred or described payload size in bytes.
	Size int64 `json:"size,omitempty"`

	// Duration is the wall-clock duration of the operation.
	Duration time.Duration `json:"duration_ns,omitempty"`

	// LockScope is the actual lock scope that coordinated a mutation.
	LockScope LockScope `json:"lock_scope,omitempty"`

	// Diagnostics are non-fatal messages.
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}
