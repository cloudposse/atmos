package artifact

// InspectResult is the result of Service.Inspect.
type InspectResult struct {
	// Operation is the typed operation result stored under StepResult.Metadata["artifact"].
	Operation OperationResult `json:"operation"`

	// Inspection is the metadata view.
	Inspection Inspection `json:"inspection"`

	// Verification is set when policy required or constrained verification; nil means unchecked.
	Verification *VerificationResult `json:"verification,omitempty"`
}

// RepositoryListing reports the outcome of listing one repository, so a failing or incapable
// repository is explained rather than silently omitted.
type RepositoryListing struct {
	// Repository is the configured repository alias.
	Repository string `json:"repository"`

	// Count is the number of entries returned from the repository.
	Count int `json:"count"`

	// Diagnostics explain capability or refresh problems.
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// ListResult is the result of Service.List and Service.Search. Its JSON name for the entries is
// "artifacts".
type ListResult struct {
	// Artifacts are the entries across the selected repositories, sorted lexically.
	Artifacts []ListEntry `json:"artifacts"`

	// Repositories reports per-repository outcomes.
	Repositories []RepositoryListing `json:"repositories,omitempty"`
}

// DocumentResult is the result of Service.ShowDocument. Content is bounded by [MaxDocumentSize].
type DocumentResult struct {
	// Operation is the typed operation result.
	Operation OperationResult `json:"operation"`

	// Document is the descriptor from the Record.
	Document Document `json:"document"`

	// Content is the verified document bytes.
	Content []byte `json:"content"`
}

// SignResult is the result of Service.Sign.
type SignResult struct {
	// Operation is the typed operation result.
	Operation OperationResult `json:"operation"`

	// Subject is the signed subject.
	Subject SignatureSubject `json:"subject"`

	// Evidence lists the stored evidence descriptors.
	Evidence []SignatureEvidence `json:"evidence"`
}

// VerifyResult is the result of Service.Verify.
type VerifyResult struct {
	// Operation is the typed operation result.
	Operation OperationResult `json:"operation"`

	// Verification is the authenticated verification result.
	Verification VerificationResult `json:"verification"`
}

// PinResult is the result of Service.Pin.
type PinResult struct {
	// Operation is the typed operation result.
	Operation OperationResult `json:"operation"`

	// Pinned is the immutable reference.
	Pinned PinnedReference `json:"pinned"`
}
