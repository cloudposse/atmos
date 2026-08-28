package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestSupportCommand_Structure verifies the support command's static
// properties (mirrors the style of other small top-level command tests in
// this package, e.g. describe_config_test.go).
func TestSupportCommand_Structure(t *testing.T) {
	_ = NewTestKit(t)

	assert.Equal(t, "support", supportCmd.Use)
	assert.NotEmpty(t, supportCmd.Short)
	assert.NotEmpty(t, supportCmd.Long)
	assert.NotNil(t, supportCmd.RunE)
	assert.True(t, supportCmd.DisableSuggestions)
	assert.True(t, supportCmd.SilenceUsage)
	assert.True(t, supportCmd.SilenceErrors)
}

// TestSupportCommand_RunE exercises supportCmd.RunE, covering the
// data.MarkdownNoWrapf render of the embedded support.md content. TestMain
// only wires up ui.InitFormatter/data.InitWriter, not
// data.SetMarkdownRenderer (root.go's PersistentPreRun normally does that),
// so it's set explicitly here to avoid ErrUIFormatterNotInitialized.
func TestSupportCommand_RunE(t *testing.T) {
	_ = NewTestKit(t)
	data.SetMarkdownRenderer(ui.Format)

	stdout, _ := captureStdoutStderr(t, func() {
		err := supportCmd.RunE(supportCmd, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, stdout, "Atmos")
}
