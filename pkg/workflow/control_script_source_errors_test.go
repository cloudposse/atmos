package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveControlStepScriptSourceTemplateError(t *testing.T) {
	step := &schema.WorkflowStep{Name: "child", Type: schema.TaskTypeScript, Source: "scripts/{{ .broken"}

	_, err := resolveControlStep(step, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "child", "the error names the step")
	assert.Contains(t, err.Error(), "source", "the error names the failing field")
}

func TestResolveControlStepLeavesNonScriptSourceUnrendered(t *testing.T) {
	step := &schema.WorkflowStep{Name: "child", Type: schema.TaskTypeShell, Source: "scripts/{{ .broken", Command: "x"}

	resolved, err := resolveControlStep(step, nil, nil)

	require.NoError(t, err, "only script steps render their source path")
	assert.Equal(t, "scripts/{{ .broken", resolved.Source)
}
