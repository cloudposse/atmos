// Package daemon makes sure the SSH agent daemon runs the current plugin version.
package daemon

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/service"
)

// Arg is the first argument that puts the plugin executable into daemon mode.
const Arg = "agent-serve"

// RootFlag names the data root in daemon mode.
const RootFlag = "--root"

// Deps are the outside dependencies of Ensure.
type Deps struct {
	// Service returns the service manager for the data root.
	Service func(p *paths.Paths) (service.Manager, error)
	// InstallExecutable copies the running executable to its stable, versioned path.
	InstallExecutable func(p *paths.Paths, version string) (string, error)
	// Version is the plugin's version.
	Version string
}

// Args returns the arguments the service passes to the executable. The root is always explicit,
// so the daemon uses the same data directory as the login that started it.
func Args(p *paths.Paths) []string { return []string{Arg, RootFlag, p.Root()} }

// Result says what Ensure found and did.
type Result struct {
	// Installed is true when Ensure wrote the service definition and started the service.
	Installed bool
	// Status is the service status before any change.
	Status service.Status
}

// Ensure makes sure the daemon runs the current version. A service that is installed, running,
// and not out of date is left alone. Otherwise the executable is copied to its stable path, the
// service is installed, and it is started.
func Ensure(ctx context.Context, d Deps, p *paths.Paths) (Result, error) {
	mgr, err := d.Service(p)
	if err != nil {
		return Result{}, fmt.Errorf("agent service: %w", err)
	}
	st, statusErr := mgr.Status(ctx)
	if errors.Is(statusErr, service.ErrUnsupportedPlatform) {
		return Result{}, fmt.Errorf("agent service: %w", statusErr)
	}
	if statusErr == nil && current(st, d.Version) {
		return Result{Status: st}, nil
	}
	exe, err := d.InstallExecutable(p, d.Version)
	if err != nil {
		return Result{Status: st}, fmt.Errorf("install the agent executable: %w", err)
	}
	if err := mgr.Install(ctx, service.Spec{Executable: exe, Args: Args(p)}); err != nil {
		return Result{Status: st}, fmt.Errorf("install the agent service: %w", err)
	}
	if err := mgr.Start(ctx); err != nil {
		return Result{Status: st}, fmt.Errorf("start the agent service: %w", err)
	}
	return Result{Installed: true, Status: st}, nil
}

// current reports whether the running service needs no change.
func current(st service.Status, version string) bool {
	return st.Installed && st.Running && !st.VersionMismatch && st.InstalledVersion == version
}
