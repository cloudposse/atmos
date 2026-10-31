package step

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	testRecordDigest   = digest.Digest("sha256:1111111111111111111111111111111111111111111111111111111111111111")
	testResolvedDigest = digest.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222")
)

// recordingCollector captures every entry a step records.
type recordingCollector struct {
	mu      sync.Mutex
	entries []report.Entry
}

func (c *recordingCollector) Record(entry *report.Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, *entry)
}

// artifactEnv is a step environment with an injected service and a fixed operation identity.
type artifactEnv struct {
	t         *testing.T
	service   *artifact.MockService
	vars      *Variables
	collector *recordingCollector
	dir       string
}

func newArtifactEnv(t *testing.T) *artifactEnv {
	t.Helper()
	ctrl := gomock.NewController(t)
	env := &artifactEnv{t: t, service: artifact.NewMockService(ctrl), vars: NewVariables(), collector: &recordingCollector{}, dir: t.TempDir()}
	env.vars.SetAtmosConfig(&schema.AtmosConfiguration{})
	env.vars.SetArtifactCollector(env.collector)

	restoreFactory := UseArtifactServiceFactory(func(*schema.AtmosConfiguration, invoke.ServiceOptions) (artifact.Service, error) {
		return env.service, nil
	})
	t.Cleanup(restoreFactory)

	previousNow, previousID := artifactNow, artifactNewID
	ticks, ids := 0, 0
	artifactNow = func() time.Time {
		ticks++
		return time.Date(2026, 10, 31, 12, 0, ticks, 0, time.UTC)
	}
	artifactNewID = func() string {
		ids++
		return "op-" + string(rune('0'+ids))
	}
	t.Cleanup(func() { artifactNow, artifactNewID = previousNow, previousID })
	return env
}

func (e *artifactEnv) run(step *schema.WorkflowStep) (*StepResult, error) {
	e.t.Helper()
	step.WorkingDirectory = e.dir
	handler := &ArtifactHandler{}
	require.NoError(e.t, handler.Validate(step))
	return handler.Execute(context.Background(), step, e.vars)
}

func completeOperation(action artifact.Action, outcome artifact.Outcome) *artifact.OperationResult {
	return &artifact.OperationResult{
		OperationID: "op-1", Action: action, Outcome: outcome, Repository: "dir", Requested: "team/app:1.0.0",
		Ref: "team/app:1.0.0", Location: "/repo/team/app/1.0.0", ResolvedDigest: testResolvedDigest, RecordDigest: testRecordDigest,
		ContentDigest: testRecordDigest, Size: 42,
	}
}

func TestArtifactHandler_PublishMapsTheStepAndReportsNamedOutputs(t *testing.T) {
	env := newArtifactEnv(t)
	var request artifact.PublishRequest
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		request = r
		return completeOperation(artifact.ActionPublish, artifact.OutcomeCreated), nil
	})

	step := artifactStepWith(artifact.ActionPublish, invoke.FieldRepository, invoke.FieldArtifactName, invoke.FieldVersion, invoke.FieldArtifactKind,
		invoke.FieldForce, invoke.FieldMetadata, invoke.FieldReadme, invoke.FieldChangelog)
	step.Source = "./dist"
	step.Version = "{{ .env.APP_VERSION }}"
	env.vars.SetEnv("APP_VERSION", "2.0.0")

	result, err := env.run(step)
	require.NoError(t, err)

	assert.Equal(t, "dir", request.Repository)
	assert.Equal(t, filepath.Join(env.dir, "dist"), request.Source, "source is anchored to the step working directory")
	assert.Equal(t, "team/app", request.Name)
	assert.Equal(t, "2.0.0", request.Version, "templates are rendered")
	assert.Equal(t, "files", request.Kind)
	assert.Equal(t, map[string]string{"team": "platform"}, request.Metadata.Labels)
	assert.Equal(t, "README.md", request.Readme, "document paths stay relative to the source, as --readme does")
	assert.Equal(t, "CHANGELOG.md", request.Changelog)
	assert.True(t, request.Force)
	assert.False(t, request.DryRun)
	assert.Equal(t, "op-1", request.Execution.OperationID)
	assert.False(t, request.Execution.PreparedAt.IsZero())
	assert.Zero(t, request.Execution.MaxAttempts, "without retry the service keeps its transport retries")

	assert.Equal(t, map[string]string{
		"outcome": "created", "ref": "team/app:1.0.0", "location": "/repo/team/app/1.0.0",
		"resolved_digest": string(testResolvedDigest), "record_digest": string(testRecordDigest),
		"content_digest": string(testRecordDigest), "size": "42",
	}, result.Outputs, "optional outputs the operation did not produce (representation_digest) are omitted")
	assert.Equal(t, "team/app:1.0.0", result.Value)
	assert.Equal(t, filepath.Join(env.dir, "dist"), result.Metadata["source"])

	payload, ok := result.Metadata[ArtifactMetadataKey].(*invoke.Result)
	require.True(t, ok, "the typed result is stored under Metadata[artifact]")
	assert.Equal(t, artifact.OutcomeCreated, payload.Outcome)
	assert.Equal(t, "op-1", payload.OperationID)
}

func TestArtifactHandler_ActionsMapToServiceCalls(t *testing.T) {
	tests := []struct {
		name   string
		step   func() *schema.WorkflowStep
		expect func(*artifactEnv)
		check  func(*testing.T, *StepResult)
	}{
		{
			name: "pull anchors the destination",
			step: func() *schema.WorkflowStep {
				return artifactStepWith(artifact.ActionPull, invoke.FieldRepository, invoke.FieldRef, invoke.FieldDestination, invoke.FieldForce)
			},
			expect: func(e *artifactEnv) {
				e.service.EXPECT().Pull(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PullRequest) (*artifact.OperationResult, error) {
					assert.Equal(t, filepath.Join(e.dir, "out"), r.Destination)
					assert.Equal(t, artifact.Ref{Name: "team/app", Version: "1.0.0"}, r.Ref)
					assert.True(t, r.Force)
					return completeOperation(artifact.ActionPull, artifact.OutcomeRead), nil
				})
			},
			check: func(t *testing.T, r *StepResult) { assert.Equal(t, "read", r.Outputs["outcome"]) },
		},
		{
			name: "copy",
			step: func() *schema.WorkflowStep {
				return artifactStepWith(artifact.ActionCopy, invoke.FieldFrom, invoke.FieldTo, invoke.FieldRef)
			},
			expect: func(e *artifactEnv) {
				e.service.EXPECT().Copy(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.CopyRequest) (*artifact.OperationResult, error) {
					assert.Equal(t, "dir", r.From)
					assert.Equal(t, "archive", r.To)
					return completeOperation(artifact.ActionCopy, artifact.OutcomeCreated), nil
				})
			},
		},
		{
			name: "delete",
			step: func() *schema.WorkflowStep {
				return artifactStepWith(artifact.ActionDelete, invoke.FieldRepository, invoke.FieldRef)
			},
			expect: func(e *artifactEnv) {
				e.service.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(completeOperation(artifact.ActionDelete, artifact.OutcomeDeleted), nil)
			},
			check: func(t *testing.T, r *StepResult) { assert.Equal(t, "deleted", r.Outputs["outcome"]) },
		},
		{
			name: "inspect",
			step: func() *schema.WorkflowStep {
				return artifactStepWith(artifact.ActionInspect, invoke.FieldRepository, invoke.FieldRef)
			},
			expect: func(e *artifactEnv) {
				e.service.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(&artifact.InspectResult{
					Operation: *completeOperation(artifact.ActionInspect, artifact.OutcomeRead),
				}, nil)
			},
			check: func(t *testing.T, r *StepResult) {
				payload := r.Metadata[ArtifactMetadataKey].(*invoke.Result)
				assert.NotNil(t, payload.Inspection)
			},
		},
		{
			name: "show",
			step: func() *schema.WorkflowStep {
				return artifactStepWith(artifact.ActionShow, invoke.FieldRepository, invoke.FieldRef, invoke.FieldDocument)
			},
			expect: func(e *artifactEnv) {
				e.service.EXPECT().ShowDocument(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.ShowDocumentRequest) (*artifact.DocumentResult, error) {
					assert.Equal(t, artifact.DocumentReadme, r.Role)
					return &artifact.DocumentResult{
						Operation: *completeOperation(artifact.ActionShow, artifact.OutcomeRead),
						Document:  artifact.Document{Role: artifact.DocumentReadme, Digest: testRecordDigest},
						Content:   []byte("# App"),
					}, nil
				})
			},
			check: func(t *testing.T, r *StepResult) {
				payload := r.Metadata[ArtifactMetadataKey].(*invoke.Result)
				require.NotNil(t, payload.Content)
				assert.Equal(t, "# App", *payload.Content)
			},
		},
		{
			name: "sign",
			step: func() *schema.WorkflowStep {
				step := artifactStepWith(artifact.ActionSign, invoke.FieldRepository)
				step.Ref = "team/app@" + string(testRecordDigest)
				return step
			},
			expect: func(e *artifactEnv) {
				e.service.EXPECT().Sign(gomock.Any(), gomock.Any()).Return(&artifact.SignResult{
					Operation: *completeOperation(artifact.ActionSign, artifact.OutcomeCreated),
					Subject:   artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: testRecordDigest},
				}, nil)
			},
			check: func(t *testing.T, r *StepResult) {
				assert.NotNil(t, r.Metadata[ArtifactMetadataKey].(*invoke.Result).Subject)
			},
		},
		{
			name: "verify",
			step: func() *schema.WorkflowStep {
				return artifactStepWith(artifact.ActionVerify, invoke.FieldRepository, invoke.FieldRef)
			},
			expect: func(e *artifactEnv) {
				e.service.EXPECT().Verify(gomock.Any(), gomock.Any()).Return(&artifact.VerifyResult{
					Operation:    *completeOperation(artifact.ActionVerify, artifact.OutcomeRead),
					Verification: artifact.VerificationResult{Status: artifact.VerificationVerified},
				}, nil)
			},
			check: func(t *testing.T, r *StepResult) {
				assert.Equal(t, artifact.VerificationVerified, r.Metadata[ArtifactMetadataKey].(*invoke.Result).Verification.Status)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newArtifactEnv(t)
			tc.expect(env)
			result, err := env.run(tc.step())
			require.NoError(t, err)
			require.NotNil(t, result.Metadata[ArtifactMetadataKey])
			if tc.check != nil {
				tc.check(t, result)
			}
		})
	}
}

func TestArtifactHandler_ListAndSearchReportAnArtifactsArray(t *testing.T) {
	entries := []artifact.ListEntry{
		{Repository: "dir", Name: "team/app", Version: "1.0.0", Kind: "files", Pinned: artifact.PinnedReference{RecordDigest: testRecordDigest}},
		{Repository: "archive", Name: "team/app", Version: "2.0.0", Kind: "files"},
	}
	t.Run("list parses the label selector forms and the repository list", func(t *testing.T) {
		for _, labels := range []any{"team=platform,env:dev", []any{"team=platform", "env=dev"}, map[string]any{"team": "platform", "env": "dev"}} {
			env := newArtifactEnv(t)
			env.service.EXPECT().List(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.ListRequest) (*artifact.ListResult, error) {
				assert.Equal(t, []string{"dir", "archive"}, r.Repositories)
				assert.Equal(t, "team/app", r.Name)
				assert.Equal(t, []artifact.Label{{Key: "env", Value: "dev"}, {Key: "team", Value: "platform"}}, r.Labels, "selectors are sorted")
				assert.True(t, r.Refresh)
				return &artifact.ListResult{Artifacts: entries}, nil
			})
			step := artifactStepWith(artifact.ActionList, invoke.FieldArtifactName, invoke.FieldRefresh)
			step.Repository = "dir, archive"
			step.Labels = labels
			result, err := env.run(step)
			require.NoError(t, err)

			collection, ok := result.Metadata[ArtifactMetadataKey].(*invoke.Collection)
			require.True(t, ok)
			require.Len(t, collection.Artifacts, 2)
			assert.Equal(t, "team/app:1.0.0", collection.Artifacts[0].Ref)
			assert.Equal(t, "team/app:2.0.0", collection.Artifacts[1].Ref)
			assert.Equal(t, collection.Artifacts, result.Metadata[ArtifactListMetadataKey])
			assert.Equal(t, "2", result.Value)
			assert.Empty(t, result.Outputs, "a collection has no single-artifact named outputs")
		}
	})
	t.Run("search passes the text", func(t *testing.T) {
		env := newArtifactEnv(t)
		env.service.EXPECT().Search(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.SearchRequest) (*artifact.ListResult, error) {
			assert.Equal(t, "app", r.Query)
			assert.Empty(t, r.Repositories, "no repository means every configured repository")
			return &artifact.ListResult{}, nil
		})
		result, err := env.run(artifactStepWith(artifact.ActionSearch, invoke.FieldQuery))
		require.NoError(t, err)
		collection := result.Metadata[ArtifactMetadataKey].(*invoke.Collection)
		assert.NotNil(t, collection.Artifacts, "an empty listing encodes as an empty array, not null")
		assert.Empty(t, collection.Artifacts)
	})
}

func TestArtifactHandler_FailureReturnsThePartialResult(t *testing.T) {
	env := newArtifactEnv(t)
	partial := &artifact.OperationResult{OperationID: "op-1", Action: artifact.ActionPublish, Repository: "dir", Ref: "team/app:1.0.0", RecordDigest: testRecordDigest}
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(partial, errUtils.ErrArtifactConflict)

	result, err := env.run(artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository))
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	require.NotNil(t, result, "a failed operation still returns what it learned")

	payload := result.Metadata[ArtifactMetadataKey].(*invoke.Result)
	assert.Equal(t, artifact.OutcomeFailed, payload.Outcome, "a failure is never reported as a success")
	assert.Contains(t, payload.Error, "conflict")
	assert.Equal(t, testRecordDigest, payload.RecordDigest)
	assert.Equal(t, "failed", result.Outputs["outcome"])
	assert.Equal(t, string(testRecordDigest), result.Outputs["record_digest"])
	assert.NotContains(t, result.Outputs, "location", "values the operation never resolved are omitted")

	require.Len(t, env.collector.entries, 1)
	assert.Error(t, env.collector.entries[0].Err)
	assert.Equal(t, "op-1", env.collector.entries[0].OperationID)
}

// TestArtifactHandler_RetryKeepsTheOperationIdentity covers the wrapper-level retry contract: the
// wrapper calls the handler again with the same Variables, the identity survives the failure, a
// success ends the operation, and the service's own transport retries are off while retry is set.
func TestArtifactHandler_RetryKeepsTheOperationIdentity(t *testing.T) {
	env := newArtifactEnv(t)
	var executions []artifact.Execution
	temporary := errors.New("temporary registry failure")
	calls := 0
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		executions = append(executions, r.Execution)
		calls++
		op := completeOperation(artifact.ActionPublish, artifact.OutcomeCreated)
		op.OperationID = r.Execution.OperationID
		if calls < 3 {
			return &artifact.OperationResult{OperationID: r.Execution.OperationID, Action: artifact.ActionPublish}, temporary
		}
		return op, nil
	}).Times(4)

	step := artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository)
	step.Retry = &schema.RetryConfig{}

	for attempt := 1; attempt <= 3; attempt++ {
		_, err := env.run(step)
		if attempt < 3 {
			require.ErrorIs(t, err, temporary)
		} else {
			require.NoError(t, err)
		}
	}
	require.Len(t, executions, 3)
	for _, execution := range executions {
		assert.Equal(t, executions[0].OperationID, execution.OperationID, "every attempt is the same logical operation")
		assert.Equal(t, executions[0].PreparedAt, execution.PreparedAt, "the preparation time is captured once")
		assert.Equal(t, 1, execution.MaxAttempts, "the wrapper owns the retry budget")
	}

	// The operation ended with the success; running the step again is a new operation.
	_, err := env.run(step)
	require.NoError(t, err)
	require.Len(t, executions, 4)
	assert.NotEqual(t, executions[0].OperationID, executions[3].OperationID)
	assert.NotEqual(t, executions[0].PreparedAt, executions[3].PreparedAt)

	require.Len(t, env.collector.entries, 4)
	for index, want := range []int{1, 2, 3, 1} {
		assert.Equal(t, want, env.collector.entries[index].Attempt)
	}
	assert.Equal(t, env.collector.entries[0].OperationID, env.collector.entries[2].OperationID, "a report deduplicates attempts by operation ID")
}

func TestArtifactHandler_DifferentInputIsADifferentOperation(t *testing.T) {
	env := newArtifactEnv(t)
	var ids []string
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		ids = append(ids, r.Execution.OperationID)
		return nil, errors.New("fail")
	}).Times(2)
	first := artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository, invoke.FieldVersion)
	second := artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository, invoke.FieldVersion)
	second.Version = "2.0.0"
	_, _ = env.run(first)
	_, _ = env.run(second)
	require.Len(t, ids, 2)
	assert.NotEqual(t, ids[0], ids[1])
}

func TestArtifactHandler_CancellationStaysReachable(t *testing.T) {
	env := newArtifactEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, artifact.PublishRequest) (*artifact.OperationResult, error) {
		cancel()
		return nil, errors.New("transfer aborted")
	})
	step := artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository)
	step.WorkingDirectory = env.dir
	result, err := (&ArtifactHandler{}).Execute(ctx, step, env.vars)
	require.ErrorIs(t, err, context.Canceled, "a retrying wrapper must be able to see the cancellation and stop")
	assert.NotNil(t, result)
}

func TestArtifactHandler_WorkflowDryRun(t *testing.T) {
	t.Run("publish switches to the service dry run", func(t *testing.T) {
		env := newArtifactEnv(t)
		env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
			assert.True(t, r.DryRun)
			return completeOperation(artifact.ActionPublish, artifact.OutcomeDryRun), nil
		})
		step := artifactStepWith(artifact.ActionPublish, invoke.FieldSource, invoke.FieldRepository)
		step.DryRun = true
		_, err := env.run(step)
		require.NoError(t, err)
	})
	t.Run("mutating actions without a dry run are skipped", func(t *testing.T) {
		for _, action := range []artifact.Action{artifact.ActionPull, artifact.ActionCopy, artifact.ActionDelete, artifact.ActionSign} {
			env := newArtifactEnv(t)
			spec, _ := invoke.SpecFor(action)
			step := artifactStepWith(action, spec.Required...)
			step.DryRun = true
			result, err := env.run(step)
			require.NoError(t, err, action)
			assert.True(t, result.Skipped, action)
			assert.Empty(t, env.collector.entries, "a skipped step reports nothing")
		}
	})
	t.Run("reads run normally", func(t *testing.T) {
		env := newArtifactEnv(t)
		env.service.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(0)
		env.service.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(&artifact.InspectResult{Operation: *completeOperation(artifact.ActionInspect, artifact.OutcomeRead)}, nil)
		step := artifactStepWith(artifact.ActionInspect, invoke.FieldRepository, invoke.FieldRef)
		step.DryRun = true
		result, err := env.run(step)
		require.NoError(t, err)
		assert.False(t, result.Skipped)
	})
}

func TestArtifactHandler_ReportsConfigurationAndTemplateProblems(t *testing.T) {
	t.Run("empty required value after rendering", func(t *testing.T) {
		env := newArtifactEnv(t)
		step := artifactStepWith(artifact.ActionInspect, invoke.FieldRepository)
		step.Ref = `{{ "" }}`
		_, err := env.run(step)
		require.ErrorIs(t, err, errUtils.ErrArtifactStepFieldRequired)
		assert.Contains(t, err.Error(), "ref")
	})
	t.Run("template error names the field", func(t *testing.T) {
		env := newArtifactEnv(t)
		step := artifactStepWith(artifact.ActionInspect, invoke.FieldRepository)
		step.Ref = "{{ .steps.nope.value | bogus }}"
		_, err := env.run(step)
		require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	})
	t.Run("missing atmos configuration", func(t *testing.T) {
		env := newArtifactEnv(t)
		env.vars.AtmosConfig = nil
		_, err := env.run(artifactStepWith(artifact.ActionInspect, invoke.FieldRepository, invoke.FieldRef))
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
	t.Run("service construction failure", func(t *testing.T) {
		env := newArtifactEnv(t)
		restore := UseArtifactServiceFactory(func(*schema.AtmosConfiguration, invoke.ServiceOptions) (artifact.Service, error) {
			return nil, errUtils.ErrArtifactConfigInvalid
		})
		defer restore()
		_, err := env.run(artifactStepWith(artifact.ActionInspect, invoke.FieldRepository, invoke.FieldRef))
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
	t.Run("invalid ref surfaces as a failed result", func(t *testing.T) {
		env := newArtifactEnv(t)
		step := artifactStepWith(artifact.ActionInspect, invoke.FieldRepository)
		step.Ref = "no-version"
		result, err := env.run(step)
		require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
		assert.Equal(t, "failed", result.Outputs["outcome"])
		assert.Equal(t, "no-version", result.Metadata[ArtifactMetadataKey].(*invoke.Result).Requested)
	})
}

func TestArtifactHandler_TimeoutLimitsEachAttempt(t *testing.T) {
	env := newArtifactEnv(t)
	env.service.EXPECT().Delete(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ artifact.DeleteRequest) (*artifact.OperationResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	step := artifactStepWith(artifact.ActionDelete, invoke.FieldRepository, invoke.FieldRef)
	step.Timeout = "20ms"
	_, err := env.run(step)
	require.ErrorIs(t, err, errUtils.ErrStepTimeout)
}

func TestArtifactHandler_UsesPriorStepOutputsInTemplates(t *testing.T) {
	env := newArtifactEnv(t)
	env.vars.Set("publish", NewStepResult("").WithOutput("ref", "team/app:1.0.0"))
	env.service.EXPECT().Inspect(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.InspectRequest) (*artifact.InspectResult, error) {
		assert.Equal(t, "team/app", r.Ref.Name)
		return &artifact.InspectResult{Operation: *completeOperation(artifact.ActionInspect, artifact.OutcomeRead)}, nil
	})
	step := artifactStepWith(artifact.ActionInspect, invoke.FieldRepository)
	step.Ref = "{{ .steps.publish.outputs.ref }}"
	_, err := env.run(step)
	require.NoError(t, err)
}

// TestArtifactHandler_ScriptDispatch runs the step through a direct step call, as an embedded
// script does with steps.artifact(...), and checks it reaches the same service with the same input.
func TestArtifactHandler_ScriptDispatch(t *testing.T) {
	env := newArtifactEnv(t)
	var request artifact.PublishRequest
	env.service.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		request = r
		return completeOperation(artifact.ActionPublish, artifact.OutcomeCreated), nil
	})
	library := NewAutomationLibrary(env.vars, nil)
	call := &api.StepCall{Type: "artifact", Configuration: map[string]any{
		"name": "publish", "action": "publish", "source": filepath.Join(env.dir, "dist"), "repository": "dir",
		"artifact_name": "team/app", "version": "1.0.0", "metadata": map[string]any{"labels": map[string]any{"team": "platform"}},
	}}
	require.NoError(t, library.Validate(call))
	result, err := library.Run(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, "created", result.Outputs["outcome"])
	assert.Equal(t, filepath.Join(env.dir, "dist"), request.Source)
	assert.Equal(t, map[string]string{"team": "platform"}, request.Metadata.Labels)

	t.Run("a field the artifact step does not read is rejected", func(t *testing.T) {
		err := library.Validate(&api.StepCall{Type: "artifact", Configuration: map[string]any{"action": "inspect", "duration": "5s"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duration")
	})
	t.Run("the search text is a string query", func(t *testing.T) {
		env.service.EXPECT().Search(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.SearchRequest) (*artifact.ListResult, error) {
			assert.Equal(t, "vpc", r.Query)
			return &artifact.ListResult{}, nil
		})
		_, err := library.Run(context.Background(), &api.StepCall{Type: "artifact", Configuration: map[string]any{"action": "search", "query": "vpc"}})
		require.NoError(t, err)
	})
}

func TestWrapperRetriesStep(t *testing.T) {
	assert.True(t, WrapperRetriesStep("artifact"))
	assert.True(t, WrapperRetriesStep(" artifact "))
	for _, other := range []string{"shell", "http", "archive", "script", ""} {
		assert.False(t, WrapperRetriesStep(other), other)
	}
}
