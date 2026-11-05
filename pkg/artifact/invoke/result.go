package invoke

import (
	"fmt"

	"github.com/opencontainers/go-digest"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Result is the structured result of one operation. The operation fields flatten into the top
// level with their named-output spelling (ref, location, resolved_digest, record_digest,
// content_digest, representation_digest, size, outcome), and the action-specific parts follow.
// JSON and YAML share these names; the display format never changes semantics. The CLI prints it
// and a step stores it under StepResult.Metadata["artifact"].
type Result struct {
	artifact.OperationResult

	// Error is set when the operation failed. The result is still reported, and the exit status is nonzero.
	Error string `json:"error,omitempty"`

	// Record is the sealed Record of a publication (set for dry runs and publications).
	Record *artifact.Record `json:"record,omitempty"`

	// Inspection is the metadata view of an inspected artifact.
	Inspection *artifact.Inspection `json:"inspection,omitempty"`

	// Verification is the authenticated verification result; absent means unchecked.
	Verification *artifact.VerificationResult `json:"verification,omitempty"`

	// Subject is the signed or verified subject.
	Subject *artifact.SignatureSubject `json:"subject,omitempty"`

	// Evidence lists the evidence stored by sign.
	Evidence []artifact.SignatureEvidence `json:"evidence,omitempty"`

	// Document is the descriptor of a shown document.
	Document *artifact.Document `json:"document,omitempty"`

	// Content is the document text of show.
	Content *string `json:"content,omitempty"`
}

// MarkFailed fills the failure markers of a result. The service leaves the outcome empty on
// failure so a failure is never mistaken for success; the reported result states it explicitly.
// The requested values fill in what the service could not report, for example when the failure
// happened before the operation began.
func (r *Result) MarkFailed(err error, action artifact.Action, repository, requested string) {
	defer perf.Track(nil, "invoke.Result.MarkFailed")()

	if err == nil {
		return
	}
	r.Error = err.Error()
	r.Outcome = artifact.OutcomeFailed
	if r.Action == "" {
		r.Action = action
	}
	if r.Repository == "" {
		r.Repository = repository
	}
	if r.Requested == "" {
		r.Requested = requested
	}
}

// Collection is the structured result of list and search: the `artifacts` array plus
// per-repository outcomes, and the error when the query failed.
type Collection struct {
	// Artifacts are the entries across the selected repositories, sorted lexically.
	Artifacts []Listed `json:"artifacts"`

	// Repositories reports per-repository outcomes.
	Repositories []artifact.RepositoryListing `json:"repositories,omitempty"`

	// Incomplete is true when a selected repository could not be listed, so Artifacts is not the
	// whole collection. The failing repository carries an error diagnostic.
	Incomplete bool `json:"incomplete,omitempty"`

	// Error is set when the query failed.
	Error string `json:"error,omitempty"`
}

// Listed is one entry of the `artifacts` array. It carries the named outputs of an operation
// result (ref, resolved_digest, record_digest, content_digest) next to the listing fields, plus the
// pinned reference a consumer lockfile persists.
type Listed struct {
	Repository     string                   `json:"repository,omitempty"`
	Ref            string                   `json:"ref"`
	Name           string                   `json:"name"`
	Version        string                   `json:"version"`
	Kind           string                   `json:"kind"`
	Description    string                   `json:"description,omitempty"`
	Labels         map[string]string        `json:"labels,omitempty"`
	Documents      []artifact.Document      `json:"documents,omitempty"`
	ResolvedDigest digest.Digest            `json:"resolved_digest,omitempty"`
	RecordDigest   digest.Digest            `json:"record_digest,omitempty"`
	ContentDigest  digest.Digest            `json:"content_digest,omitempty"`
	Pinned         artifact.PinnedReference `json:"pinned"`
}

// ListedFrom converts service entries into collection entries. The result is never nil, so an
// empty listing encodes as an empty array.
func ListedFrom(entries []artifact.ListEntry) []Listed {
	defer perf.Track(nil, "invoke.ListedFrom")()

	out := make([]Listed, 0, len(entries))
	for index := range entries {
		entry := &entries[index]
		out = append(out, Listed{
			Repository:     entry.Repository,
			Ref:            artifact.Ref{Name: entry.Name, Version: entry.Version}.String(),
			Name:           entry.Name,
			Version:        entry.Version,
			Kind:           entry.Kind,
			Description:    entry.Description,
			Labels:         entry.Labels,
			Documents:      entry.Documents,
			ResolvedDigest: entry.Pinned.ResolvedDigest,
			RecordDigest:   entry.Pinned.RecordDigest,
			ContentDigest:  entry.Pinned.ContentDigest,
			Pinned:         entry.Pinned,
		})
	}
	return out
}

// Pair is one named output.
type Pair struct {
	// Name is the output name.
	Name string

	// Value is the output text.
	Value string
}

// Named output names. These are the JSON field names of [artifact.OperationResult] too.
const (
	OutputOutcome              = "outcome"
	OutputRef                  = "ref"
	OutputLocation             = "location"
	OutputResolvedDigest       = "resolved_digest"
	OutputRecordDigest         = "record_digest"
	OutputContentDigest        = "content_digest"
	OutputRepresentationDigest = "representation_digest"
	OutputSize                 = "size"
)

// OutputPairs returns the named outputs of an operation in a stable order. An optional value that
// the operation did not produce is omitted, never filled with a misleading empty string.
func OutputPairs(op *artifact.OperationResult) []Pair {
	defer perf.Track(nil, "invoke.OutputPairs")()

	candidates := []Pair{
		{OutputOutcome, string(op.Outcome)},
		{OutputRef, op.Ref},
		{OutputLocation, op.Location},
		{OutputResolvedDigest, string(op.ResolvedDigest)},
		{OutputRecordDigest, string(op.RecordDigest)},
		{OutputContentDigest, string(op.ContentDigest)},
		{OutputRepresentationDigest, string(op.RepresentationDigest)},
	}
	if op.Size > 0 {
		candidates = append(candidates, Pair{OutputSize, fmt.Sprintf("%d", op.Size)})
	}
	pairs := make([]Pair, 0, len(candidates))
	for _, pair := range candidates {
		if pair.Value != "" {
			pairs = append(pairs, pair)
		}
	}
	return pairs
}

// NamedOutputs is [OutputPairs] as a map, the shape of StepResult.Outputs.
func NamedOutputs(op *artifact.OperationResult) map[string]string {
	defer perf.Track(nil, "invoke.NamedOutputs")()

	pairs := OutputPairs(op)
	outputs := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		outputs[pair.Name] = pair.Value
	}
	return outputs
}

// Outcome is the result of [Run]: a Result for single-artifact actions, or a Collection for list
// and search. Exactly one is set.
type Outcome struct {
	// Result is set for every action except list and search.
	Result *Result

	// Collection is set for list and search.
	Collection *Collection
}

// Payload returns the typed value that is reported as JSON or stored in step metadata.
func (o *Outcome) Payload() any {
	defer perf.Track(nil, "invoke.Outcome.Payload")()

	if o.Collection != nil {
		return o.Collection
	}
	return o.Result
}
