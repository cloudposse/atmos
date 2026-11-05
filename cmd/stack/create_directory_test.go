package stack

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDirectoryWithComponents(t *testing.T) {
	output := createProject(t)
	cmd := newStackCreateCommand()
	require.NoError(t, cmd.ParseFlags([]string{"--target", "stacks/dev", "--name", "dev", "--component", "terraform:network", "--defaults", "--dry-run"}))
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.RunE(cmd, nil))
	assert.Contains(t, output.String(), "stacks/dev/stack.yaml")
	assert.Contains(t, output.String(), "network")
	assert.NoDirExists(t, "stacks/dev")
	require.NoError(t, cmd.Flags().Set("dry-run", "false"))
	require.NoError(t, cmd.RunE(cmd, nil))
	content, err := os.ReadFile("stacks/dev/stack.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(content), "enabled: false")
	assert.Contains(t, string(content), "network")
}
