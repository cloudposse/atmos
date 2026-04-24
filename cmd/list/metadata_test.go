package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMetadataCommand verifies the metadata command is wired up.
func TestMetadataCommand(t *testing.T) {
	assert.Equal(t, "metadata", metadataCmd.Use)
	assert.Contains(t, metadataCmd.Short, "metadata")
	assert.NotNil(t, metadataCmd.RunE)
}

// TestMetadataProcessTemplatesAndFunctionsFlags verifies that --process-templates
// and --process-functions are registered on the real `metadata` cobra command
// with the documented defaults (both true).
func TestMetadataProcessTemplatesAndFunctionsFlags(t *testing.T) {
	processTemplatesFlag := metadataCmd.Flags().Lookup("process-templates")
	if processTemplatesFlag == nil {
		processTemplatesFlag = metadataCmd.PersistentFlags().Lookup("process-templates")
	}
	assert.NotNil(t, processTemplatesFlag, "process-templates flag should be registered on metadata command")
	if processTemplatesFlag != nil {
		assert.Equal(t, "true", processTemplatesFlag.DefValue)
	}

	processFunctionsFlag := metadataCmd.Flags().Lookup("process-functions")
	if processFunctionsFlag == nil {
		processFunctionsFlag = metadataCmd.PersistentFlags().Lookup("process-functions")
	}
	assert.NotNil(t, processFunctionsFlag, "process-functions flag should be registered on metadata command")
	if processFunctionsFlag != nil {
		assert.Equal(t, "true", processFunctionsFlag.DefValue)
	}
}
