package cloudformation

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestTimeoutRejectsInvalidValues(t *testing.T) {
	for _, value := range []any{"bad", "0s", "-1m", true, map[string]any{"minutes": 1}} {
		_, err := buildStackSpec(map[string]any{"stack_name": "test", "path": "template.yaml", "timeout": value})
		require.Error(t, err, "timeout %v", value)
	}
}

func TestTimeoutBoundsRemoteOperation(t *testing.T) {
	spec, err := buildStackSpec(map[string]any{"stack_name": "test", "path": "template.yaml", "timeout": "20ms"})
	require.NoError(t, err)
	original := operationHandlers[OperationOutput]
	t.Cleanup(func() { operationHandlers[OperationOutput] = original })
	operationHandlers[OperationOutput] = func(octx *opContext, _ CloudFormationClient, _ *stackSpec, summary map[string]any) (map[string]any, error) {
		deadline, ok := octx.Ctx.Deadline()
		require.True(t, ok, "the remote operation needs a deadline")
		assert.LessOrEqual(t, time.Until(deadline), 20*time.Millisecond)
		<-octx.Ctx.Done()
		return summary, octx.Ctx.Err()
	}
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = runRemoteOperation(&opContext{Ctx: parent}, OperationOutput, nil, spec, nil)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NoError(t, parent.Err(), "the component timeout must expire before the parent")
}

func TestTimeoutDurationFormats(t *testing.T) {
	for _, tt := range []struct {
		value any
		want  time.Duration
	}{
		{"30m", 30 * time.Minute}, {"1h30m", 90 * time.Minute}, {"2d", 48 * time.Hour}, {"PT2H", 2 * time.Hour}, {"hourly", time.Hour}, {90, 90 * time.Second}, {1.5, 1500 * time.Millisecond},
	} {
		spec, err := buildStackSpec(map[string]any{"stack_name": "test", "path": "template.yaml", "timeout": tt.value})
		require.NoError(t, err)
		assert.Equal(t, tt.want, spec.Timeout)
	}
	spec, err := buildStackSpec(map[string]any{"stack_name": "test", "path": "template.yaml", "timeout": "1h", "timeout_in_minutes": 10})
	require.NoError(t, err)
	assert.Equal(t, time.Hour, spec.Timeout)
	spec, err = buildStackSpec(map[string]any{"stack_name": "test", "path": "template.yaml"})
	require.NoError(t, err)
	assert.Zero(t, spec.Timeout)
}

func TestWithDefaultTimeoutHonorsConfiguredDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()
	bounded, stop := withDefaultTimeout(parent, time.Minute)
	defer stop()
	deadline, _ := parent.Deadline()
	got, ok := bounded.Deadline()
	require.True(t, ok)
	assert.Equal(t, deadline, got, "a configured long timeout must not be shortened by a stage default")
	plain, stopPlain := withDefaultTimeout(context.Background(), time.Minute)
	defer stopPlain()
	got, ok = plain.Deadline()
	require.True(t, ok)
	assert.InDelta(t, 60, time.Until(got).Seconds(), 1)
}

func TestTimeoutCancelsStalledAWSRequest(t *testing.T) {
	client := NewMockCloudFormationClient(gomock.NewController(t))
	client.EXPECT().DescribeStacks(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ *cloudformation.DescribeStacksInput, _ ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := runRemoteOperation(&opContext{Ctx: parent}, OperationOutput, client, &stackSpec{StackName: "demo", Timeout: 10 * time.Millisecond}, map[string]any{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NoError(t, parent.Err())
}
