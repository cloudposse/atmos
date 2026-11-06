package artifact

import "context"

//go:generate mockgen -package artifact -destination mock_service.go github.com/cloudposse/atmos/pkg/artifact Service

// Service is the repository-independent artifact API consumed by commands, steps, hooks, and CI
// adapters. This package defines only the interface and its request and result types; the
// implementation (preparation, packaging, conflict policy, retries) lives in the service package.
//
// Every method takes a context first and a typed request struct, and returns a typed result plus
// an error wrapping the sentinels in errors/errors.go. Display format never changes semantics.
type Service interface {
	// Publish prepares the source and stores it in one repository.
	Publish(ctx context.Context, request PublishRequest) (*OperationResult, error)

	// Pull materializes an artifact into a local directory after verifying every consumed descriptor.
	Pull(ctx context.Context, request PullRequest) (*OperationResult, error)

	// Copy transfers an artifact between repositories, evaluating both source and destination policy.
	Copy(ctx context.Context, request CopyRequest) (*OperationResult, error)

	// Inspect returns the metadata view without downloading the payload.
	Inspect(ctx context.Context, request InspectRequest) (*InspectResult, error)

	// List enumerates artifacts across the selected repositories.
	List(ctx context.Context, request ListRequest) (*ListResult, error)

	// Search filters enumerated artifacts by text.
	Search(ctx context.Context, request SearchRequest) (*ListResult, error)

	// Delete removes an artifact within the repository's declared delete capability.
	Delete(ctx context.Context, request DeleteRequest) (*OperationResult, error)

	// ShowDocument reads one README or CHANGELOG without downloading the payload.
	ShowDocument(ctx context.Context, request ShowDocumentRequest) (*DocumentResult, error)

	// Sign adds signature evidence to a digest reference.
	Sign(ctx context.Context, request SignRequest) (*SignResult, error)

	// Verify requires a trusted valid signature for the artifact.
	Verify(ctx context.Context, request VerifyRequest) (*VerifyResult, error)

	// Pin returns an immutable reference for consumer lockfiles.
	Pin(ctx context.Context, request PinRequest) (*PinResult, error)
}
