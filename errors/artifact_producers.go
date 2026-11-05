package errors

import "errors"

// Artifact producer sentinels. They describe outcomes specific to the kind-specific producers
// (scaffold and component) layered over the generic files producer.
var (
	// ErrArtifactScaffoldInvalid is returned when a scaffold source fails scaffold configuration
	// validation. The wrapped cause names the offending file or field.
	ErrArtifactScaffoldInvalid = errors.New("invalid scaffold source")

	// ErrArtifactComponentInvalid is returned when a component source cannot be published, for
	// example when its component.yaml manifest is malformed.
	ErrArtifactComponentInvalid = errors.New("invalid component source")

	// ErrArtifactProducerUnavailable is returned when a producer needs a collaborator that the
	// running binary did not register, for example the scaffold configuration validator.
	ErrArtifactProducerUnavailable = errors.New("artifact producer is unavailable")
)

// ErrArtifactPayloadLayer is returned when an Atmos artifact package does not have exactly one
// payload layer for a materializer to extract.
var ErrArtifactPayloadLayer = errors.New("artifact package payload layer is missing or ambiguous")
