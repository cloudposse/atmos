//go:build windows

package process

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestInterruptTerminalChildIsUnsupportedOnWindows(t *testing.T) {
	// The terminal UI then terminates the child, as it did before graceful stops existed.
	assert.ErrorIs(t, interruptTerminalChild(&os.Process{Pid: 42}, false), errUtils.ErrInterruptUnsupported)
}
