// Package service manages the per-user background service that runs the SSH agent daemon: a
// LaunchAgent on macOS and a systemd user unit on Linux.
//
// Atmos kills the plugin's whole process group when an invocation ends, so the agent can never be
// a child of a plugin call. launchd or systemd starts it instead, running a stable copy of the
// plugin executable in daemon mode. Every external command goes through a Runner, so tests never
// run launchctl or systemctl.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/cloudposse/atmos/plugins/ssh/paths"
)

// Sentinel errors. Wrap them with fmt.Errorf("%w ...") and test with errors.Is.
var (
	// ErrUnsupportedPlatform means there is no service manager for this operating system.
	ErrUnsupportedPlatform = errors.New("managing the agent service is not supported on this platform")
	// ErrNotInstalled means the service definition does not exist.
	ErrNotInstalled = errors.New("agent service is not installed")
	// ErrInvalidSpec means an install request is incomplete or unsafe.
	ErrInvalidSpec = errors.New("invalid service specification")
	// ErrCommandFailed means a service manager command exited with a failure.
	ErrCommandFailed = errors.New("service manager command failed")
	// ErrNoSystemctl means systemctl was not found in any known location.
	ErrNoSystemctl = errors.New("systemctl not found")
)

// Spec says what the service runs.
type Spec struct {
	// Executable is the absolute path of the stable copy of the plugin executable (see
	// InstallExecutable). It must not be a path that changes on update.
	Executable string
	// Args are the arguments that put the executable into daemon mode.
	Args []string
}

// validate checks that the spec can be written into a service definition.
func (s Spec) validate() error {
	if s.Executable == "" || !isAbs(s.Executable) {
		return fmt.Errorf("%w: executable must be an absolute path, got %q", ErrInvalidSpec, s.Executable)
	}
	return nil
}

// Status describes the service as the service manager and the daemon's state file report it.
type Status struct {
	// Installed is true when the service definition exists.
	Installed bool
	// Loaded is true when the service manager knows the service.
	Loaded bool
	// Running is true when the daemon process is up.
	Running bool
	// PID is the daemon's process id when Running, else 0.
	PID int
	// Executable is the executable the installed definition runs.
	Executable string
	// InstalledVersion is the version of that executable, read from its stable file name.
	InstalledVersion string
	// DaemonVersion is the version the running daemon recorded in the state file. It is empty when
	// the daemon is not running or its state file does not belong to the running process.
	DaemonVersion string
	// VersionMismatch is true when the running daemon is a different version from the installed one.
	VersionMismatch bool
	// DefinitionPath is where the definition lives.
	DefinitionPath string
	// LogFile is where the daemon's output goes.
	LogFile string
}

// Manager installs and drives the per-user service.
type Manager interface {
	// Install writes the service definition and (re)starts the service with it.
	Install(ctx context.Context, spec Spec) error
	// Start starts an installed service.
	Start(ctx context.Context) error
	// Stop stops the service, and keeps it stopped until Start or the next login.
	Stop(ctx context.Context) error
	// Status reports the service's state.
	Status(ctx context.Context) (Status, error)
	// Uninstall stops the service and removes its definition.
	Uninstall(ctx context.Context) error
}

// Command is one external command to run.
type Command struct {
	// Name is the absolute path of the program.
	Name string
	Args []string
	// Env is the complete environment of the command. The plugin has none of its own.
	Env []string
}

// Output is what a command printed and how it exited.
type Output struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner runs external commands. A command that starts and exits non-zero is reported through
// Output.ExitCode, not as an error; the error is for failing to run it at all.
type Runner interface {
	Run(ctx context.Context, cmd Command) (Output, error)
}

// Options configures New.
type Options struct {
	// Paths locates the state file, the log file and the stable executables. Required.
	Paths *paths.Paths
	// Runner runs launchctl and systemctl. It defaults to ExecRunner.
	Runner Runner
	// DefinitionDir overrides where the plist or unit is written. It defaults to
	// ~/Library/LaunchAgents on macOS and ~/.config/systemd/user on Linux. Tests set it.
	DefinitionDir string
	// UID overrides the user id used for the launchd domain and the systemd runtime directory.
	UID int
	// FindExecutable finds the first existing file among candidates. It defaults to a stat check.
	FindExecutable func(candidates []string) (string, bool)
}

// New returns the Manager for the current operating system. On any other system it returns a
// Manager whose methods all fail with ErrUnsupportedPlatform.
func New(opts Options) (Manager, error) {
	return newForOS(runtime.GOOS, opts)
}

func newForOS(goos string, opts Options) (Manager, error) {
	if opts.Paths == nil {
		return nil, fmt.Errorf("%w: Options.Paths is required", ErrInvalidSpec)
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.FindExecutable == nil {
		opts.FindExecutable = findExisting
	}
	if opts.UID == 0 {
		opts.UID = os.Getuid()
	}
	switch goos {
	case "darwin":
		return newLaunchd(opts)
	case "linux":
		return newSystemd(opts)
	default:
		return unsupported{}, nil
	}
}

// findExisting returns the first candidate that is a regular, executable file.
func findExisting(candidates []string) (string, bool) {
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return c, true
		}
	}
	return "", false
}

// ExecRunner runs commands with os/exec, using exactly the environment in the Command.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, c Command) (Output, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Env = append([]string{}, c.Env...)
	var stdout, stderr limitedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	out := Output{Stdout: stdout.String(), Stderr: stderr.String()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		out.ExitCode = exitErr.ExitCode()
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("run %s: %w", c.Name, err)
	}
	return out, nil
}

// unsupported is the Manager on operating systems without a service manager implementation.
type unsupported struct{}

func (unsupported) Install(context.Context, Spec) error { return ErrUnsupportedPlatform }
func (unsupported) Start(context.Context) error         { return ErrUnsupportedPlatform }
func (unsupported) Stop(context.Context) error          { return ErrUnsupportedPlatform }
func (unsupported) Uninstall(context.Context) error     { return ErrUnsupportedPlatform }
func (unsupported) Status(context.Context) (Status, error) {
	return Status{}, ErrUnsupportedPlatform
}
