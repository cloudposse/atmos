package invoke_test

import (
	"context"
	"errors"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
)

const (
	recordDigest   = digest.Digest("sha256:1111111111111111111111111111111111111111111111111111111111111111")
	resolvedDigest = digest.Digest("sha256:2222222222222222222222222222222222222222222222222222222222222222")
)

func operation(action artifact.Action) *artifact.OperationResult {
	return &artifact.OperationResult{
		OperationID: "op-1", Action: action, Outcome: artifact.OutcomeCreated, Repository: "dir",
		Ref: "team/app:1.0.0", Location: "/repo", ResolvedDigest: resolvedDigest, RecordDigest: recordDigest, Size: 7,
	}
}

func TestSpecCoversEveryCommandAndStepAction(t *testing.T) {
	want := []artifact.Action{
		artifact.ActionCopy, artifact.ActionDelete, artifact.ActionInspect, artifact.ActionList, artifact.ActionPublish,
		artifact.ActionPull, artifact.ActionSearch, artifact.ActionShow, artifact.ActionSign, artifact.ActionVerify,
	}
	assert.Equal(t, want, invoke.Actions())
	_, ok := invoke.SpecFor(artifact.ActionPin)
	assert.False(t, ok, "pin is a service operation, not a command or step action")

	publish, ok := invoke.SpecFor(artifact.ActionPublish)
	require.True(t, ok)
	assert.True(t, publish.Accepts(invoke.FieldMetadata))
	assert.False(t, publish.Accepts(invoke.FieldLabels), "publish labels are metadata.labels; labels select in list and search")
	assert.Equal(t, append([]invoke.Field{invoke.FieldSource, invoke.FieldRepository}, publish.Optional...), publish.Fields())

	list, _ := invoke.SpecFor(artifact.ActionList)
	assert.Empty(t, list.Required, "list and search default to every configured repository")
	assert.Contains(t, invoke.AllFields(), invoke.FieldDocument)
}

func TestOutputPairsOmitOptionalValues(t *testing.T) {
	pairs := invoke.OutputPairs(&artifact.OperationResult{Outcome: artifact.OutcomeUnchanged, Ref: "team/app:1"})
	assert.Equal(t, []invoke.Pair{{Name: "outcome", Value: "unchanged"}, {Name: "ref", Value: "team/app:1"}}, pairs)

	full := invoke.NamedOutputs(operation(artifact.ActionPublish))
	assert.Equal(t, map[string]string{
		"outcome": "created", "ref": "team/app:1.0.0", "location": "/repo", "resolved_digest": string(resolvedDigest),
		"record_digest": string(recordDigest), "size": "7",
	}, full)
	assert.Empty(t, invoke.NamedOutputs(&artifact.OperationResult{}))
}

func TestRunMapsParamsOntoServiceRequests(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := artifact.NewMockService(ctrl)
	execution := artifact.Execution{OperationID: "retained", MaxAttempts: 1}

	svc.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r artifact.PublishRequest) (*artifact.OperationResult, error) {
		assert.Equal(t, artifact.PublishRequest{
			Execution: execution, Repository: "dir", Source: "./dist", Name: "team/app", Version: "1", Kind: "files",
			Metadata: artifact.Metadata{Labels: map[string]string{"a": "b"}}, Readme: "R.md", Changelog: "C.md", Force: true, DryRun: true,
		}, r)
		return operation(artifact.ActionPublish), nil
	})
	outcome, err := invoke.Run(context.Background(), svc, &invoke.Params{
		Action: artifact.ActionPublish, Repository: "dir", Source: "./dist", Name: "team/app", Version: "1", Kind: "files",
		Labels: map[string]string{"a": "b"}, Readme: "R.md", Changelog: "C.md", Force: true, DryRun: true, Execution: execution,
	})
	require.NoError(t, err)
	assert.Nil(t, outcome.Collection)
	assert.Equal(t, outcome.Result, outcome.Payload())
	assert.Equal(t, "op-1", outcome.Result.OperationID)
}

func TestRunReturnsAFailedResultWhenTheReferenceIsInvalid(t *testing.T) {
	svc := artifact.NewMockService(gomock.NewController(t))
	outcome, err := invoke.Run(context.Background(), svc, &invoke.Params{Action: artifact.ActionDelete, Repository: "dir", Ref: "bad"})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
	require.NotNil(t, outcome.Result)
	assert.Equal(t, artifact.OutcomeFailed, outcome.Result.Outcome)
	assert.Equal(t, artifact.ActionDelete, outcome.Result.Action)
	assert.Equal(t, "dir", outcome.Result.Repository)
	assert.Equal(t, "bad", outcome.Result.Requested)
	assert.Contains(t, outcome.Result.Error, "implicit latest")
}

func TestRunRejectsUnknownActionsAndDocuments(t *testing.T) {
	svc := artifact.NewMockService(gomock.NewController(t))
	_, err := invoke.Run(context.Background(), svc, &invoke.Params{Action: artifact.ActionPin, Repository: "dir", Ref: "a:1"})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)

	_, err = invoke.Run(context.Background(), svc, &invoke.Params{Action: artifact.ActionShow, Repository: "dir", Ref: "a:1", Document: "license"})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

func TestRunCollectionEnvelopeSurvivesFailure(t *testing.T) {
	svc := artifact.NewMockService(gomock.NewController(t))
	boom := errors.New("catalog unavailable")
	svc.EXPECT().Search(gomock.Any(), gomock.Any()).Return(nil, boom)
	outcome, err := invoke.Run(context.Background(), svc, &invoke.Params{Action: artifact.ActionSearch, Query: "x", Labels: map[string]string{"b": "2", "a": "1"}})
	require.ErrorIs(t, err, boom)
	require.NotNil(t, outcome.Collection)
	assert.NotNil(t, outcome.Collection.Artifacts)
	assert.Empty(t, outcome.Collection.Artifacts)
	assert.Equal(t, "catalog unavailable", outcome.Collection.Error)
	assert.Equal(t, outcome.Collection, outcome.Payload())
}

func TestSelectorLabelsAreSorted(t *testing.T) {
	assert.Nil(t, invoke.SelectorLabels(nil))
	assert.Equal(t, []artifact.Label{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}}, invoke.SelectorLabels(map[string]string{"b": "2", "a": "1"}))
}

func TestResultMarkFailedKeepsWhatTheServiceReported(t *testing.T) {
	result := &invoke.Result{OperationResult: artifact.OperationResult{Action: artifact.ActionPull, Repository: "kept", Requested: "kept:1"}}
	result.MarkFailed(nil, artifact.ActionPublish, "x", "y")
	assert.Empty(t, result.Error, "no error, no failure markers")

	result.MarkFailed(errors.New("nope"), artifact.ActionPublish, "other", "other:2")
	assert.Equal(t, artifact.OutcomeFailed, result.Outcome)
	assert.Equal(t, artifact.ActionPull, result.Action)
	assert.Equal(t, "kept", result.Repository)
	assert.Equal(t, "kept:1", result.Requested)
	assert.Equal(t, "nope", result.Error)
}
