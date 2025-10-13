package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetEnvironmentVariable_NilSafe(t *testing.T) {
	// Should not panic when called with nil stack.
	// Use defer/recover to verify no panic occurs.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetEnvironmentVariable panicked with nil stack: %v", r)
		}
	}()

	SetEnvironmentVariable(nil, "KEY", "VAL")

	// Test passes if no panic occurs.
	assert.True(t, true, "Function executed without panic on nil stack")
}

func TestSetEnvironmentVariable_SetsValues(t *testing.T) {
	stack := &schema.ConfigAndStacksInfo{}

	SetEnvironmentVariable(stack, "AWS_PROFILE", "dev")
	assert.NotNil(t, stack.ComponentEnvSection)
	assert.Equal(t, "dev", stack.ComponentEnvSection["AWS_PROFILE"])
	assert.Contains(t, stack.ComponentEnvList, "AWS_PROFILE=dev")

	// Add another value to ensure accumulation.
	SetEnvironmentVariable(stack, "AWS_REGION", "us-east-2")
	assert.Equal(t, "us-east-2", stack.ComponentEnvSection["AWS_REGION"])
	assert.Contains(t, stack.ComponentEnvList, "AWS_REGION=us-east-2")
}
