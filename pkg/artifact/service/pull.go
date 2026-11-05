package service

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Pull implements [artifact.Service]. It resolves the reference once, verifies the pinned subject
// against policy, refuses to overlay a different non-empty destination without force, extracts
// into staging, and only then replaces the destination.
//
//nolint:gocritic // The request is passed by value because artifact.Service fixes the signature.
func (s *Service) Pull(ctx context.Context, request artifact.PullRequest) (*artifact.OperationResult, error) {
	defer perf.Track(nil, "service.Service.Pull")()

	op := s.begin(artifact.ActionPull, request.Repository, request.Ref.String())
	err := s.pull(ctx, op, &request)
	result := op.done(err)
	return &result, err
}

func (s *Service) pull(ctx context.Context, op *operation, request *artifact.PullRequest) error {
	if err := validatePull(request); err != nil {
		return err
	}
	info, repo, err := s.openForRead(ctx, request.Repository, opPull)
	if err != nil {
		return err
	}
	pinned, inspection, err := s.pinAndInspect(ctx, op, repo, request.Repository, request.Ref)
	if err != nil {
		return err
	}
	if inspection.Record == nil {
		return artifact.NewUnsupportedCapability(request.Repository, opPull, "materialize/graph", "a native OCI graph without a portable Record cannot be materialized as a file tree")
	}
	if _, err := gate(ctx, request.Repository, repo, inspection, info.Policy); err != nil {
		return err
	}
	unchanged, err := checkDestination(op, request, inspection.Record)
	if err != nil || unchanged {
		return err
	}
	return s.download(ctx, op, request, &pinnedSource{repo: repo, pinned: &pinned, inspection: inspection})
}

// pinnedSource is a repository together with the subject that was pinned and inspected in it.
type pinnedSource struct {
	repo       artifact.Repository
	pinned     *artifact.PinnedReference
	inspection *artifact.Inspection
}

// download reads the pinned content and materializes it, replacing the destination only after the
// whole tree was extracted and verified in staging.
func (s *Service) download(ctx context.Context, op *operation, request *artifact.PullRequest, source *pinnedSource) error {
	content, err := source.repo.Read(ctx, *source.pinned)
	if err != nil {
		return fmt.Errorf("read %s from repository %q: %w", request.Ref, request.Repository, err)
	}
	defer content.Close()
	if err := bindContent(content, source.inspection); err != nil {
		return err
	}
	if err := materialize.Materialize(ctx, content, request.Destination); err != nil {
		return err
	}
	op.result.Outcome = artifact.OutcomeRead
	return nil
}

func validatePull(request *artifact.PullRequest) error {
	if err := validateRepositoryRef(request.Repository, request.Ref); err != nil {
		return err
	}
	if request.Destination == "" {
		return fmt.Errorf("%w: a destination is required", errUtils.ErrArtifactConfigInvalid)
	}
	return nil
}

// checkDestination classifies the output directory. It reports unchanged when the destination
// already holds exactly the artifact and refuses a different non-empty destination without force.
func checkDestination(op *operation, request *artifact.PullRequest, record *artifact.Record) (bool, error) {
	state, err := materialize.InspectDestination(request.Destination, record)
	if err != nil {
		return false, err
	}
	op.result.Location = request.Destination
	switch {
	case state == materialize.DestinationMatches:
		op.result.Outcome = artifact.OutcomeUnchanged
		op.info("unchanged", "the destination already holds exactly this artifact; nothing was downloaded")
		return true, nil
	case state == materialize.DestinationDiffers && !request.Force:
		return false, fmt.Errorf("%w: destination %q already holds different content; use force to replace it", errUtils.ErrArtifactConflict, request.Destination)
	}
	return false, nil
}

// bindContent checks the content belongs to the inspected, pinned Record and fills a missing one.
func bindContent(content *artifact.Content, inspection *artifact.Inspection) error {
	if content.Record == nil {
		content.Record = inspection.Record
		return nil
	}
	got, err := content.Record.RecordDigest()
	if err != nil {
		return err
	}
	want, err := inspection.Record.RecordDigest()
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%w: the content record %s differs from the pinned record %s", errUtils.ErrArtifactIntegrityFailed, got, want)
	}
	return nil
}

// openForRead describes the repository offline, requires read support, and opens it.
func (s *Service) openForRead(ctx context.Context, alias, operation string) (RepositoryInfo, artifact.Repository, error) {
	info, err := s.repos.Describe(alias)
	if err != nil {
		return RepositoryInfo{}, nil, err
	}
	if !info.Capabilities.Read {
		return info, nil, artifact.NewUnsupportedCapability(alias, operation, "read", "the repository is write-only")
	}
	repo, err := s.repos.Open(ctx, alias)
	if err != nil {
		return info, nil, err
	}
	return info, repo, nil
}

// pinAndInspect resolves the reference exactly once and inspects the pinned subject. Everything
// after this point uses the returned pin, never the mutable reference.
func (s *Service) pinAndInspect(ctx context.Context, op *operation, repo artifact.Repository, alias string, ref artifact.Ref) (artifact.PinnedReference, *artifact.Inspection, error) {
	pinned, err := repo.Resolve(ctx, ref)
	if err != nil {
		return pinned, nil, fmt.Errorf("resolve %s in repository %q: %w", ref, alias, err)
	}
	inspection, err := repo.Inspect(ctx, pinned)
	if err != nil {
		return pinned, nil, fmt.Errorf("inspect %s in repository %q: %w", ref, alias, err)
	}
	fillFromInspection(op, &pinned, inspection)
	return pinned, inspection, nil
}

// fillFromInspection records the pinned identity in the typed result.
func fillFromInspection(op *operation, pinned *artifact.PinnedReference, inspection *artifact.Inspection) {
	op.result.ResolvedDigest = pinned.ResolvedDigest
	op.result.RecordDigest = pinned.RecordDigest
	op.result.ContentDigest = pinned.ContentDigest
	op.result.RepresentationDigest = inspection.Representation.RepresentationDigest
	op.result.Size = payloadSize(inspection)
	if inspection.Record != nil {
		op.result.Ref = artifact.Ref{Name: inspection.Record.Name, Version: inspection.Record.Version}.String()
		op.result.ContentDigest = inspection.Record.ContentDigest
	}
}
