package exec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifactwire/wiretest"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// artifactWorkflow loads a workflow from YAML, so the test exercises the same decoding as a
// workflow file.
func artifactWorkflow(t *testing.T, manifest string) (*schema.AtmosConfiguration, *schema.WorkflowDefinition, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "artifacts.yaml")
	require.NoError(t, os.WriteFile(path, []byte(manifest), 0o600))
	definitions, err := LoadWorkflowConfig(&schema.AtmosConfiguration{}, path)
	require.NoError(t, err)
	definition := definitions["publish"]
	return &schema.AtmosConfiguration{BasePath: dir}, &definition, path
}

// yamlPath renders a path for an unquoted-safe, double-quoted YAML scalar on every platform.
func yamlPath(path string) string {
	return strings.ReplaceAll(filepath.ToSlash(path), `"`, `\"`)
}

func TestWorkflowArtifactStepMatchesTheSharedImplementation(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	f := wiretest.New(t)
	config, definition, path := artifactWorkflow(t, `
workflows:
  publish:
    steps:
      - name: publish
        type: artifact
        action: publish
        source: "`+yamlPath(f.Source)+`"
        repository: dir
        artifact_name: team/app
        version: "1.0.0"
        metadata:
          labels:
            team: platform
`)
	require.NoError(t, ExecuteWorkflow(*config, "publish", path, definition, false, "", "", ""))

	result, ok := stepExecutorState.GetResult("publish")
	require.True(t, ok)
	payload, ok := result.Metadata[stepPkg.ArtifactMetadataKey].(*invoke.Result)
	require.True(t, ok, "the typed result is stored under Metadata[artifact]")

	baseline := f.Baseline(t)
	assert.Equal(t, f.Normalize(&baseline.OperationResult), f.Normalize(&payload.OperationResult), "same result as the CLI implementation")
	assert.Equal(t, baseline.Record, payload.Record)
	assert.Equal(t, invoke.NamedOutputs(&payload.OperationResult), result.Outputs, "named outputs are the result's named fields")
	assert.Equal(t, artifact.OutcomeCreated, payload.Outcome)
	assert.True(t, strings.HasPrefix(payload.Location, f.Root), "the repository root is the configured one: %s", payload.Location)

	requests := f.Recorder.Requests()
	require.Len(t, requests, 1)
	// The workflow declares the absolute source with slashes (see yamlPath), so compare on that form.
	assert.Equal(t, filepath.ToSlash(f.Source), filepath.ToSlash(requests[0].Source))
	assert.Equal(t, wiretest.OperationID, requests[0].Execution.OperationID)
	assert.True(t, wiretest.PreparedAt.Equal(requests[0].Execution.PreparedAt))
}

func TestWorkflowArtifactStepRetryKeepsTheOperationIdentity(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	f := wiretest.New(t)
	f.Recorder.FailPublications(2, errors.New("temporary registry failure"))
	config, definition, path := artifactWorkflow(t, `
workflows:
  publish:
    steps:
      - name: publish
        type: artifact
        action: publish
        source: "`+yamlPath(f.Source)+`"
        repository: dir
        artifact_name: team/app
        version: "1.0.0"
        retry:
          max_attempts: 3
          backoff_strategy: constant
          initial_delay: 1ms
`)
	require.NoError(t, ExecuteWorkflow(*config, "publish", path, definition, false, "", "", ""))

	requests := f.Recorder.Requests()
	require.Len(t, requests, 3, "the runner's retry loop owns the attempts")
	for _, request := range requests {
		assert.Equal(t, requests[0].Execution, request.Execution, "every attempt is the same logical operation")
		assert.Equal(t, 1, request.Execution.MaxAttempts, "the service's transport retries do not multiply the runner's")
	}
	results := f.Recorder.Results()
	require.Len(t, results, 3)
	assert.Empty(t, results[0].Outcome, "a failed attempt carries no outcome")
	assert.Equal(t, artifact.OutcomeCreated, results[2].Outcome)
	for _, result := range results {
		assert.Equal(t, wiretest.OperationID, result.OperationID)
	}
	result, ok := stepExecutorState.GetResult("publish")
	require.True(t, ok)
	assert.Equal(t, "created", result.Outputs["outcome"], "the final outcome is what later steps see")
}

func TestWorkflowArtifactStepFailureStopsOrContinues(t *testing.T) {
	broken := errors.New("repository unavailable")
	manifest := func(f *wiretest.Fixture, continueField string) string {
		return `
workflows:
  publish:
    steps:
      - name: publish
        type: artifact
        action: publish
        source: "` + yamlPath(f.Source) + `"
        repository: dir
        artifact_name: team/app
        version: "1.0.0"
` + continueField + `
      - name: inspect
        type: artifact
        action: inspect
        repository: dir
        ref: "team/app:1.0.0"
`
	}

	t.Run("a failure stops the workflow and later steps never run", func(t *testing.T) {
		ResetStepExecutorState()
		t.Cleanup(ResetStepExecutorState)
		f := wiretest.New(t)
		f.Recorder.FailPublications(1, broken)
		config, definition, path := artifactWorkflow(t, manifest(f, ""))
		err := ExecuteWorkflow(*config, "publish", path, definition, false, "", "", "")
		require.ErrorIs(t, err, broken)
		_, inspected := stepExecutorState.GetResult("inspect")
		assert.False(t, inspected)
		assert.Len(t, f.Recorder.Requests(), 1)
	})

	t.Run("continue forgives the failure and the next step runs", func(t *testing.T) {
		ResetStepExecutorState()
		t.Cleanup(ResetStepExecutorState)
		f := wiretest.New(t)
		f.Recorder.FailPublications(1, broken)
		config, definition, path := artifactWorkflow(t, manifest(f, "        continue: always"))
		// The inspect step fails with not found, because the publication failed; it is the
		// workflow's own last step, so the workflow fails there rather than at the forgiven step.
		err := ExecuteWorkflow(*config, "publish", path, definition, false, "", "", "")
		require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
		assert.NotErrorIs(t, err, broken, "the forgiven failure is not the workflow's failure")
	})
}

func TestWorkflowArtifactStepCancellationStopsRetries(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)
	f := wiretest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxAttempts := 5
	step := schema.WorkflowStep{
		Name: "publish", Type: schema.TaskTypeArtifact, Action: "publish", Source: f.Source, Repository: wiretest.Alias,
		ArtifactName: wiretest.Name, Version: wiretest.Version,
		Retry: &schema.RetryConfig{MaxAttempts: &maxAttempts, BackoffStrategy: schema.BackoffConstant, InitialDelay: durationPtr(time.Millisecond)},
	}
	f.Recorder.FailPublications(5, errors.New("transfer aborted"))
	attempts := 0
	err := stepPkg.RetryWithConditions(ctx, step.Retry, func() error {
		attempts++
		cancel() // The operator interrupts the run while the first attempt is in flight.
		return executeExtendedStep(ctx, &step, &schema.WorkflowDefinition{}, nil, extendedStepOptions{AtmosConfig: f.Config})
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, attempts, "a canceled operation is not retried")
	assert.Len(t, f.Recorder.Requests(), 1)
	for _, request := range f.Recorder.Requests() {
		assert.Equal(t, wiretest.OperationID, request.Execution.OperationID)
	}
}

func durationPtr(d time.Duration) *time.Duration { return &d }
