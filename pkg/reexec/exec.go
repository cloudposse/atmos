package reexec

// ExecFunc matches the signature of syscall.Exec and is used to replace
// the current process image on Unix or spawn-and-exit on Windows. It is
// declared as a type so callers can accept a function value for dependency
// injection in tests.
type ExecFunc func(argv0 string, argv []string, envv []string) error
