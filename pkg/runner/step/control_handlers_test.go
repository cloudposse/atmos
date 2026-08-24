package step

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControlStepHandlersValidateAndRejectDirectExecute(t *testing.T) {
	tests := []struct {
		name       string
		stepType   string
		wantErrMsg string
	}{
		{
			name:       "parallel",
			stepType:   schema.TaskTypeParallel,
			wantErrMsg: "parallel steps require workflow executor context",
		},
		{
			name:       "matrix",
			stepType:   schema.TaskTypeMatrix,
			wantErrMsg: "matrix steps require workflow executor context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := Get(tt.stepType)
			require.True(t, ok)
			assert.Equal(t, CategoryCommand, handler.GetCategory())
			assert.False(t, handler.RequiresTTY())

			valid := &schema.WorkflowStep{
				Name: tt.name,
				Type: tt.stepType,
				Steps: []schema.WorkflowStep{{
					Name:    "child",
					Type:    schema.TaskTypeShell,
					Command: "echo ok",
				}},
			}
			if tt.stepType == schema.TaskTypeMatrix {
				valid.Matrix = map[string][]string{"stack": {"dev"}}
			}
			require.NoError(t, handler.Validate(valid))

			invalid := &schema.WorkflowStep{Name: tt.name, Type: tt.stepType}
			require.Error(t, handler.Validate(invalid))

			result, err := handler.Execute(context.Background(), valid, NewVariables())
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
			assert.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
		})
	}
}
