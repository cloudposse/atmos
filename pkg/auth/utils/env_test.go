package utils

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestSetEnvironmentVariable_NilSafe(t *testing.T) {
    // Should not panic.
	SetEnvironmentVariable(nil, "KEY", "VAL")
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
