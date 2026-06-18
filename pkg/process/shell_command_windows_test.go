//go:build windows

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
	require.NotNil(t, cmd.SysProcAttr)
	assert.Contains(t, cmd.SysProcAttr.CmdLine, `/S /C "echo hello"`)
}
