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
	assert.Equal(t, "string", labels.Value.Type())
}
