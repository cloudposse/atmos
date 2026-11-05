package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Inspect implements [artifact.Service]. It reads metadata only. Verification is reported when
// policy requires or constrains it; a failed or unchecked verification is information here, not a
// reason to hide metadata, because inspect consumes no payload.
func (s *Service) Inspect(ctx context.Context, request artifact.InspectRequest) (*artifact.InspectResult, error) {
	defer perf.Track(nil, "service.Service.Inspect")()

	op := s.begin(artifact.ActionInspect, request.Repository, request.Ref.String())
	result := &artifact.InspectResult{}
	err := s.inspect(ctx, op, &request, result)
	result.Operation = op.done(err)
	return result, err
}

func (s *Service) inspect(ctx context.Context, op *operation, request *artifact.InspectRequest, result *artifact.InspectResult) error {
	if err := validateRepositoryRef(request.Repository, request.Ref); err != nil {
		return err
	}
	info, repo, err := s.openForRead(ctx, request.Repository, opInspect)
	if err != nil {
		return err
	}
	_, inspection, err := s.pinAndInspect(ctx, op, repo, request.Repository, request.Ref)
	if err != nil {
		return err
	}
	result.Inspection = *inspection
	op.result.Outcome = artifact.OutcomeRead
	op.result.Location = inspection.Representation.Locator
	verification, verifyErr := gate(ctx, request.Repository, repo, inspection, info.Policy)
	if verification != nil && verification.Status != artifact.VerificationUnchecked {
		result.Verification = verification
	}
	if verifyErr != nil {
		op.warn("verification-failed", verifyErr.Error())
	}
	return nil
}

// ShowDocument implements [artifact.Service]. It never downloads the payload; the document bytes
// are bounded and verified against the Record descriptor.
//
//nolint:gocritic // The request is passed by value because artifact.Service fixes the signature.
func (s *Service) ShowDocument(ctx context.Context, request artifact.ShowDocumentRequest) (*artifact.DocumentResult, error) {
	defer perf.Track(nil, "service.Service.ShowDocument")()

	op := s.begin(artifact.ActionShow, request.Repository, request.Ref.String())
	result := &artifact.DocumentResult{}
	err := s.showDocument(ctx, op, &request, result)
	result.Operation = op.done(err)
	return result, err
}

func (s *Service) showDocument(ctx context.Context, op *operation, request *artifact.ShowDocumentRequest, result *artifact.DocumentResult) error {
	if err := validateShow(request); err != nil {
		return err
	}
	info, repo, err := s.openForRead(ctx, request.Repository, opShow)
	if err != nil {
		return err
	}
	pinned, err := repo.Resolve(ctx, request.Ref)
	if err != nil {
		return fmt.Errorf("resolve %s in repository %q: %w", request.Ref, request.Repository, err)
	}
	if err := s.gatePinned(ctx, op, &target{alias: request.Repository, repo: repo, info: &info}, &pinned); err != nil {
		return err
	}
	stream, err := repo.OpenDocument(ctx, pinned, request.Role)
	if err != nil {
		return fmt.Errorf("open %s of %s in repository %q: %w", request.Role, request.Ref, request.Repository, err)
	}
	defer stream.Reader.Close()
	content, err := readDocument(stream)
	if err != nil {
		return err
	}
	result.Document, result.Content = stream.Document, content
	op.result.Outcome = artifact.OutcomeRead
	op.result.Size = int64(len(content))
	return nil
}

func validateShow(request *artifact.ShowDocumentRequest) error {
	if err := validateRepositoryRef(request.Repository, request.Ref); err != nil {
		return err
	}
	if request.Role != artifact.DocumentReadme && request.Role != artifact.DocumentChangelog {
		return fmt.Errorf("%w: document role must be %q or %q", errUtils.ErrArtifactConfigInvalid, artifact.DocumentReadme, artifact.DocumentChangelog)
	}
	return nil
}

// gatePinned enforces required verification on an already pinned subject. It inspects only when
// policy needs the subject, so unconstrained reads stay metadata-light.
func (s *Service) gatePinned(ctx context.Context, op *operation, tgt *target, pinned *artifact.PinnedReference) error {
	if !verificationRequired(tgt.info.Policy) {
		return nil
	}
	inspection, err := tgt.repo.Inspect(ctx, *pinned)
	if err != nil {
		return err
	}
	fillFromInspection(op, pinned, inspection)
	_, err = gate(ctx, tgt.alias, tgt.repo, inspection, tgt.info.Policy)
	return err
}

// readDocument reads a document within the size bound and verifies it against its descriptor.
func readDocument(stream *artifact.DocumentStream) ([]byte, error) {
	data, err := artifact.ReadBounded(stream.Reader, artifact.MaxDocumentSize)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != stream.Document.Size || digest.NewDigestFromBytes(digest.SHA256, sum[:]) != stream.Document.Digest {
		return nil, fmt.Errorf("%w: the %s does not match its descriptor", errUtils.ErrArtifactIntegrityFailed, stream.Document.Role)
	}
	return data, nil
}

// List implements [artifact.Service].
func (s *Service) List(ctx context.Context, request artifact.ListRequest) (*artifact.ListResult, error) {
	defer perf.Track(nil, "service.Service.List")()

	return s.enumerate(ctx, artifact.ActionList, &enumeration{
		repositories: request.Repositories, name: request.Name, labels: request.Labels, refresh: request.Refresh,
	})
}

// Search implements [artifact.Service]. The text is matched case-insensitively against names,
// kinds, descriptions, and label keys and values of the enumerated collection.
func (s *Service) Search(ctx context.Context, request artifact.SearchRequest) (*artifact.ListResult, error) {
	defer perf.Track(nil, "service.Service.Search")()

	return s.enumerate(ctx, artifact.ActionSearch, &enumeration{
		repositories: request.Repositories, labels: request.Labels, refresh: request.Refresh, text: request.Query,
	})
}

type enumeration struct {
	repositories []string
	name         string
	labels       []artifact.Label
	refresh      bool
	text         string
}

func (s *Service) enumerate(ctx context.Context, _ artifact.Action, request *enumeration) (*artifact.ListResult, error) {
	result := &artifact.ListResult{Artifacts: []artifact.ListEntry{}}
	err := s.runEnumeration(ctx, request, result)
	return result, err
}

// runEnumeration fans out over the selected repositories with bounded concurrency, collects
// per-repository outcomes, and fails only when an explicitly requested repository fails or none
// succeeded. Unsupported enumeration on one of several default repositories is reported, not fatal.
func (s *Service) runEnumeration(ctx context.Context, request *enumeration, result *artifact.ListResult) error {
	aliases := request.repositories
	explicit := len(aliases) > 0
	if !explicit {
		aliases = s.repos.Aliases()
	}
	outcomes := make([]listOutcome, len(aliases))
	gates := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup
	for index := range aliases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gates <- struct{}{}
			defer func() { <-gates }()
			outcomes[index] = s.listRepository(ctx, aliases[index], request)
		}()
	}
	wg.Wait()
	var errs []error
	succeeded := 0
	for index := range outcomes {
		result.Artifacts = append(result.Artifacts, outcomes[index].entries...)
		result.Repositories = append(result.Repositories, outcomes[index].listing)
		if outcomes[index].err != nil {
			errs = append(errs, outcomes[index].err)
			continue
		}
		succeeded++
	}
	sortEntries(result.Artifacts)
	if explicit && len(errs) > 0 || succeeded == 0 && len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
