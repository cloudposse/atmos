//go:build windows

package process

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyWindowsCmdExeQuoting_CmdExeSlashC verifies that a `cmd.exe /C
// <command>` invocation is rewritten to a verbatim SysProcAttr.CmdLine (the
// same `/S /C "<command>"` shape NewShellCommand already uses for
// session-attached commands), instead of relying on os/exec's default
// per-argument Windows escaping -- which re-quotes and backslash-escapes a
// command string containing spaces, corrupting any quote characters already
// inside it (e.g. a quoted executable path) before cmd.exe ever sees it.
func TestApplyWindowsCmdExeQuoting_CmdExeSlashC(t *testing.T) {
	command := `"C:\Users\RUNNER~1\AppData\Local\Temp\go-build\cmd.test.exe" -test.run=TestFoo -- abc`
	cmd := exec.Command("cmd.exe", "/C", command)
	require.Nil(t, cmd.SysProcAttr, "precondition: exec.Command must not set SysProcAttr on its own")

	applyWindowsCmdExeQuoting(cmd, "cmd.exe", []string{"/C", command})

	require.NotNil(t, cmd.SysProcAttr, "SysProcAttr must be set to bypass Go's default arg escaping")
	assert.Equal(t, `"cmd.exe" /S /C "`+command+`"`, cmd.SysProcAttr.CmdLine)
}

// TestApplyWindowsCmdExeQuoting_CaseInsensitiveAndComspecPath verifies the
// cmd.exe basename match is case-insensitive and tolerates a full COMSPEC
// path (e.g. `C:\Windows\System32\cmd.exe`), matching
// controlShellInvocationForOS's os.Getenv("COMSPEC") fallback.
func TestApplyWindowsCmdExeQuoting_CaseInsensitiveAndComspecPath(t *testing.T) {
	command := "echo hi"
	program := `C:\Windows\System32\CMD.EXE`
	cmd := exec.Command(program, "/C", command)

	applyWindowsCmdExeQuoting(cmd, program, []string{"/C", command})

	require.NotNil(t, cmd.SysProcAttr)
	assert.Equal(t, `"`+program+`" /S /C "`+command+`"`, cmd.SysProcAttr.CmdLine)
}

// TestApplyWindowsCmdExeQuoting_NoOpForOtherShapes verifies the rewrite only
// triggers for the exact `cmd.exe /C <command>` shape
// controlShellInvocationForOS produces, leaving any other program/args
// combination (e.g. a direct argv-style invocation with no shell involved)
// untouched so it keeps Go's normal, correct per-argument escaping.
func TestApplyWindowsCmdExeQuoting_NoOpForOtherShapes(t *testing.T) {
	cases := []struct {
		name    string
		program string
		args    []string
	}{
		{name: "not cmd.exe", program: "sh", args: []string{"/C", "echo hi"}},
		{name: "wrong flag", program: "cmd.exe", args: []string{"/K", "echo hi"}},
		{name: "extra arg", program: "cmd.exe", args: []string{"/C", "echo", "hi"}},
		{name: "no args", program: "cmd.exe", args: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(tc.program, tc.args...)
			applyWindowsCmdExeQuoting(cmd, tc.program, tc.args)
			assert.Nil(t, cmd.SysProcAttr)
		})
	}
}
