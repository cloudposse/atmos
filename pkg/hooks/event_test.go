package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHookEvent_IsPostExecution(t *testing.T) {
	tests := []struct {
		name  string
		event HookEvent
		want  bool
	}{
		{name: "before-init is pre-execution", event: BeforeTerraformInit, want: false},
		{name: "after-init is post-execution", event: AfterTerraformInit, want: true},
		{name: "before-plan is pre-execution", event: BeforeTerraformPlan, want: false},
		{name: "after-plan is post-execution", event: AfterTerraformPlan, want: true},
		{name: "after-apply is post-execution", event: AfterTerraformApply, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.event.IsPostExecution())
		})
	}
}

func TestHookEvent_Normalize_InitNotAliased(t *testing.T) {
	// init events have no deploy/apply alias — they normalize to themselves.
	assert.Equal(t, BeforeTerraformInit, BeforeTerraformInit.Normalize())
	assert.Equal(t, AfterTerraformInit, AfterTerraformInit.Normalize())
}
