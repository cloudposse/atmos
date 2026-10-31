package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/opencontainers/go-digest"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
)

const (
	// The maxDiagnostics constant caps the diagnostics listed for one operation.
	maxDiagnostics = 20

	// The maxInventoryFiles constant caps the payload entries listed for one operation.
	maxInventoryFiles = 25

	// The maxListedArtifacts constant caps the artifacts listed for one list or search.
	maxListedArtifacts = 20

	// The maxLabels constant caps the labels listed for one operation.
	maxLabels = 20
)

// buildOperation turns one recorded entry into the template context of its section. It reads the
// typed result only: nothing here calls a service or repository.
func buildOperation(number int, entry *Entry) *Operation {
	result, collection := resultsOf(entry)
	op := &result.OperationResult
	action := op.Action
	if action == "" {
		action = entry.Action
	}
	outcome := op.Outcome
	if collection != nil && outcome == "" {
		outcome = artifact.OutcomeRead
	}
	word, kind := statusOf(action, outcome, result, collection, entry.Err)

	section := &Operation{
		Number:      number,
		Step:        plain(entry.Step),
		Action:      string(action),
		ActionLabel: actionLabel(action),
		Icon:        kindIcons[kind],
		Status:      word,
		Kind:        kind,
		Repository:  code(op.Repository),
		Ref:         code(op.Ref),
		Destination: code(op.Location),
		Size:        humanSize(op.Size),
		ExactSize:   exactSize(op.Size),
		Duration:    formatDuration(op.Duration),
		Attempts:    max(entry.Attempt, 1),
		Digests:     digestsOf(op),
		Aliases:     codes(op.Aliases),
	}
	if op.Requested != "" && op.Requested != op.Ref {
		section.Requested = code(op.Requested)
	}
	section.Title = titleOf(section, op)
	section.Lock = lockFor(action, op.LockScope, outcome, entry.Err != nil)
	fillDetails(section, entry, result, collection)
	return section
}

// resultsOf returns the single-artifact result and the collection of an entry. The result is never
// nil, so a failure before the service ran still reports its identity.
func resultsOf(entry *Entry) (*invoke.Result, *invoke.Collection) {
	var result *invoke.Result
	var collection *invoke.Collection
	if entry.Outcome != nil {
		result, collection = entry.Outcome.Result, entry.Outcome.Collection
	}
	if result == nil {
		result = &invoke.Result{}
	}
	return result, collection
}

// statusOf returns the status wording and kind of an operation. A listing that could not cover
// every repository is a success with a warning, never a plain success.
func statusOf(action artifact.Action, outcome artifact.Outcome, result *invoke.Result, collection *invoke.Collection, err error) (word, kind string) {
	word, kind = classify(action, outcome, result.Verification, err)
	if collection != nil && collection.Incomplete && kind == kindSuccess {
		return word + " (incomplete)", kindPartial
	}
	return word, kind
}

// fillDetails adds the digests note, signature, diagnostics, error, record details, and collection
// summary to a section.
func fillDetails(section *Operation, entry *Entry, result *invoke.Result, collection *invoke.Collection) {
	section.Integrity = integrityNote(section, result)
	section.Signature = signatureOf(artifact.Action(section.Action), result)
	section.Diagnostics, section.DiagnosticsMore = diagnosticsOf(result.Diagnostics)
	section.Error = errorBlock(entry, result)
	addRecordDetails(section, result)
	section.Collection = collectionOf(collection)
	section.Summary = summaryOf(section, collection)
}

// titleOf is the section heading text: the action and the best reference known.
func titleOf(section *Operation, op *artifact.OperationResult) string {
	reference := section.Ref
	if reference == "" {
		reference = code(op.Requested)
	}
	if reference == "" {
		return section.ActionLabel
	}
	return section.ActionLabel + " " + reference
}

// codes renders each value as a code span and drops empty ones.
func codes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if span := code(value); span != "" {
			out = append(out, span)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// digestsOf lists the digests an operation reported, each labelled with what it identifies. The
// four digest types are never merged: a manifest digest, a record digest, a content digest, and a
// transfer checksum answer different questions.
func digestsOf(op *artifact.OperationResult) []Digest {
	candidates := []struct {
		label, meaning string
		value          digest.Digest
	}{
		{"Resolved", "OCI manifest or index digest; the record digest for local and S3 repositories", op.ResolvedDigest},
		{"Record", "canonical bytes of the package Record", op.RecordDigest},
		{"Content", "portable content identity of the inventory and documents", op.ContentDigest},
		{"Representation", "checksum of the transferred payload; it proves integrity, not the publisher", op.RepresentationDigest},
	}
	var digests []Digest
	for _, candidate := range candidates {
		if candidate.value == "" {
			continue
		}
		digests = append(digests, Digest{
			Label:   candidate.label,
			Meaning: candidate.meaning,
			Value:   code(string(candidate.value)),
			Short:   code(shortDigest(candidate.value)),
		})
	}
	return digests
}

// integrityNote says what the reported digests prove. A checksum or content digest shows the bytes
// are intact; only a verified signature authenticates the publisher.
func integrityNote(section *Operation, result *invoke.Result) string {
	if len(section.Digests) == 0 {
		return ""
	}
	if result.Verification != nil && result.Verification.Status == artifact.VerificationVerified {
		return "The signature was verified against the configured trust policy, so the signer is authenticated. The digests identify the verified content."
	}
	return "Digests prove that the content is intact. They do not authenticate the publisher, and this operation did not report a verified signature."
}

// errorBlock renders the failure message as a fenced block. The message is masked and stripped of
// URL credentials, and the fence cannot be closed by the message.
func errorBlock(entry *Entry, result *invoke.Result) string {
	message := ""
	switch {
	case entry.Err != nil:
		message = entry.Err.Error()
	case result.Error != "":
		message = result.Error
	}
	return fenced(message)
}

// diagnosticsOf converts operation diagnostics, capped at maxDiagnostics.
func diagnosticsOf(diagnostics []artifact.Diagnostic) (listed []Diagnostic, more int) {
	for index := range diagnostics {
		if index >= maxDiagnostics {
			more = len(diagnostics) - maxDiagnostics
			break
		}
		listed = append(listed, Diagnostic{
			Severity: string(diagnostics[index].Severity),
			Code:     code(diagnostics[index].Code),
			Message:  plain(diagnostics[index].Message),
		})
	}
	return listed, more
}

// addRecordDetails fills the description, labels, and inventory from the Record the result carries:
// the sealed Record of a publication, or the Record of an inspection.
func addRecordDetails(section *Operation, result *invoke.Result) {
	record := result.Record
	if record == nil && result.Inspection != nil {
		record = result.Inspection.Record
	}
	if record == nil {
		return
	}
	section.Description = plain(record.Metadata.Description)
	section.Labels = labelsOf(record.Metadata.Labels)
	section.Inventory = inventoryOf(record.Inventory)
}

// labelsOf lists labels sorted by key, capped at maxLabels.
func labelsOf(labels map[string]string) []Label {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	listed := make([]Label, 0, len(keys))
	for _, key := range keys {
		if len(listed) == maxLabels {
			break
		}
		listed = append(listed, Label{Key: code(key), Value: code(labels[key])})
	}
	if len(listed) == 0 {
		return nil
	}
	return listed
}

// inventoryOf summarizes the payload entries of a Record.
func inventoryOf(entries []artifact.Entry) *Inventory {
	if len(entries) == 0 {
		return nil
	}
	inventory := &Inventory{Count: len(entries)}
	var total int64
	for index := range entries {
		total += entries[index].Size
		if index < maxInventoryFiles {
			inventory.Files = append(inventory.Files, File{Path: code(entries[index].Path), Size: humanSize(entries[index].Size)})
		}
	}
	inventory.More = max(len(entries)-maxInventoryFiles, 0)
	inventory.Bytes = humanSize(total)
	return inventory
}

// collectionOf summarizes a list or search result.
func collectionOf(collection *invoke.Collection) *Collection {
	if collection == nil {
		return nil
	}
	view := &Collection{Count: len(collection.Artifacts), Incomplete: collection.Incomplete}
	for index := range collection.Repositories {
		listing := &collection.Repositories[index]
		view.Repositories = append(view.Repositories, RepositoryResult{
			Repository: code(listing.Repository),
			Count:      listing.Count,
			Source:     plain(listing.Source),
		})
	}
	for index := range collection.Artifacts {
		if index >= maxListedArtifacts {
			view.More = len(collection.Artifacts) - maxListedArtifacts
			break
		}
		view.Artifacts = append(view.Artifacts, code(collection.Artifacts[index].Ref))
	}
	return view
}

// summaryOf states in one sentence what happened.
func summaryOf(section *Operation, collection *invoke.Collection) string {
	target := section.Ref
	if target == "" {
		target = section.Requested
	}
	if collection != nil {
		return collectionSummary(section, collection)
	}
	if section.Kind == kindFailed || section.Kind == kindCanceled {
		return failureSummary(section, target)
	}
	subject := strings.TrimSpace(strings.ToLower(section.Status) + " " + target)
	if section.Repository != "" {
		subject += " in repository " + section.Repository
	}
	return titleCase(strings.TrimSpace(subject)) + "."
}

// failureSummary words a failed or canceled operation without claiming any outcome.
func failureSummary(section *Operation, target string) string {
	verb := "failed"
	switch section.Status {
	case "Canceled":
		verb = "was canceled"
	case "Timed out":
		verb = "timed out"
	}
	text := "The " + strings.ToLower(section.Action) + " operation"
	if target != "" {
		text += " for " + target
	}
	return text + " " + verb + "."
}

// collectionSummary words a list or search result, including an incomplete listing.
func collectionSummary(section *Operation, collection *invoke.Collection) string {
	text := fmt.Sprintf("%s %d artifact", strings.TrimSuffix(section.Status, " (incomplete)"), len(collection.Artifacts))
	if len(collection.Artifacts) != 1 {
		text += "s"
	}
	if collection.Incomplete {
		text += "; the listing is incomplete because a repository could not be listed"
	}
	return text + "."
}
