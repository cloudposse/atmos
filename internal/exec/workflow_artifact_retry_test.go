package exec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteWorkflowRetriesArtifactStep(t *testing.T) {
	ctrl := gomock.NewController(t)
	svc := artifact.NewMockService(ctrl)
	var executions []artifact.Execution
	svc.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, req artifact.PublishRequest) (*artifact.OperationResult, error) {
		executions = append(executions, req.Execution)
		if len(executions) == 1 {
			return nil, errors.New("transient")
		}
		return &artifact.OperationResult{Outcome: artifact.OutcomeCreated}, nil
	}).Times(2)
	restore := stepPkg.UseArtifactServiceFactory(func(*schema.AtmosConfiguration, invoke.ServiceOptions) (artifact.Service, error) { return svc, nil })
	t.Cleanup(restore)
	attempts := 2
	delay := time.Millisecond
	workflow := &schema.WorkflowDefinition{Steps: []schema.WorkflowStep{{Name: "publish", Type: "artifact", Action: "publish", Source: t.TempDir(), Repository: "test", ArtifactName: "app", Version: "1", Retry: &schema.RetryConfig{MaxAttempts: &attempts, InitialDelay: &delay, MaxDelay: &delay}}}}
	err := ExecuteWorkflow(schema.AtmosConfiguration{}, "test", "test.yaml", workflow, false, "", "", "")
	require.NoError(t, err)
	require.Len(t, executions, 2)
	assert.Equal(t, executions[0].OperationID, executions[1].OperationID)
	assert.NotEmpty(t, executions[0].OperationID)
}
