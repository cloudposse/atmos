package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTestHandlerRejectsUncapturableChildren(t *testing.T) {
	h := &TestHandler{}
	for _, tc := range []struct {
		name, kind, message string
		grouped             bool
	}{
		{"nested test", "test", "unsupported test child type", false},
		{"unknown", "missing-test-handler", "unknown test step type", false},
		{"interactive", "input", "interactive step", false},
		{"grouped interactive", "input", "interactive step", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := schema.WorkflowStep{Name: "unsafe", Type: tc.kind}
			if tc.grouped {
				child = schema.WorkflowStep{Name: "group", Type: "parallel", Steps: []schema.WorkflowStep{child}}
			}
			parent := &schema.WorkflowStep{Type: "test", Steps: []schema.WorkflowStep{child}}
			_, err := h.Execute(context.Background(), parent, NewVariables())
			require.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
			assert.ErrorContains(t, err, tc.message)
		})
	}
}

func TestTestHandlerRequiresRegisteredRunner(t *testing.T) {
	original := testRunner
	t.Cleanup(func() { RegisterTestRunner(original) })
	RegisterTestRunner(nil)
	h := &TestHandler{}
	parent := &schema.WorkflowStep{Type: "test", Steps: []schema.WorkflowStep{{Command: "unused"}}}
	require.NoError(t, h.Validate(parent), "an untyped child defaults to shell")
	result, err := h.ExecuteWithWorkflow(context.Background(), parent, NewVariables(), nil)
	assert.Nil(t, result)
	require.ErrorIs(t, err, schema.ErrWorkflowControlStepInvalid)
	assert.ErrorContains(t, err, "test runner is not registered")
}
