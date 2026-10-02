//go:build !windows

package aws

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertDefaultBuilderShellInvocation verifies the command runs through `sh -c <command>` like
// the AWS SDKs do.
func assertDefaultBuilderShellInvocation(t *testing.T, cmd *exec.Cmd, command string) {
	t.Helper()
	assert.Equal(t, []string{"sh", "-c", command}, cmd.Args)
}
