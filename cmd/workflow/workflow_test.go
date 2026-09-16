package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowSelectorFlags(t *testing.T) {
	tags := workflowCmd.Flags().Lookup("tags")
	require.NotNil(t, tags)
	assert.Equal(t, "stringSlice", tags.Value.Type())

	labels := workflowCmd.Flags().Lookup("labels")
	require.NotNil(t, labels)
	assert.Equal(t, "stringSlice", labels.Value.Type())
}

// TestWorkflowLabelsFlag_BoundToEnvVar is a regression test: bindFlagToViper only
// binds environment variables listed by a flag's GetEnvVars(), so --labels must be
// registered with flags.WithEnvVars("labels", "ATMOS_WORKFLOW_LABELS") for
// ATMOS_WORKFLOW_LABELS to configure this filter at all.
func TestWorkflowLabelsFlag_BoundToEnvVar(t *testing.T) {
	labelsFlag := workflowParser.Registry().Get("labels")
	require.NotNil(t, labelsFlag)
	assert.Contains(t, labelsFlag.GetEnvVars(), "ATMOS_WORKFLOW_LABELS")
}
