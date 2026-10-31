package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/perf"
)

// copyPlan is everything resolved before a copy mutates the destination.
type copyPlan struct {
	ref        artifact.Ref
	src, dst   *target
	pinned     artifact.PinnedReference
	inspection *artifact.Inspection
}

// copyWrite is how a copy will write the destination: the incoming description, the write
// closure, and a cleanup for any staging it owns.
type copyWrite struct {
	in      *incoming
	write   writer
	cleanup func()
}

// Copy implements [artifact.Service]. The source reference is resolved once and verified against
// BOTH the source and destination policies before the destination is touched; force never
// bypasses that. A compatible native transfer is used when available, otherwise the bounded
// staging fallback repacks the unchanged Record for the destination's packaging.
//
//nolint:gocritic // The request is passed by value because artifact.Service fixes the signature.
func (s *Service) Copy(ctx context.Context, request artifact.CopyRequest) (*artifact.OperationResult, error) {
	defer perf.Track(nil, "service.Service.Copy")()

	op := s.begin(&request.Execution, artifact.ActionCopy, request.To, request.Ref.String())
	err := s.copyArtifact(ctx, op, &request)
	result := op.done(err)
	return &result, err
}

func validateCopy(request *artifact.CopyRequest) error {
	if request.From == "" || request.To == "" || request.From == request.To {
		return fmt.Errorf("%w: copy needs distinct source and destination repositories", errUtils.ErrArtifactConfigInvalid)
	}
	return request.Ref.Validate()
}

func (s *Service) copyArtifact(ctx context.Context, op *operation, request *artifact.CopyRequest) error {
	if err := validateCopy(request); err != nil {
		return err
	}
	plan, err := s.planCopy(ctx, op, request)
	if err != nil {
		return err
	}
	if plan.inspection.Record == nil && !canNativeCopy(plan) {
		return artifact.NewUnsupportedCapability(request.To, opCopy, "transfer/graph", "a native OCI graph can only be copied between repositories that support a compatible native transfer")
	}
	cw, err := s.copyWriter(ctx, op, plan)
	if err != nil {
		return err
	}
	defer cw.cleanup()

	pub := &publication{op: op, tgt: plan.dst, in: cw.in, force: request.Force, write: cw.write, state: &publishState{}}
	if err := s.commit(ctx, pub); err != nil {
		return err
	}
	if op.result.Outcome == "" {
		op.result.Outcome = artifact.OutcomeCreated
	}
	op.result.Repository = request.To
	if op.result.Outcome != artifact.OutcomeUnchanged {
		s.carryEvidence(ctx, op, plan)
	}
	return nil
}

// planCopy checks capabilities and lock guarantees, resolves the source once, and enforces both policies.
func (s *Service) planCopy(ctx context.Context, op *operation, request *artifact.CopyRequest) (*copyPlan, error) {
	srcInfo, srcRepo, err := s.openForRead(ctx, request.From, opCopy)
	if err != nil {
		return nil, err
	}
	dstInfo, _, err := s.describeForWrite(request.To, opCopy)
	if err != nil {
		return nil, err
	}
	if err := checkLockGuarantee(&dstInfo, opCopy); err != nil {
		return nil, err
	}
	pinned, inspection, err := s.pinAndInspect(ctx, op, srcRepo, request.From, request.Ref)
	if err != nil {
		return nil, err
	}
	if _, err := gate(ctx, request.From, srcRepo, inspection, srcInfo.Policy, dstInfo.Policy); err != nil {
		return nil, err
	}
	dstRepo, err := s.repos.Open(ctx, request.To)
	if err != nil {
		return nil, err
	}
	return &copyPlan{
		ref:        request.Ref,
		src:        &target{alias: request.From, repo: srcRepo, info: &srcInfo},
		dst:        &target{alias: request.To, repo: dstRepo, info: &dstInfo},
		pinned:     pinned,
		inspection: inspection,
	}, nil
}

func canNativeCopy(plan *copyPlan) bool {
	copier, ok := plan.dst.repo.(artifact.Copier)
	return ok && copier.CanCopyFrom(plan.src.repo)
}

// copyWriter chooses the native transfer or the staging fallback.
func (s *Service) copyWriter(ctx context.Context, op *operation, plan *copyPlan) (*copyWrite, error) {
	if canNativeCopy(plan) {
		return nativeCopyWriter(plan)
	}
	prepared, cleanup, err := s.stageCopy(ctx, op, plan)
	if err != nil {
		return nil, err
	}
	in, err := newIncoming(prepared)
	if err != nil {
		cleanup()
		return nil, err
	}
	in.inspection = plan.inspection
	write := func(ctx context.Context, options artifact.PublishOptions) (*artifact.PublishResult, error) {
		return plan.dst.repo.Publish(ctx, prepared, options)
	}
	return &copyWrite{in: in, write: write, cleanup: cleanup}, nil
}

// nativeRef is the destination reference of a native transfer: the Record's name and version, or
// the requested tag for a foreign graph. A digest-only graph has no tag to publish under.
func nativeRef(plan *copyPlan) (artifact.Ref, error) {
	if record := plan.inspection.Record; record != nil {
		return artifact.Ref{Name: record.Name, Version: record.Version}, nil
	}
	if plan.ref.Version == "" {
		return artifact.Ref{}, fmt.Errorf("%w: copying a native graph needs a name:version reference to publish under in the destination", errUtils.ErrArtifactRefInvalid)
	}
	return artifact.Ref{Name: plan.ref.Name, Version: plan.ref.Version}, nil
}

func nativeCopyWriter(plan *copyPlan) (*copyWrite, error) {
	copier, _ := plan.dst.repo.(artifact.Copier)
	ref, err := nativeRef(plan)
	if err != nil {
		return nil, err
	}
	in := &incoming{ref: ref, inspection: plan.inspection}
	if record := plan.inspection.Record; record != nil {
		if err := in.bindRecord(record, plan.inspection); err != nil {
			return nil, err
		}
	}
	write := func(ctx context.Context, options artifact.PublishOptions) (*artifact.PublishResult, error) {
		return copier.CopyFrom(ctx, plan.src.repo, plan.pinned, artifact.CopyFromOptions(options))
	}
	return &copyWrite{in: in, write: write, cleanup: func() {}}, nil
}

// bindRecord fills the identity of a stored Record into the incoming description.
func (in *incoming) bindRecord(record *artifact.Record, inspection *artifact.Inspection) error {
	recordDigest, err := record.RecordDigest()
	if err != nil {
		return err
	}
	intent, err := record.IntentDigest()
	if err != nil {
		return err
	}
	in.record, in.recordDigest, in.intent = record, recordDigest, intent
	in.payload = inspection.Representation.Payload
	in.size = payloadSize(inspection)
	return nil
}

// stageCopy reads the pinned source into a bounded temporary tree and repacks the unchanged Record
// for the destination packaging. The returned cleanup removes every owned temporary directory.
func (s *Service) stageCopy(ctx context.Context, op *operation, plan *copyPlan) (*artifact.PreparedArtifact, func(), error) {
	content, err := plan.src.repo.Read(ctx, plan.pinned)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s from repository %q: %w", plan.pinned.Ref, plan.src.alias, err)
	}
	defer content.Close()
	if err := bindContent(content, plan.inspection); err != nil {
		return nil, nil, err
	}
	docs, err := collectDocumentBytes(content)
	if err != nil {
		return nil, nil, err
	}
	tree, err := os.MkdirTemp(s.stagingRoot, "atmos-artifact-copy-*")
	if err != nil {
		return nil, nil, fmt.Errorf("%w: create copy staging: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	cleanup := func() { _ = os.RemoveAll(tree) }
	files := filepath.Join(tree, "files")
	if err := materialize.Materialize(ctx, content, files); err != nil {
		cleanup()
		return nil, nil, err
	}
	prepared, err := s.repacker.Repack(ctx, &produce.RepackRequest{
		Record:      *plan.inspection.Record,
		Tree:        files,
		Documents:   docs,
		Packaging:   packagingFor(plan.dst.info),
		OperationID: op.id,
		PreparedAt:  op.preparedAt,
	})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	prior := prepared.Cleanup
	prepared.Cleanup = func() error {
		var firstErr error
		if prior != nil {
			firstErr = prior()
		}
		cleanup()
		return firstErr
	}
	return prepared, func() { _ = prepared.Close() }, nil
}

func packagingFor(info *RepositoryInfo) artifact.Packaging {
	if info.Packaging != "" {
		return info.Packaging
	}
	if len(info.Capabilities.Packagings) > 0 {
		return info.Capabilities.Packagings[0]
	}
	return artifact.PackagingNone
}

// collectDocumentBytes reads the (bounded) document streams of the content by role.
func collectDocumentBytes(content *artifact.Content) (map[artifact.DocumentRole][]byte, error) {
	docs := map[artifact.DocumentRole][]byte{}
	if content.Record == nil {
		return docs, nil
	}
	for index := range content.Streams {
		stream := &content.Streams[index]
		if stream.Role != artifact.BlobDocument || stream.Reader == nil {
			continue
		}
		data, err := artifact.ReadBounded(stream.Reader, artifact.MaxDocumentSize)
		if err != nil {
			return nil, err
		}
		role, ok := documentRoleFor(content.Record, stream.Descriptor.Digest.String())
		if !ok {
			return nil, fmt.Errorf("%w: a document stream matches no descriptor in the Record", errUtils.ErrArtifactIntegrityFailed)
		}
		docs[role] = data
	}
	return docs, nil
}

func documentRoleFor(record *artifact.Record, digestValue string) (artifact.DocumentRole, bool) {
	for _, doc := range record.Documents {
		if doc.Digest.String() == digestValue {
			return doc.Role, true
		}
	}
	return "", false
}

// carryEvidence moves signature evidence along with a copy when both repositories support it.
// Evidence is never invented: without both optional interfaces, a diagnostic explains that the
// destination holds no signatures.
func (s *Service) carryEvidence(ctx context.Context, op *operation, plan *copyPlan) {
	reader, canRead := plan.src.repo.(artifact.EvidenceReader)
	writer, canWrite := plan.dst.repo.(artifact.EvidenceWriter)
	if !canRead || !canWrite {
		if verificationRequired(plan.dst.info.Policy) {
			op.warn("evidence-not-carried", "the destination policy requires verification but the repositories cannot transfer signature evidence; consumers of the destination copy must verify against the source evidence")
		}
		return
	}
	blobs, err := reader.ReadEvidence(ctx, &plan.pinned)
	if err != nil || len(blobs) == 0 {
		if err != nil {
			op.warn("evidence-read-failed", err.Error())
		}
		return
	}
	dstRef, err := nativeRef(plan)
	if err == nil {
		var dstPinned artifact.PinnedReference
		if dstPinned, err = plan.dst.repo.Resolve(ctx, dstRef); err == nil {
			_, err = writer.WriteEvidence(ctx, &dstPinned, blobs, nil)
		}
	}
	if err != nil {
		op.warn("evidence-write-failed", err.Error())
	}
}
