package step

import (
	"context"
	"errors"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/artifactwire"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Metadata keys of an artifact step result.
const (
	// ArtifactMetadataKey holds the typed operation result: an *invoke.Result for single-artifact
	// actions, or an *invoke.Collection (with its `artifacts` array) for list and search. It is the
	// same payload the CLI prints with --format=json.
	ArtifactMetadataKey = "artifact"

	// ArtifactListMetadataKey holds the `artifacts` array of a list or search result.
	ArtifactListMetadataKey = "artifacts"
)

// ArtifactHandler runs the artifact operations (publish, pull, copy, inspect, list, search,
// delete, sign, verify, and show) through the same service and result mapping as `atmos
// artifacts`. It never shells out to the CLI. Available as a component lifecycle hook through the
// existing `kind: step` bridge (`kind: step` / `type: artifact` / flat fields under `with:`); no
// dedicated hook kind is registered for it.
//
// The handler does not retry: a wrapper that owns the step's `retry:` (the workflow runner, a
// custom command, or a hook) calls it again, and the retained operation identity makes every
// attempt the same logical operation. When `retry:` is set, the handler also turns off the
// service's own transport retries for the call, so the two budgets never multiply.
type ArtifactHandler struct {
	BaseHandler
}

func init() {
	Register(&ArtifactHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeArtifact, CategoryCommand, false),
	})
}

// artifactServiceFactory builds the artifact service. It is a variable so tests can inject a fake.
var artifactServiceFactory invoke.ServiceFactory = artifactwire.NewService

// UseArtifactServiceFactory replaces the factory that builds the artifact service for artifact
// steps and returns a function that restores the previous one. It is the seam tests use to run a
// step against an injected service; production code never calls it.
func UseArtifactServiceFactory(factory invoke.ServiceFactory) (restore func()) {
	defer perf.Track(nil, "step.UseArtifactServiceFactory")()

	previous := artifactServiceFactory
	artifactServiceFactory = factory
	return func() { artifactServiceFactory = previous }
}

// Execute runs the configured artifact action. The returned result is non-nil whenever the
// operation produced one, even alongside an error, so a failed operation still reports the
// identity and whatever the service resolved before it failed.
func (h *ArtifactHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ArtifactHandler.Execute")()

	params, err := h.artifactParams(step, vars)
	if err != nil {
		return nil, err
	}
	if err := checkResolvedArtifactParams(step, params); err != nil {
		return nil, err
	}
	if h.applyDryRun(step, params) {
		ui.Infof("Dry run: skipped artifact %s in step **%s**.", params.Action, step.Name)
		return NewStepResult("").WithSkipped().WithMetadata("action", string(params.Action)), nil
	}
	if vars.AtmosConfig == nil {
		return nil, errUtils.Build(errUtils.ErrArtifactConfigInvalid).
			WithContext("step", step.Name).
			WithExplanation("Atmos configuration is not loaded, so the configured artifact repositories are unknown").
			Err()
	}
	svc, err := artifactServiceFactory(vars.AtmosConfig, invoke.ServiceOptions{})
	if err != nil {
		return nil, err
	}
	// The step's `timeout:` limits each attempt: the handler does not retry, so a wrapper's retry
	// loop starts a fresh attempt, with a fresh limit, after a failure.
	deadline, err := StartStepDeadline(ctx, step, vars)
	if err != nil {
		return nil, err
	}
	defer deadline.Stop()

	result, runErr := h.runOperation(deadline.Context(), step, vars, svc, params)
	return result, deadline.Wrap(runErr)
}

// runOperation runs one attempt of the operation under the retained logical identity, records the
// attempt into the report collector, and builds the step result.
func (h *ArtifactHandler) runOperation(ctx context.Context, step *schema.WorkflowStep, vars *Variables, svc artifact.Service, params *invoke.Params) (*StepResult, error) {
	session := vars.artifactSession()
	key := artifactOperationKey(step.Name, params)
	operation, collector := session.begin(key, vars.artifactScopeOrRoot())
	params.Execution = artifact.Execution{OperationID: operation.id, PreparedAt: operation.preparedAt}
	if step.Retry != nil {
		// The wrapper owns the retry budget, so the service makes a single transport attempt.
		params.Execution.MaxAttempts = 1
	}

	outcome, runErr := invoke.Run(ctx, svc, params)
	if runErr != nil && ctx.Err() != nil && !errors.Is(runErr, ctx.Err()) {
		// Keep the cancellation reachable so a retrying wrapper stops instead of starting another attempt.
		runErr = errors.Join(ctx.Err(), runErr)
	}
	session.finish(key, runErr == nil)
	collector.Record(&report.Entry{Step: step.Name, OperationID: operation.id, Action: params.Action, Attempt: operation.attempts, Position: operation.position, Outcome: outcome, Err: runErr})

	result := artifactStepResult(params, outcome)
	if runErr != nil {
		return result, artifactStepError(step, runErr)
	}
	return result, nil
}

// WrapperRetriesStep reports whether the step type leaves its `retry:` policy to the code that
// calls the handler. Such a caller must apply the policy around the step; the handler keeps the
// operation identity stable between the attempts.
func WrapperRetriesStep(stepType string) bool {
	defer perf.Track(nil, "step.WrapperRetriesStep")()

	return strings.TrimSpace(stepType) == schema.TaskTypeArtifact
}

// applyDryRun applies a workflow-level `--dry-run`. A publication switches to the service's own
// dry run, which validates and plans without writing; the actions that would mutate state without
// a dry run of their own are skipped; reads run normally. It reports whether to skip the step.
func (h *ArtifactHandler) applyDryRun(step *schema.WorkflowStep, params *invoke.Params) bool {
	if !step.DryRun {
		return false
	}
	switch params.Action {
	case artifact.ActionPublish:
		params.DryRun = true
	case artifact.ActionPull, artifact.ActionCopy, artifact.ActionDelete, artifact.ActionSign:
		return true
	}
	return false
}

// artifactStepResult builds the step result from an operation outcome: the typed payload under the
// `artifact` metadata key, the resolved paths, and the named string outputs. Optional outputs the
// operation did not produce are omitted, never filled with empty strings.
func artifactStepResult(params *invoke.Params, outcome *invoke.Outcome) *StepResult {
	result := NewStepResult("").
		WithMetadata("action", string(params.Action)).
		WithMetadata(ArtifactMetadataKey, outcome.Payload())
	if params.Source != "" {
		result.WithMetadata("source", params.Source)
	}
	if params.Destination != "" {
		result.WithMetadata("destination", params.Destination)
	}
	if outcome.Collection != nil {
		result.Value = strconv.Itoa(len(outcome.Collection.Artifacts))
		return result.WithMetadata(ArtifactListMetadataKey, outcome.Collection.Artifacts)
	}
	op := &outcome.Result.OperationResult
	result.Value = op.Ref
	for _, pair := range invoke.OutputPairs(op) {
		result.WithOutput(pair.Name, pair.Value)
	}
	return result
}

// artifactStepError adds the step context and the hints that fit a step to an operation failure,
// keeping the original error chain intact for errors.Is.
func artifactStepError(step *schema.WorkflowStep, err error) error {
	builder := errUtils.Build(err).WithContext("step", step.Name).WithContext("action", step.Action)
	switch {
	case errors.Is(err, errUtils.ErrArtifactConflict):
		builder = builder.WithHint("Set `force: true` on the step to replace the existing reference. Force never bypasses verification or immutable release formats.")
	case errors.Is(err, errUtils.ErrArtifactNotFound):
		builder = builder.WithHint("Add a step with `action: list` to see the published artifacts and versions.")
	case errors.Is(err, errUtils.ErrArtifactVerificationFailed):
		builder = builder.WithHint("Configure `artifacts.verification` with the signer identities or keys you trust.")
	}
	return builder.Err()
}
