package invoke

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/perf"
)

// detailedPublisher is implemented by the real service. It returns the sealed Record next to the
// operation result, which a dry run needs in order to show the files it selected.
type detailedPublisher interface {
	PublishDetailed(ctx context.Context, request artifact.PublishRequest) (*service.PublishOutcome, error)
}

// Run executes one operation against the service and returns its typed result.
//
// The returned Outcome is never nil, and the matching Result or Collection is never nil, even when
// the operation fails: a partial result (the identity, whatever the service resolved before it
// failed, and the failure markers) accompanies the error. Callers report it and then surface the
// error, so a failure is never reported as a success and never loses what was learned.
func Run(ctx context.Context, svc artifact.Service, params *Params) (*Outcome, error) {
	defer perf.Track(nil, "invoke.Run")()

	if params.Action == artifact.ActionList || params.Action == artifact.ActionSearch {
		collection, err := runCollection(ctx, svc, params)
		return &Outcome{Collection: collection}, err
	}
	result, err := runOperation(ctx, svc, params)
	if result == nil {
		result = &Result{}
	}
	result.MarkFailed(err, params.Action, params.Repository, requestedOf(params))
	return &Outcome{Result: result}, err
}

// requestedOf is the reference as the caller typed it, for failure results. Publish has no
// reference to type; the service reports the name and version it resolved.
func requestedOf(params *Params) string {
	return params.Ref
}

// runOperation dispatches the single-artifact actions.
func runOperation(ctx context.Context, svc artifact.Service, params *Params) (*Result, error) {
	switch params.Action {
	case artifact.ActionPublish:
		return runPublish(ctx, svc, params)
	case artifact.ActionPull:
		return withRef(params, func(ref artifact.Ref) (*Result, error) {
			op, err := svc.Pull(ctx, artifact.PullRequest{Execution: params.Execution, Repository: params.Repository, Ref: ref, Destination: params.Destination, Force: params.Force})
			return fromOperation(op), err
		})
	case artifact.ActionCopy:
		return withRef(params, func(ref artifact.Ref) (*Result, error) {
			op, err := svc.Copy(ctx, artifact.CopyRequest{Execution: params.Execution, Ref: ref, From: params.From, To: params.To, Force: params.Force})
			return fromOperation(op), err
		})
	case artifact.ActionInspect:
		return withRef(params, func(ref artifact.Ref) (*Result, error) { return runInspect(ctx, svc, params, ref) })
	case artifact.ActionDelete:
		return withRef(params, func(ref artifact.Ref) (*Result, error) {
			op, err := svc.Delete(ctx, artifact.DeleteRequest{Execution: params.Execution, Repository: params.Repository, Ref: ref})
			return fromOperation(op), err
		})
	case artifact.ActionShow:
		return withRef(params, func(ref artifact.Ref) (*Result, error) { return runShow(ctx, svc, params, ref) })
	case artifact.ActionSign:
		return withRef(params, func(ref artifact.Ref) (*Result, error) { return runSign(ctx, svc, params, ref) })
	case artifact.ActionVerify:
		return withRef(params, func(ref artifact.Ref) (*Result, error) { return runVerify(ctx, svc, params, ref) })
	default:
		return nil, fmt.Errorf("%w: unsupported artifact action %q", errUtils.ErrArtifactConfigInvalid, params.Action)
	}
}

// withRef parses the reference once, before any service call, and runs the action with it.
func withRef(params *Params, run func(ref artifact.Ref) (*Result, error)) (*Result, error) {
	ref, err := artifact.ParseRef(params.Ref)
	if err != nil {
		return nil, err
	}
	return run(ref)
}

// fromOperation builds a result from a plain operation result.
func fromOperation(op *artifact.OperationResult) *Result {
	if op == nil {
		return &Result{}
	}
	return &Result{OperationResult: *op}
}

func runPublish(ctx context.Context, svc artifact.Service, params *Params) (*Result, error) {
	request := artifact.PublishRequest{
		Execution:  params.Execution,
		Repository: params.Repository,
		Source:     params.Source,
		Name:       params.Name,
		Version:    params.Version,
		Kind:       params.Kind,
		Metadata:   artifact.Metadata{Labels: params.Labels},
		Readme:     params.Readme,
		Changelog:  params.Changelog,
		Force:      params.Force,
		DryRun:     params.DryRun,
	}
	if detailed, ok := svc.(detailedPublisher); ok {
		outcome, err := detailed.PublishDetailed(ctx, request)
		res := fromOperation(&outcome.Operation)
		if outcome.Record.Name != "" {
			record := outcome.Record
			res.Record = &record
		}
		return res, err
	}
	op, err := svc.Publish(ctx, request)
	return fromOperation(op), err
}

func runInspect(ctx context.Context, svc artifact.Service, params *Params, ref artifact.Ref) (*Result, error) {
	inspected, err := svc.Inspect(ctx, artifact.InspectRequest{Execution: params.Execution, Repository: params.Repository, Ref: ref})
	if inspected == nil {
		return nil, err
	}
	res := fromOperation(&inspected.Operation)
	res.Inspection = &inspected.Inspection
	res.Verification = inspected.Verification
	return res, err
}

func runShow(ctx context.Context, svc artifact.Service, params *Params, ref artifact.Ref) (*Result, error) {
	role := artifact.DocumentRole(params.Document)
	if role != artifact.DocumentReadme && role != artifact.DocumentChangelog {
		return nil, fmt.Errorf("%w: document must be %q or %q, got %q", errUtils.ErrArtifactConfigInvalid, artifact.DocumentReadme, artifact.DocumentChangelog, params.Document)
	}
	document, err := svc.ShowDocument(ctx, artifact.ShowDocumentRequest{Execution: params.Execution, Repository: params.Repository, Ref: ref, Role: role})
	if document == nil {
		return nil, err
	}
	res := fromOperation(&document.Operation)
	if document.Document.Digest != "" {
		descriptor := document.Document
		res.Document = &descriptor
		content := string(document.Content)
		res.Content = &content
	}
	return res, err
}

func runSign(ctx context.Context, svc artifact.Service, params *Params, ref artifact.Ref) (*Result, error) {
	signed, err := svc.Sign(ctx, artifact.SignRequest{Execution: params.Execution, Repository: params.Repository, Ref: ref})
	if signed == nil {
		return nil, err
	}
	res := fromOperation(&signed.Operation)
	if signed.Subject.Digest != "" {
		res.Subject = &signed.Subject
	}
	res.Evidence = signed.Evidence
	return res, err
}

func runVerify(ctx context.Context, svc artifact.Service, params *Params, ref artifact.Ref) (*Result, error) {
	verified, err := svc.Verify(ctx, artifact.VerifyRequest{Execution: params.Execution, Repository: params.Repository, Ref: ref})
	if verified == nil {
		return nil, err
	}
	res := fromOperation(&verified.Operation)
	verification := verified.Verification
	res.Verification = &verification
	if verification.Subject.Digest != "" {
		res.Subject = &verification.Subject
	}
	return res, err
}

// runCollection runs list or search and wraps the entries in the `artifacts` envelope.
func runCollection(ctx context.Context, svc artifact.Service, params *Params) (*Collection, error) {
	selectors := SelectorLabels(params.Labels)
	var (
		listing *artifact.ListResult
		err     error
	)
	if params.Action == artifact.ActionSearch {
		listing, err = svc.Search(ctx, artifact.SearchRequest{Repositories: params.Repositories, Query: params.Query, Labels: selectors, Refresh: params.Refresh})
	} else {
		listing, err = svc.List(ctx, artifact.ListRequest{Repositories: params.Repositories, Name: params.Name, Labels: selectors, Refresh: params.Refresh})
	}
	collection := &Collection{Artifacts: []Listed{}}
	if listing != nil {
		collection.Artifacts = ListedFrom(listing.Artifacts)
		collection.Repositories = listing.Repositories
		collection.Incomplete = listing.Incomplete
	}
	if err != nil {
		collection.Error = err.Error()
	}
	return collection, err
}

// SelectorLabels converts exact key/value selectors into the sorted label list the service takes.
func SelectorLabels(parsed map[string]string) []artifact.Label {
	defer perf.Track(nil, "invoke.SelectorLabels")()

	if len(parsed) == 0 {
		return nil
	}
	keys := make([]string, 0, len(parsed))
	for key := range parsed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	labels := make([]artifact.Label, 0, len(keys))
	for _, key := range keys {
		labels = append(labels, artifact.Label{Key: key, Value: parsed[key]})
	}
	return labels
}
