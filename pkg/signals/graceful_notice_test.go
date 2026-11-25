package signals

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGracefulStopNotice(t *testing.T) {
	assert.Equal(t, "Interrupt received; waiting for terraform to stop gracefully. Press Ctrl-C again to force exit.",
		GracefulStopNotice(os.Interrupt, []string{"terraform"}))
	assert.Equal(t, "Termination requested; waiting for terraform, tofu to stop gracefully. "+
		"Send the signal again to force exit.",
		GracefulStopNotice(syscall.SIGTERM, []string{"tofu", "terraform", "tofu"}))
	assert.Contains(t, GracefulStopNotice(os.Interrupt, nil), "waiting for the running command to stop")
}
