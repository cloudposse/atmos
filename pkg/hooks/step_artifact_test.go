package hooks

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifactwire/wiretest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// artifactHooks builds a component's hooks with one artifact step hook, as the stack manifest
// would declare it:
//
//	hooks:
//	  backup:
//	    events: [after-terraform-apply]
//	    kind: step
//	    type: artifact
//	    with: {action: publish, source: ..., repository: dir, ...}
func artifactHooks(f *wiretest.Fixture, mutate func(*Hook)) *Hooks {
	hook := Hook{
		Kind:   stepKindName,
		Type:   "artifact",
		Events: []string{string(AfterTerraformApply)},
		With:   f.StepFields(),
	}
	if mutate != nil {
		mutate(&hook)
	}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev", ComponentType: "terraform"}
	return &Hooks{config: f.Config, info: info, items: map[string]Hook{"backup": hook}}
}

func retryOf(attempts int) *schema.RetryConfig {
	delay := time.Millisecond
	return &schema.RetryConfig{MaxAttempts: &attempts, BackoffStrategy: schema.BackoffConstant, InitialDelay: &delay}
}

func TestArtifactStepHookMatchesTheSharedImplementation(t *testing.T) {
	f := wiretest.New(t)
	h := artifactHooks(f, nil)

	require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))

	results := f.Recorder.Results()
	require.Len(t, results, 1)
	baseline := f.Baseline(t)
	assert.Equal(t, f.Normalize(&baseline.OperationResult), f.Normalize(&results[0]), "the hook reports what the CLI implementation reports")
	assert.Equal(t, artifact.OutcomeCreated, results[0].Outcome)
	assert.Contains(t, results[0].Location, f.Root, "the repository root is the configured one")

	requests := f.Recorder.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, f.Source, requests[0].Source, "an absolute source stays as written")
	assert.Equal(t, map[string]string{"team": "platform"}, requests[0].Metadata.Labels)
	assert.Zero(t, requests[0].Execution.MaxAttempts, "no retry: the service keeps its transport retries")
}

func TestArtifactStepHookResolvesRelativeSourceAgainstTheComponentDirectory(t *testing.T) {
	f := wiretest.New(t)
	component := filepath.Join(t.TempDir(), "components", "terraform", "vpc")
	h := artifactHooks(f, func(hook *Hook) {
		with := f.StepFields()
		with["source"] = "dist"
		with["working_directory"] = component
		hook.With = with
	})
	require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))
	requests := f.Recorder.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, filepath.Join(component, "dist"), requests[0].Source)
}

func TestArtifactStepHookRetryKeepsTheOperationIdentity(t *testing.T) {
	f := wiretest.New(t)
	f.Recorder.FailPublications(2, errors.New("temporary registry failure"))
	h := artifactHooks(f, func(hook *Hook) {
		hook.Retry = retryOf(3)
		hook.OnFailure = OnFailureFail
	})

	require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))

	requests := f.Recorder.Requests()
	require.Len(t, requests, 3)
	for _, request := range requests {
		assert.Equal(t, requests[0].Execution, request.Execution, "every attempt is the same logical operation")
		assert.Equal(t, 1, request.Execution.MaxAttempts, "the hook's retries and the transport retries do not multiply")
	}
	results := f.Recorder.Results()
	require.Len(t, results, 3)
	assert.Equal(t, artifact.OutcomeCreated, results[2].Outcome, "the final attempt reports the outcome")
	assert.Equal(t, wiretest.OperationID, results[0].OperationID)
}

func TestArtifactStepHookFailureModes(t *testing.T) {
	broken := errors.New("repository unavailable")
	tests := []struct {
		name      string
		onFailure string
		wantErr   bool
	}{
		{"fail propagates the error", OnFailureFail, true},
		{"warn logs and continues", OnFailureWarn, false},
		{"ignore swallows the error", OnFailureIgnore, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := wiretest.New(t)
			f.Recorder.FailPublications(1, broken)
			h := artifactHooks(f, func(hook *Hook) { hook.OnFailure = tc.onFailure })
			err := h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil)
			if tc.wantErr {
				require.ErrorIs(t, err, broken)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, f.Recorder.Requests(), 1, "the failed operation was attempted once")
			assert.Empty(t, f.Recorder.Results()[0].Outcome)
		})
	}
}

func TestArtifactStepHookHonorsEventsAndConditions(t *testing.T) {
	t.Run("a different event does not run the hook", func(t *testing.T) {
		f := wiretest.New(t)
		h := artifactHooks(f, nil)
		require.NoError(t, h.RunAll(BeforeTerraformApply, f.Config, h.info, nil, nil))
		assert.Empty(t, f.Recorder.Requests())
	})
	t.Run("the default condition skips the hook after a failed operation", func(t *testing.T) {
		f := wiretest.New(t)
		h := artifactHooks(f, nil)
		h.SetOutcome(Outcome{Status: RunFailure, Err: errors.New("apply failed"), ExitCode: 1})
		require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))
		assert.Empty(t, f.Recorder.Requests())
	})
	t.Run("when: always runs the hook after a failed operation", func(t *testing.T) {
		f := wiretest.New(t)
		h := artifactHooks(f, func(hook *Hook) { hook.When = schema.MustCondition(schema.ConditionPredicateAlways) })
		h.SetOutcome(Outcome{Status: RunFailure, Err: errors.New("apply failed"), ExitCode: 1})
		require.NoError(t, h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil))
		assert.Len(t, f.Recorder.Requests(), 1)
	})
}

func TestArtifactStepHookRejectsInvalidFieldCombinations(t *testing.T) {
	f := wiretest.New(t)
	h := artifactHooks(f, func(hook *Hook) {
		with := f.StepFields()
		with["destination"] = "./out" // pull's field, not publish's.
		hook.With = with
		hook.OnFailure = OnFailureFail
	})
	err := h.RunAll(AfterTerraformApply, f.Config, h.info, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "destination")
	assert.Empty(t, f.Recorder.Requests(), "an invalid step never reaches the service")
}
