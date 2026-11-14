package artifact

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CapabilityError reports an operation that a repository cannot honor. It is never a successful
// no-op: callers receive it instead of silently degraded behavior. It wraps a sentinel
// (ErrArtifactUnsupportedCapability by default, or ErrArtifactPublicationUnavailable) and an
// optional cause, so errors.Is works against both.
type CapabilityError struct {
	// Repository is the repository alias or kind that lacks the capability.
	Repository string

	// Operation is the attempted operation, for example "lock" or "sign".
	Operation string

	// Capability names the missing guarantee, for example "lock/repository" or "enumerate/collection".
	Capability string

	// Reason explains the limitation and, where useful, the supported alternative.
	Reason string

	// Sentinel is the wrapped sentinel; nil means ErrArtifactUnsupportedCapability.
	Sentinel error

	// Cause is an optional underlying error kept for diagnostics.
	Cause error
}

// NewUnsupportedCapability returns a CapabilityError wrapping ErrArtifactUnsupportedCapability.
func NewUnsupportedCapability(repository, operation, capability, reason string) *CapabilityError {
	defer perf.Track(nil, "artifact.NewUnsupportedCapability")()

	return &CapabilityError{
		Repository: repository,
		Operation:  operation,
		Capability: capability,
		Reason:     reason,
		Sentinel:   errUtils.ErrArtifactUnsupportedCapability,
	}
}

// NewPublicationUnavailable returns a CapabilityError wrapping ErrArtifactPublicationUnavailable.
func NewPublicationUnavailable(repository, operation, reason string, cause error) *CapabilityError {
	defer perf.Track(nil, "artifact.NewPublicationUnavailable")()

	return &CapabilityError{
		Repository: repository,
		Operation:  operation,
		Reason:     reason,
		Sentinel:   errUtils.ErrArtifactPublicationUnavailable,
		Cause:      cause,
	}
}

// Error implements error.
func (e *CapabilityError) Error() string {
	defer perf.Track(nil, "artifact.CapabilityError.Error")()

	message := e.sentinel().Error()
	if e.Repository != "" {
		message = fmt.Sprintf("%s: repository %q", message, e.Repository)
	}
	if e.Operation != "" {
		message = fmt.Sprintf("%s: %s", message, e.Operation)
	}
	if e.Capability != "" {
		message = fmt.Sprintf("%s: capability %s", message, e.Capability)
	}
	if e.Reason != "" {
		message = fmt.Sprintf("%s: %s", message, e.Reason)
	}
	return message
}

// Unwrap exposes the sentinel and the cause to errors.Is and errors.As.
func (e *CapabilityError) Unwrap() []error {
	defer perf.Track(nil, "artifact.CapabilityError.Unwrap")()

	if e.Cause == nil {
		return []error{e.sentinel()}
	}
	return []error{e.sentinel(), e.Cause}
}

func (e *CapabilityError) sentinel() error {
	if e.Sentinel != nil {
		return e.Sentinel
	}
	return errUtils.ErrArtifactUnsupportedCapability
}

// AsCapabilityError extracts a *CapabilityError from err's chain.
func AsCapabilityError(err error) (*CapabilityError, bool) {
	defer perf.Track(nil, "artifact.AsCapabilityError")()

	var target *CapabilityError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
