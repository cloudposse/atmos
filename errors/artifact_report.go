package errors

import "errors"

// Artifact report errors. Reporting is best-effort: these sentinels describe problems rendering or
// writing the CI summary and outputs of artifact operations. They are logged by the execution
// boundary and never change the exit status of the artifact operation itself.
var (
	// ErrArtifactReportRender is returned when the artifact summary template cannot be loaded or rendered.
	ErrArtifactReportRender = errors.New("failed to render the artifact report")

	// ErrArtifactReportWrite is returned when the artifact summary or CI outputs cannot be written.
	ErrArtifactReportWrite = errors.New("failed to write the artifact report")
)
