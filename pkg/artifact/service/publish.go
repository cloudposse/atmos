package service

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PublishOutcome is the detailed result of [Service.PublishDetailed]. On an unchanged no-op,
// Record is the EXISTING record, byte for byte, and Operation reports the current execution
// separately (its own operation ID and duration).
type PublishOutcome struct {
	// Operation is the typed operation result.
	Operation artifact.OperationResult

	// Record is the Record now published (the existing one on a no-op); it is empty on failure
	// before preparation and for dry runs holds the Record that would be published.
	Record artifact.Record

	// Existing reports an unchanged no-op that returned the stored Record.
	Existing bool
}

// Publish implements [artifact.Service].
//
//nolint:gocritic // The request is passed by value because artifact.Service fixes the signature.
func (s *Service) Publish(ctx context.Context, request artifact.PublishRequest) (*artifact.OperationResult, error) {
	defer perf.Track(nil, "service.Service.Publish")()

	outcome, err := s.PublishDetailed(ctx, request)
	return &outcome.Operation, err
}

// PublishDetailed prepares the source and stores it in one repository, returning the Record
// alongside the typed operation result. The result is always non-nil, even on error.
//
//nolint:gocritic // The request is passed by value to mirror Publish, which artifact.Service fixes.
func (s *Service) PublishDetailed(ctx context.Context, request artifact.PublishRequest) (*PublishOutcome, error) {
	defer perf.Track(nil, "service.Service.PublishDetailed")()

	op := s.begin(&request.Execution, artifact.ActionPublish, request.Repository, requestedName(request.Name, request.Version))
	state := &publishState{}
	err := s.publish(ctx, op, &request, state)
	return &PublishOutcome{Operation: op.done(err), Record: state.record, Existing: state.existing}, err
}

func requestedName(name, version string) string {
	if name == "" || version == "" {
		return name
	}
	return artifact.Ref{Name: name, Version: version}.String()
}

func (s *Service) publish(ctx context.Context, op *operation, request *artifact.PublishRequest, state *publishState) error {
	if err := validatePublish(request); err != nil {
		return err
	}
	info, packaging, err := s.describeForWrite(request.Repository, opPublish)
	if err != nil {
		return err
	}
	if err := checkLockGuarantee(&info, opPublish); err != nil {
		return err
	}
	producer, ok := s.producers.Lookup(request.Kind)
	if !ok {
		return fmt.Errorf("%w: no producer is registered for kind %q (registered: %v)", errUtils.ErrArtifactConfigInvalid, request.Kind, s.producers.Kinds())
	}
	prepared, err := producer.Prepare(ctx, artifact.PrepareRequest{
		Source:      request.Source,
		Name:        request.Name,
		Version:     request.Version,
		Kind:        request.Kind,
		Metadata:    mergeMetadata(info.Defaults, request.Metadata),
		Readme:      request.Readme,
		Changelog:   request.Changelog,
		Packaging:   packaging,
		OperationID: op.id,
		PreparedAt:  op.preparedAt,
	})
	if err != nil {
		return err
	}
	defer prepared.Close()

	in, err := newIncoming(prepared)
	if err != nil {
		return err
	}
	state.record = prepared.Record
	fillPrepared(op, in)
	reportExclusions(op, prepared.Exclusions)
	if request.DryRun {
		op.result.Outcome = artifact.OutcomeDryRun
		op.info("dry-run", fmt.Sprintf("would publish %s to repository %q as %s packaging; no repository was contacted", op.result.Ref, request.Repository, packaging))
		return nil
	}
	repo, err := s.repos.Open(ctx, request.Repository)
	if err != nil {
		return err
	}
	pub := &publication{
		op:    op,
		tgt:   &target{alias: request.Repository, repo: repo, info: &info},
		in:    in,
		force: request.Force,
		state: state,
		write: func(ctx context.Context, options artifact.PublishOptions) (*artifact.PublishResult, error) {
			return repo.Publish(ctx, prepared, options)
		},
	}
	s.event(ctx, pub, PhasePrepared, 0, nil)
	return s.commit(ctx, pub)
}

// validatePublish checks the request before anything is staged or contacted.
func validatePublish(request *artifact.PublishRequest) error {
	if err := requireRepository(request.Repository); err != nil {
		return err
	}
	if request.Source == "" {
		return fmt.Errorf("%w: a source is required", errUtils.ErrArtifactConfigInvalid)
	}
	if request.Kind == "" {
		request.Kind = artifact.KindFiles
	}
	if err := artifact.ValidateKind(request.Kind); err != nil {
		return err
	}
	if request.Name != "" {
		if err := artifact.ValidateName(request.Name); err != nil {
			return err
		}
	}
	if request.Version != "" {
		if err := artifact.ValidateVersion(request.Version); err != nil {
			return err
		}
	}
	return artifact.ValidateLabels(request.Metadata.Labels)
}

// describeForWrite resolves the repository description offline and checks that it can be written.
func (s *Service) describeForWrite(alias, operation string) (RepositoryInfo, artifact.Packaging, error) {
	info, err := s.repos.Describe(alias)
	if err != nil {
		return RepositoryInfo{}, "", err
	}
	if !info.Capabilities.Write {
		return info, "", artifact.NewUnsupportedCapability(alias, operation, "write", "the repository is read-only")
	}
	packaging := info.Packaging
	if packaging == "" && len(info.Capabilities.Packagings) > 0 {
		packaging = info.Capabilities.Packagings[0]
	}
	if !info.Capabilities.HasPackaging(packaging) {
		return info, packaging, artifact.NewUnsupportedCapability(alias, operation, "packaging/"+string(packaging), fmt.Sprintf("supported packagings: %v", info.Capabilities.Packagings))
	}
	return info, packaging, nil
}

// mergeMetadata layers explicit metadata over repository defaults: explicit labels override
// matching default keys, and an explicit description overrides the default.
func mergeMetadata(defaults, explicit artifact.Metadata) artifact.Metadata {
	merged := artifact.Metadata{
		Description: explicit.Description,
		Provenance:  explicit.Provenance,
	}
	if merged.Description == "" {
		merged.Description = defaults.Description
	}
	if len(defaults.Labels) > 0 || len(explicit.Labels) > 0 {
		merged.Labels = make(map[string]string, len(defaults.Labels)+len(explicit.Labels))
		for key, value := range defaults.Labels {
			merged.Labels[key] = value
		}
		for key, value := range explicit.Labels {
			merged.Labels[key] = value
		}
	}
	return merged
}

// fillPrepared records the identity of the prepared artifact in the typed result.
func fillPrepared(op *operation, in *incoming) {
	op.result.Ref = artifact.Ref{Name: in.record.Name, Version: in.record.Version}.String()
	op.result.Requested = op.result.Ref
	op.result.RecordDigest = in.recordDigest
	op.result.ContentDigest = in.record.ContentDigest
	op.result.Size = in.size
}

// diagnosticExcluded is the diagnostic code of a source entry that a kind-specific producer left out.
const diagnosticExcluded = "excluded"

// reportExclusions makes the default exclusions of a kind-specific producer visible: every excluded
// entry becomes one info diagnostic, so dry runs and real publications, structured output, and
// step results all show what was left out and which rule excluded it.
func reportExclusions(op *operation, exclusions []artifact.Exclusion) {
	for index := range exclusions {
		op.info(diagnosticExcluded, fmt.Sprintf("excluded %s (rule %s)", exclusions[index].Path, exclusions[index].Rule))
	}
}

// reportUnchanged fills the result from the EXISTING record; the current execution keeps its own
// operation ID and duration.
func (s *Service) reportUnchanged(pub *publication, cur *current) {
	op, record := pub.op, cur.inspection.Record
	pub.state.existing = true
	if record != nil {
		pub.state.record = *record
		op.result.ContentDigest = record.ContentDigest
	}
	op.result.Outcome = artifact.OutcomeUnchanged
	op.result.Location = cur.inspection.Representation.Locator
	op.result.ResolvedDigest = cur.pinned.ResolvedDigest
	op.result.RecordDigest = cur.pinned.RecordDigest
	op.result.RepresentationDigest = cur.inspection.Representation.RepresentationDigest
	op.result.Size = payloadSize(cur.inspection)
	op.info("unchanged", fmt.Sprintf("an identical artifact already exists in repository %q; the existing record was returned unchanged", pub.tgt.alias))
}

func payloadSize(inspection *artifact.Inspection) int64 {
	var total int64
	for index := range inspection.Representation.Payload {
		total += inspection.Representation.Payload[index].Size
	}
	return total
}

// reportWritten fills the result after a successful write.
func (s *Service) reportWritten(pub *publication, result *artifact.PublishResult, locks *lockSet, decision verdict, cur *current) {
	op, in := pub.op, pub.in
	if in.record != nil {
		pub.state.record = *in.record
	}
	outcome := result.Outcome
	if outcome == "" {
		outcome = artifact.OutcomeCreated
		if decision == verdictReplace || cur.unavailable {
			outcome = artifact.OutcomeReplaced
		}
	}
	op.result.Outcome = outcome
	op.result.Location = result.Location
	op.result.Aliases = result.Aliases
	op.result.ResolvedDigest = result.Pinned.ResolvedDigest
	op.result.RepresentationDigest = result.Representation.RepresentationDigest
	op.result.LockScope = result.LockScope
	if scope := locks.scope(); scope != "" {
		op.result.LockScope = scope
	}
	if op.result.ResolvedDigest == "" {
		op.result.ResolvedDigest = in.recordDigest
	}
}
