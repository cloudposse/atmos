package report

// Report is the template context of the frame template (ci.templates.artifacts.summary). Every
// string is already escaped Markdown: a template prints the fields as they are and never needs to
// escape them. Strings that are code spans, such as references and digests, are wrapped in
// backticks already.
type Report struct {
	// Heading is the report heading text.
	Heading string

	// Overview is a one-line count of the operations and their outcomes.
	Overview string

	// Total is the number of operations in the report.
	Total int

	// Succeeded, Unchanged, Failed, and Canceled count the operations by result. An unchanged
	// operation succeeded without writing anything and is counted only as unchanged.
	Succeeded int
	Unchanged int
	Failed    int
	Canceled  int

	// Rows are the compact overview table rows, one per operation, in report order.
	Rows []Row

	// Sections are the rendered per-operation sections, in report order. When the size limit
	// forced operations out of the report, only the first Total-Omitted operations have a section.
	Sections []string

	// Omitted is the number of operation sections left out to fit the summary size limit.
	Omitted int

	// Notice is a visible note about a degraded report, such as omitted sections. It is empty
	// when the report is complete.
	Notice string
}

// Row is one row of the overview table.
type Row struct {
	// Number is the one-based position of the operation in the report.
	Number int

	// Operation is the action, for example Publish or Pull.
	Operation string

	// Icon is the status symbol, and Status the status wording, for example Published.
	Icon   string
	Status string

	// Artifact is the resolved reference as a code span, or the requested one when none resolved.
	Artifact string

	// Repository is the configured repository alias as a code span.
	Repository string

	// Destination is the primary resolved location as a code span, with credentials and query
	// strings removed.
	Destination string

	// Digest is the shortened primary digest with its type, for example `sha256:0123456789ab…` (resolved).
	Digest string

	// Size and Duration are human-readable values; empty when the operation has none.
	Size     string
	Duration string
}

// Operation is the template context of one per-operation section (ci.templates.artifacts.<action>).
// Every string is already escaped Markdown.
type Operation struct {
	// Number is the one-based position of the operation in the report.
	Number int

	// Step is the name of the step that ran the operation, or empty for a standalone command.
	Step string

	// Action is the action name, for example publish, and ActionLabel its title-cased form.
	Action      string
	ActionLabel string

	// Title is the section heading text: the action label and the reference.
	Title string

	// Icon, Status, and Kind classify the result. Kind is one of success, unchanged, dry-run,
	// unchecked, failed, or canceled.
	Icon   string
	Status string
	Kind   string

	// Summary is one sentence stating what happened.
	Summary string

	// Repository, Requested, Ref, and Destination are code spans. Requested is empty when it equals Ref.
	Repository  string
	Requested   string
	Ref         string
	Destination string

	// Aliases lists the additional locations or tags that resolve to the same content, as code spans.
	Aliases []string

	// Size and Duration are human-readable. ExactSize adds the byte count.
	Size      string
	ExactSize string
	Duration  string

	// Attempts is the number of attempts the final result took; one means no retry.
	Attempts int

	// Lock is the lock guarantee of a mutation, or nil when the operation did not coordinate one.
	Lock *Lock

	// Digests lists every digest the operation reported, each labelled with its type.
	Digests []Digest

	// Integrity says what the digests prove and what they do not.
	Integrity string

	// Signature is the signature subject, evidence, and verification result, or nil when the
	// operation neither signed nor verified.
	Signature *Signature

	// Error is the fenced error block of a failed operation.
	Error string

	// Diagnostics are the non-fatal messages of the operation, capped; DiagnosticsMore counts the rest.
	Diagnostics     []Diagnostic
	DiagnosticsMore int

	// Description and Labels come from the published Record.
	Description string
	Labels      []Label

	// Inventory lists the payload files of a published or inspected Record.
	Inventory *Inventory

	// Collection summarizes a list or search result.
	Collection *Collection
}

// Lock states the guarantee of the lock that coordinated a mutation.
type Lock struct {
	// Scope is the actual lock scope: host, repository, or none.
	Scope string

	// Guarantee says in a sentence what the scope does and does not exclude.
	Guarantee string
}

// Digest is one labelled digest.
type Digest struct {
	// Label names the digest type: Resolved, Record, Content, or Representation.
	Label string

	// Meaning says what the digest identifies.
	Meaning string

	// Value is the full digest as a code span, and Short its shortened form.
	Value string
	Short string
}

// Signature reports the signature facts of a sign or verify operation. Signed and verified are
// different claims: signing stores evidence, and only verification against a trust policy
// authenticates the signer.
type Signature struct {
	// Status is signed, verified, unchecked, or failed.
	Status string

	// Statement says in a sentence what the status does and does not prove.
	Statement string

	// Subject is the immutable thing the signature covers: its kind and digest.
	SubjectKind      string
	SubjectDigest    string
	SubjectMediaType string

	// Policy is the shortened fingerprint of the verification policy that was applied.
	Policy string

	// Claims are the verified signer identities.
	Claims []Claim

	// Evidence lists the signature evidence that was stored or considered.
	Evidence []Evidence
}

// Claim is a verified signer identity. Identity and Issuer are set for keyless signing and KeyID
// for key-based signing.
type Claim struct {
	Identity string
	Issuer   string
	KeyID    string
}

// Evidence is one evidence blob.
type Evidence struct {
	// Format is the evidence encoding, and Digest the evidence blob digest as a code span.
	Format string
	Digest string
}

// Diagnostic is one non-fatal message.
type Diagnostic struct {
	// Severity is info, warning, or error; Code may be empty.
	Severity string
	Code     string
	Message  string
}

// Label is one metadata label as code spans.
type Label struct {
	Key   string
	Value string
}

// Inventory lists payload files.
type Inventory struct {
	// Count is the number of entries in the Record, and Bytes their total size.
	Count int
	Bytes string

	// Files are the first entries; More counts the entries not listed.
	Files []File
	More  int
}

// File is one payload entry.
type File struct {
	// Path is a code span, and Size a human-readable size, empty for directories and symlinks.
	Path string
	Size string
}

// Collection summarizes a list or search.
type Collection struct {
	// Count is the number of artifacts returned.
	Count int

	// Incomplete is true when a repository could not be listed, so Count is not the whole collection.
	Incomplete bool

	// Repositories reports each repository's result.
	Repositories []RepositoryResult

	// Artifacts lists the first returned artifacts as code spans; More counts the rest.
	Artifacts []string
	More      int
}

// RepositoryResult is the listing outcome of one repository.
type RepositoryResult struct {
	// Repository is the alias as a code span, Count the entries it returned, and Source where they
	// came from: live, cache, or catalog.
	Repository string
	Count      int
	Source     string
}
