package errors

import "errors"

// Artifact step errors. The step handler (type: artifact) reports configuration problems with these
// sentinels; the artifact operations themselves keep returning the ErrArtifact* family.
var (
	// ErrArtifactStepInvalidAction is returned when an artifact step names an unknown or missing action.
	ErrArtifactStepInvalidAction = errors.New("invalid artifact step action")

	// ErrArtifactStepFieldRequired is returned when an artifact step omits a field its action requires.
	ErrArtifactStepFieldRequired = errors.New("artifact step is missing a required field")

	// ErrArtifactStepFieldForbidden is returned when an artifact step sets a field its action does not accept.
	ErrArtifactStepFieldForbidden = errors.New("artifact step field is not valid for the action")

	// ErrArtifactStepInvalidSource is returned when an artifact step source is not a plain string path.
	ErrArtifactStepInvalidSource = errors.New("artifact step source must be a string")

	// ErrArtifactStepInvalidField is returned when an artifact step field has the wrong shape or value.
	ErrArtifactStepInvalidField = errors.New("invalid artifact step field")
)
