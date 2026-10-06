package starlark

//go:generate go run go.uber.org/mock/mockgen -destination mock_step_library_test.go -package starlark github.com/cloudposse/atmos/pkg/automation StepLibrary

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestStepAdapterUsesGoAPI(t *testing.T) {
	library := NewMockStepLibrary(gomock.NewController(t))
	library.EXPECT().Names().Return([]string{"example", "wait-all"})
	library.EXPECT().Fork().Return(library)
	library.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, call *automation.StepCall) (*automation.StepResult, error) {
		assert.Equal(t, "example", call.Type)
		assert.Equal(t, map[string]any{"with": map[string]any{"retries": 3}, "enabled": true, "options": []any{"a", "b"}}, call.Configuration)
		assert.False(t, call.Parallel)
		return &automation.StepResult{Value: "done", Metadata: map[string]any{"code": 201}, Outputs: map[string]string{"image": "demo:dev"}}, nil
	})
	result, err := New().Execute(t.Context(), script.Spec{Steps: library, Source: `r = steps.example(with_={"retries":3},enabled=True,options=["a","b"])
output = [r.value, r.metadata["code"], r.outputs["image"], "wait_all" in dir(steps)]`})
	require.NoError(t, err)
	assert.JSONEq(t, `["done",201,"demo:dev",true]`, result.Value)
}

func TestStepAdapterRequiresHost(t *testing.T) {
	_, err := New().Execute(t.Context(), script.Spec{Source: `steps.run("input",prompt="Name?")`})
	require.ErrorContains(t, err, "host has not provided a step library")
}

func TestStepAdapterRejectsInvalidCallsBeforeDispatch(t *testing.T) {
	for _, tc := range []struct{ source, message string }{
		{`steps.example("positional")`, "keyword arguments"},
		{`steps.run()`, "one positional step type"},
		{`steps.run(1)`, "must be a string"},
		{`steps.example(type="other")`, "type is selected"},
		{`steps.example(with_={}, **{"with":{}})`, "duplicate step field"},
		{`steps.example(content=lambda: None)`, "cannot encode"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			library := NewMockStepLibrary(gomock.NewController(t))
			library.EXPECT().Names().Return([]string{"example", "parallel"})
			library.EXPECT().Fork().Return(library)
			_, err := New().Execute(t.Context(), script.Spec{Steps: library, Source: tc.source})
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestStepAdapterNormalizesEmptyResults(t *testing.T) {
	library := NewMockStepLibrary(gomock.NewController(t))
	library.EXPECT().Names().Return([]string{"example"})
	library.EXPECT().Fork().Return(library)
	library.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, call *automation.StepCall) (*automation.StepResult, error) {
		assert.Equal(t, map[string]any{"for": []any{"job"}, "continue": "never"}, call.Configuration)
		return nil, nil
	})
	result, err := New().Execute(t.Context(), script.Spec{Steps: library, Source: `r = steps.run("example",for_=["job"],continue_="never")
output=[r.value,r.values,r.metadata,r.outputs,r.error,r.skipped]`})
	require.NoError(t, err)
	assert.JSONEq(t, `["",[],{},{},"",false]`, result.Value)
}

func TestStepAdapterPreservesHostErrors(t *testing.T) {
	library := NewMockStepLibrary(gomock.NewController(t))
	library.EXPECT().Names().Return([]string{"example"})
	library.EXPECT().Fork().Return(library)
	library.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil, context.Canceled)
	_, err := New().Execute(t.Context(), script.Spec{Steps: library, Source: `steps.example()`})
	require.ErrorIs(t, err, context.Canceled)
}

func TestStepAdapterRejectsNonDataResults(t *testing.T) {
	library := NewMockStepLibrary(gomock.NewController(t))
	library.EXPECT().Names().Return([]string{"example"})
	library.EXPECT().Fork().Return(library)
	library.EXPECT().Run(gomock.Any(), gomock.Any()).Return(&automation.StepResult{Metadata: map[string]any{"unsupported": make(chan int)}}, nil)
	_, err := New().Execute(t.Context(), script.Spec{Steps: library, Source: `steps.example()`})
	require.ErrorContains(t, err, "cannot convert step result")
}
