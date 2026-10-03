//go:build windows

package aws

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertDefaultBuilderShellInvocation verifies the command runs through cmd.exe with a verbatim
// command line. pkg/process builds Args as just the shell and carries the real command line in
// SysProcAttr.CmdLine (`"<shell>" /S /C "<command>"`) so quoted paths are not re-escaped by Go.
func assertDefaultBuilderShellInvocation(t *testing.T, cmd *exec.Cmd, command string) {
	t.Helper()

	require.NotNil(t, cmd.SysProcAttr, "SysProcAttr.CmdLine must carry the verbatim command line")
	require.Len(t, cmd.Args, 1, "Args holds only the shell; the arguments live in CmdLine")
	assert.Equal(t, `"`+cmd.Args[0]+`" /S /C "`+command+`"`, cmd.SysProcAttr.CmdLine)
}

// TestDefaultCredentialProcessCommandBuilder_UsesComspec pins the shell the command runs under.
func TestDefaultCredentialProcessCommandBuilder_UsesComspec(t *testing.T) {
	const comspec = `C:\Windows\System32\cmd.exe`
	t.Setenv("COMSPEC", comspec)

	cmd, err := DefaultCredentialProcessCommandBuilder(context.Background(), `"C:\Program Files\helper.exe" --flag`, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{comspec}, cmd.Args)
	assert.Equal(t, `"`+comspec+`" /S /C ""C:\Program Files\helper.exe" --flag"`, cmd.SysProcAttr.CmdLine)
}
