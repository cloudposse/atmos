package about

import (
	"testing"

	"github.com/stretchr/testify/assert"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestAboutCmd(t *testing.T) {
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	assert.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)

	// Execute the command - output goes to global I/O context.
	err = aboutCmd.RunE(aboutCmd, []string{})
	assert.NoError(t, err, "'atmos about' command should execute without error")
}

func TestAboutCommandProvider(t *testing.T) {
	provider := &AboutCommandProvider{}

	// Test GetCommand.
	cmd := provider.GetCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "about", cmd.Use)

	// Test GetName.
	assert.Equal(t, "about", provider.GetName())

	// Test GetGroup.
	assert.Equal(t, "Other Commands", provider.GetGroup())
}
