package step

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPublishEnvironmentIsolation(t *testing.T) {
	t.Parallel()
	vars := NewVariables()
	vars.Env = map[string]string{"AWS_ACCESS_KEY_ID": "component-key", "KEEP": "value"}
	vars.Set("credentials", NewStepResult("step-key"))
	step := &schema.WorkflowStep{Env: map[string]string{"AWS_ACCESS_KEY_ID": "{{ .steps.credentials.value }}", "NEW": "step-value"}}
	env, err := publishEnvironment(step, vars)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"AWS_ACCESS_KEY_ID": "step-key", "KEEP": "value", "NEW": "step-value"}, env)
	assert.Equal(t, "component-key", vars.Env["AWS_ACCESS_KEY_ID"])
	assert.NotContains(t, vars.Env, "NEW")
	assert.Equal(t, "{{ .steps.credentials.value }}", step.Env["AWS_ACCESS_KEY_ID"])
}

func TestPublishEnvironmentRejectsTemplateErrors(t *testing.T) {
	t.Parallel()
	vars := NewVariables()
	vars.Env = nil
	_, err := publishEnvironment(&schema.WorkflowStep{Env: map[string]string{"BAD": "{{"}}, vars)
	require.Error(t, err)
	env, err := publishEnvironment(&schema.WorkflowStep{Env: map[string]string{"KEY": "value"}}, vars)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"KEY": "value"}, env)
	assert.Nil(t, vars.Env)
}
