package layout

import (
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// State is the publication state of a reference.
type State string

// Reference states. A pending reference is unavailable to managed readers and absent from
// completed search results. The completed state is written last in the publication sequence.
const (
	// StatePending marks a reference whose publication started and has not completed. It stays
	// pending after an interruption until the same intent retries and repairs it.
	StatePending State = "pending"

	// StateCompleted marks a reference whose Record, payload, and documents are all in place.
	StateCompleted State = "completed"
)

// Reference is the stable-root reference record: it binds a name and version to the digest of the
// immutable Record. It is the only mutable document readers consult, and it is replaced
// atomically. Generation increases on every state change so a reader can detect that the
// reference moved while it was reading from a mutable payload location.
type Reference struct {
	// SchemaVersion is [SchemaVersion].
	SchemaVersion int `json:"schema_version"`

	// Name is the artifact name.
	Name string `json:"name"`

	// Version is the artifact version.
	Version string `json:"version"`

	// State is pending or completed.
	State State `json:"state"`

	// Generation increases by one on every state change; the first record has generation 1.
	Generation int64 `json:"generation"`

	// OperationID is the operation that wrote this state: the pending operation, or the operation
	// that completed the publication.
	OperationID string `json:"operation_id"`

	// IntentDigest is the intent digest of the publication that wrote this state.
	IntentDigest digest.Digest `json:"intent_digest"`

	// RecordDigest is the digest of the canonical Record. It is required when completed and empty
	// while a first publication is pending.
	RecordDigest digest.Digest `json:"record_digest,omitempty"`

	// PriorRecordDigest is the Record this pending publication replaces, kept for recovery
	// diagnostics. It is empty for a first publication and once completed.
	PriorRecordDigest digest.Digest `json:"prior_record_digest,omitempty"`
}

// Available reports whether managed readers may use the reference.
func (r *Reference) Available() bool {
	defer perf.Track(nil, "layout.Reference.Available")()

	return r.State == StateCompleted
}

// Key returns the reference key.
func (r *Reference) Key() string {
	defer perf.Track(nil, "layout.Reference.Key")()

	return ReferenceKey(r.Name, r.Version)
}

// Validate checks the reference's shape and state invariants.
func (r *Reference) Validate() error {
	defer perf.Track(nil, "layout.Reference.Validate")()

	if err := r.validateIdentity(); err != nil {
		return err
	}
	if err := r.validateState(); err != nil {
		return err
	}
	return validateOptionalDigests(r.RecordDigest, r.PriorRecordDigest)
}

func (r *Reference) validateIdentity() error {
	if r.SchemaVersion != SchemaVersion {
		return invalid(kindReference, "unsupported schema_version %d", r.SchemaVersion)
	}
	if err := artifact.ValidateName(r.Name); err != nil {
		return err
	}
	if err := artifact.ValidateVersion(r.Version); err != nil {
		return err
	}
	if r.Generation < 1 || r.OperationID == "" {
		return invalid(kindReference, "generation and operation_id are required")
	}
	return artifact.ValidateDigest(r.IntentDigest)
}

func (r *Reference) validateState() error {
	switch r.State {
	case StateCompleted:
		if r.RecordDigest == "" || r.PriorRecordDigest != "" {
			return invalid(kindReference, "a completed reference needs record_digest and no prior_record_digest")
		}
	case StatePending:
		if r.RecordDigest != "" {
			return invalid(kindReference, "a pending reference must not name a record_digest")
		}
	default:
		return invalid(kindReference, "unsupported state %q", r.State)
	}
	return nil
}

func validateOptionalDigests(digests ...digest.Digest) error {
	for _, d := range digests {
		if d == "" {
			continue
		}
		if err := artifact.ValidateDigest(d); err != nil {
			return err
		}
	}
	return nil
}

// NewPendingReference returns the pending state that replaces prior (nil for a first publication).
func NewPendingReference(name, version, operationID string, intent digest.Digest, prior *Reference) Reference {
	defer perf.Track(nil, "layout.NewPendingReference")()

	pending := Reference{
		SchemaVersion: SchemaVersion, Name: name, Version: version, State: StatePending,
		Generation: 1, OperationID: operationID, IntentDigest: intent,
	}
	if prior != nil {
		pending.Generation = prior.Generation + 1
		if prior.State == StateCompleted {
			pending.PriorRecordDigest = prior.RecordDigest
		} else {
			pending.PriorRecordDigest = prior.PriorRecordDigest
		}
	}
	return pending
}

// Completed returns the completed state that follows a pending reference once the Record,
// payload, and documents are in place.
func (r *Reference) Completed(recordDigest digest.Digest) Reference {
	defer perf.Track(nil, "layout.Reference.Completed")()

	done := *r
	done.State = StateCompleted
	done.Generation = r.Generation + 1
	done.RecordDigest = recordDigest
	done.PriorRecordDigest = ""
	return done
}

// Marshal returns the canonical bytes of the reference.
func (r *Reference) Marshal() ([]byte, error) {
	defer perf.Track(nil, "layout.Reference.Marshal")()

	return encode(r, MaxDocumentBytes)
}

// DecodeReference strictly parses a stored reference record.
func DecodeReference(raw []byte) (*Reference, error) {
	defer perf.Track(nil, "layout.DecodeReference")()

	var ref Reference
	if err := decode(kindReference, raw, &ref, MaxDocumentBytes); err != nil {
		return nil, err
	}
	return &ref, nil
}

// Representation is the mutable storage description of one reference's current Record. It holds
// the resolved payload location rendered once at publication; later reads and deletes use it and
// never render the key_pattern again.
type Representation struct {
	// SchemaVersion is [SchemaVersion].
	SchemaVersion int `json:"schema_version"`

	// RepositoryKind is the repository kind that wrote it, for example "local/archive".
	RepositoryKind string `json:"repository_kind"`

	// Name is the artifact name.
	Name string `json:"name"`

	// Version is the artifact version.
	Version string `json:"version"`

	// RecordDigest is the digest of the Record this representation belongs to.
	RecordDigest digest.Digest `json:"record_digest"`

	// OperationID is the operation that wrote the representation.
	OperationID string `json:"operation_id"`

	// Packaging is the payload packaging.
	Packaging artifact.Packaging `json:"packaging"`

	// Location is the resolved payload location, relative to the repository root.
	Location string `json:"location"`

	// Payload lists the payload blob descriptors: one archive, or one per file.
	Payload []ocispec.Descriptor `json:"payload"`

	// Documents lists the document blob descriptors stored alongside the payload.
	Documents []ocispec.Descriptor `json:"documents"`

	// RepresentationDigest is the checksum of the archive, or of the per-file payload digests.
	RepresentationDigest digest.Digest `json:"representation_digest,omitempty"`
}

// Validate checks the representation's shape.
func (r *Representation) Validate() error {
	defer perf.Track(nil, "layout.Representation.Validate")()

	if r.SchemaVersion != SchemaVersion {
		return invalid(kindRepresentation, "unsupported schema_version %d", r.SchemaVersion)
	}
	if err := validateNameVersion(r.Name, r.Version); err != nil {
		return err
	}
	if err := artifact.ValidateDigest(r.RecordDigest); err != nil {
		return err
	}
	if r.RepositoryKind == "" || r.OperationID == "" {
		return invalid(kindRepresentation, "repository_kind and operation_id are required")
	}
	if _, err := CleanLocation(r.Location); err != nil || r.Location != cleanedOrSelf(r.Location) {
		return invalid(kindRepresentation, "location %q is not a canonical relative location", r.Location)
	}
	if r.Payload == nil || r.Documents == nil {
		return invalid(kindRepresentation, "payload and documents lists are required (use empty lists)")
	}
	return validateOptionalDigests(r.RepresentationDigest)
}

// validateNameVersion checks the name and version of a document.
func validateNameVersion(name, version string) error {
	if err := artifact.ValidateName(name); err != nil {
		return err
	}
	return artifact.ValidateVersion(version)
}

// cleanedOrSelf returns the canonical form of location, or location itself when it is invalid.
func cleanedOrSelf(location string) string {
	if cleaned, err := CleanLocation(location); err == nil {
		return cleaned
	}
	return location
}

// Marshal returns the canonical bytes of the representation.
func (r *Representation) Marshal() ([]byte, error) {
	defer perf.Track(nil, "layout.Representation.Marshal")()

	return encode(r, MaxDocumentBytes)
}

// DecodeRepresentation strictly parses a stored representation record.
func DecodeRepresentation(raw []byte) (*Representation, error) {
	defer perf.Track(nil, "layout.DecodeRepresentation")()

	var rep Representation
	if err := decode(kindRepresentation, raw, &rep, MaxDocumentBytes); err != nil {
		return nil, err
	}
	return &rep, nil
}

// Artifact converts the stored description into the repository-neutral [artifact.Representation].
// The locator argument is the display locator of the payload, for example an absolute path or an
// s3:// URI.
func (r *Representation) Artifact(locator string) artifact.Representation {
	defer perf.Track(nil, "layout.Representation.Artifact")()

	return artifact.Representation{
		Packaging:            r.Packaging,
		Payload:              append([]ocispec.Descriptor(nil), r.Payload...),
		Documents:            append([]ocispec.Descriptor(nil), r.Documents...),
		Locator:              locator,
		RecordDigest:         r.RecordDigest,
		RepresentationDigest: r.RepresentationDigest,
	}
}

// Prior describes the state an operation replaces.
type Prior struct {
	// RecordDigest is the replaced Record.
	RecordDigest digest.Digest `json:"record_digest"`

	// Location is where the replaced payload lives, so a recovery can retire it.
	Location string `json:"location,omitempty"`

	// Packaging is the packaging of the replaced payload, for backends such as S3 whose object
	// keys depend on it. Local backends leave it empty.
	Packaging artifact.Packaging `json:"packaging,omitempty"`
}

// Operation is the publication journal: an implementation recovery record, not a public catalog.
// It is written before any payload path changes and records everything a same-intent retry needs
// to resume or repair the publication: the captured time, the intent, the prior reference, the
// resolved location, and the prepared representation checksums.
type Operation struct {
	// SchemaVersion is [SchemaVersion].
	SchemaVersion int `json:"schema_version"`

	// RepositoryKind is the repository kind performing the publication.
	RepositoryKind string `json:"repository_kind"`

	// OperationID is the logical operation identifier, stable across retries.
	OperationID string `json:"operation_id"`

	// Name is the artifact name.
	Name string `json:"name"`

	// Version is the artifact version.
	Version string `json:"version"`

	// IntentDigest is the intent digest of the artifact being published.
	IntentDigest digest.Digest `json:"intent_digest"`

	// RecordDigest is the digest of the Record being published.
	RecordDigest digest.Digest `json:"record_digest"`

	// CapturedAt is the creation time captured once for the publication.
	CapturedAt artifact.Timestamp `json:"captured_at"`

	// Packaging is the payload packaging being written.
	Packaging artifact.Packaging `json:"packaging"`

	// Location is the resolved payload location reserved for this operation.
	Location string `json:"location"`

	// ExpectedPrior is the record digest the writer observed under the guard; empty means none.
	ExpectedPrior digest.Digest `json:"expected_prior,omitempty"`

	// Prior describes the state being replaced, if any.
	Prior *Prior `json:"prior,omitempty"`

	// Payload and Documents are the prepared representation checksums.
	Payload   []ocispec.Descriptor `json:"payload"`
	Documents []ocispec.Descriptor `json:"documents"`

	// Owner identifies the writer for recovery diagnostics, for example host and process.
	Owner string `json:"owner,omitempty"`
}

// Validate checks the journal's shape.
func (o *Operation) Validate() error {
	defer perf.Track(nil, "layout.Operation.Validate")()

	if o.SchemaVersion != SchemaVersion {
		return invalid(kindOperation, "unsupported schema_version %d", o.SchemaVersion)
	}
	if o.OperationID == "" || o.RepositoryKind == "" {
		return invalid(kindOperation, "operation_id and repository_kind are required")
	}
	if err := validateNameVersion(o.Name, o.Version); err != nil {
		return err
	}
	if err := o.validateState(); err != nil {
		return err
	}
	return validateOptionalDigests(o.IntentDigest, o.RecordDigest, o.ExpectedPrior)
}

func (o *Operation) validateState() error {
	if o.CapturedAt.IsZero() {
		return invalid(kindOperation, "captured_at is required")
	}
	if _, err := CleanLocation(o.Location); err != nil {
		return invalid(kindOperation, "location %q is not a valid relative location", o.Location)
	}
	if o.Payload == nil || o.Documents == nil {
		return invalid(kindOperation, "payload and documents lists are required (use empty lists)")
	}
	if o.Prior != nil && o.Prior.RecordDigest == "" {
		return invalid(kindOperation, "prior needs a record_digest")
	}
	return nil
}

// Claim returns the paths the operation reserves.
func (o *Operation) Claim() Claim {
	defer perf.Track(nil, "layout.Operation.Claim")()

	return Claim{
		ReferenceKey: ReferenceKey(o.Name, o.Version), Name: o.Name, Version: o.Version,
		OperationID: o.OperationID, Paths: PayloadPaths(o.RepositoryKind, o.Location),
	}
}

// Marshal returns the canonical bytes of the journal.
func (o *Operation) Marshal() ([]byte, error) {
	defer perf.Track(nil, "layout.Operation.Marshal")()

	return encode(o, MaxDocumentBytes)
}

// DecodeOperation strictly parses a stored operation journal.
func DecodeOperation(raw []byte) (*Operation, error) {
	defer perf.Track(nil, "layout.DecodeOperation")()

	var op Operation
	if err := decode(kindOperation, raw, &op, MaxDocumentBytes); err != nil {
		return nil, err
	}
	return &op, nil
}

// Claim returns the paths a completed representation owns.
func (r *Representation) Claim() Claim {
	defer perf.Track(nil, "layout.Representation.Claim")()

	return Claim{
		ReferenceKey: ReferenceKey(r.Name, r.Version), Name: r.Name, Version: r.Version,
		Paths: PayloadPaths(r.RepositoryKind, r.Location),
	}
}
