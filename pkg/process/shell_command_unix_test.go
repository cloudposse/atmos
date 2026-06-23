//go:build !windows

package process

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewShellCommand(t *testing.T) {
	cmd := NewShellCommand(context.Background(), "echo hello")
	require.NotNil(t, cmd)
	assert.Equal(t, []string{"sh", "-c", "echo hello"}, cmd.Args)
}
