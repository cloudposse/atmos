//go:build unix

package reexec

import "syscall"

// Exec replaces the current process image with the given program, argv,
// and envv (Unix execve semantics). Declared as a variable so tests can
// override it to avoid actually replacing the test process.
var Exec ExecFunc = syscall.Exec
