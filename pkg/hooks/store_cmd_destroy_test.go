package hooks

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// Compile-time guards for the events this behavior depends on.
var (
	_ = BeforeTerraformDestroy
	_ = AfterTerraformDestroy
)

func newDestroyTestCommand(getter TerraformOutputGetter, mock *MockStore) *StoreCommand {
	return &StoreCommand{
		atmosConfig: &schema.AtmosConfiguration{Stores: map[string]store.Store{"test-store": mock}},
		info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "test-component",
			Stack:            "test-stack",
		},
		outputGetter: getter,
	}
}

func TestStoreCommand_ProcessStoreCommand_DestroyMissingOutputs(t *testing.T) {
	getter := func(_ *schema.AtmosConfiguration, _, _, output string, _ bool, _ *schema.AuthContext, _ any) (any, bool, error) {
		if output == "present" {
			return "present-value", true, nil
		}
		return nil, false, nil
	}
	hook := &Hook{
		Name: "test-store",
		Outputs: map[string]string{
			"missing_key": ".missing",
			"present_key": ".present",
			"literal_key": "literal-value",
		},
	}

	tests := []struct {
		name  string
		event HookEvent
	}{
		{name: "after destroy, outputs are gone because the destroy succeeded", event: AfterTerraformDestroy},
		{name: "before destroy, outputs are gone because an earlier destroy already ran", event: BeforeTerraformDestroy},
		{name: "aggregate destroy event", event: AfterTerraformDestroyAggregate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockStore()

			err := newDestroyTestCommand(getter, mock).processStoreCommand(hook, tt.event)
			require.NoError(t, err)

			// The missing output is skipped; everything that can be resolved is still written.
			assert.NotContains(t, mock.data, "test-stack/test-component/missing_key")
			assert.Equal(t, "present-value", mock.data["test-stack/test-component/present_key"])
			assert.Equal(t, "literal-value", mock.data["test-stack/test-component/literal_key"])
		})
	}
}

func TestStoreCommand_ProcessStoreCommand_MissingOutputsFailOutsideDestroy(t *testing.T) {
	getter := func(_ *schema.AtmosConfiguration, _, _, _ string, _ bool, _ *schema.AuthContext, _ any) (any, bool, error) {
		return nil, false, nil
	}
	hook := &Hook{Name: "test-store", Outputs: map[string]string{"missing_key": ".missing"}}

	for _, event := range []HookEvent{AfterTerraformApply, BeforeTerraformApply, AfterTerraformPlan, AfterTerraformOutput} {
		t.Run(string(event), func(t *testing.T) {
			err := newDestroyTestCommand(getter, NewMockStore()).processStoreCommand(hook, event)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrTerraformOutputNotFound)
		})
	}
}

func TestStoreCommand_ProcessStoreCommand_DestroyKeepsOutputReadErrors(t *testing.T) {
	// A failed read says nothing about whether the output exists, so destroy still reports it.
	readErr := errors.New("backend unavailable")
	getter := func(_ *schema.AtmosConfiguration, _, _, _ string, _ bool, _ *schema.AuthContext, _ any) (any, bool, error) {
		return "ignored", false, readErr
	}
	hook := &Hook{Name: "test-store", Outputs: map[string]string{"key": ".output"}}

	for _, event := range []HookEvent{BeforeTerraformDestroy, AfterTerraformDestroy} {
		t.Run(string(event), func(t *testing.T) {
			err := newDestroyTestCommand(getter, NewMockStore()).processStoreCommand(hook, event)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrTerraformOutputFailed)
			assert.ErrorIs(t, err, readErr)
		})
	}
}
