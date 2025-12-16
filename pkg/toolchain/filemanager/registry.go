package filemanager

import (
	"context"
	"errors"
	"fmt"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// errFmtNamedError formats an error with a name prefix.
	errFmtNamedError = "%s: %w"
	// errFmtWrappedError formats an error wrapping another error.
	errFmtWrappedError = "%w: %w"
)

// Registry coordinates updates across multiple file managers.
type Registry struct {
	managers []FileManager
}

// NewRegistry creates a new file manager registry.
func NewRegistry(managers ...FileManager) *Registry {
	defer perf.Track(nil, "filemanager.NewRegistry")()

	return &Registry{
		managers: managers,
	}
}

// AddTool adds a tool to all enabled managers.
func (r *Registry) AddTool(ctx context.Context, tool, version string, opts ...AddOption) error {
	defer perf.Track(nil, "filemanager.Registry.AddTool")()

	var errs []error

	for _, mgr := range r.managers {
		if !mgr.Enabled() {
			continue
		}

		if err := mgr.AddTool(ctx, tool, version, opts...); err != nil {
			errs = append(errs, fmt.Errorf(errFmtNamedError, mgr.Name(), err))
		} else {
			log.Debug("Updated file", "manager", mgr.Name(), "tool", tool, "version", version)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(errFmtWrappedError, ErrUpdateFailed, errors.Join(errs...))
	}

	return nil
}

// RemoveTool removes a tool from all enabled managers.
func (r *Registry) RemoveTool(ctx context.Context, tool, version string) error {
	defer perf.Track(nil, "filemanager.Registry.RemoveTool")()

	var errs []error

	for _, mgr := range r.managers {
		if !mgr.Enabled() {
			continue
		}

		if err := mgr.RemoveTool(ctx, tool, version); err != nil {
			errs = append(errs, fmt.Errorf(errFmtNamedError, mgr.Name(), err))
		} else {
			log.Debug("Removed from file", "manager", mgr.Name(), "tool", tool, "version", version)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(errFmtWrappedError, ErrUpdateFailed, errors.Join(errs...))
	}

	return nil
}

// SetDefault sets a tool version as default in all enabled managers.
func (r *Registry) SetDefault(ctx context.Context, tool, version string) error {
	defer perf.Track(nil, "filemanager.Registry.SetDefault")()

	var errs []error

	for _, mgr := range r.managers {
		if !mgr.Enabled() {
			continue
		}

		if err := mgr.SetDefault(ctx, tool, version); err != nil {
			errs = append(errs, fmt.Errorf(errFmtNamedError, mgr.Name(), err))
		} else {
			log.Debug("Set default in file", "manager", mgr.Name(), "tool", tool, "version", version)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(errFmtWrappedError, ErrUpdateFailed, errors.Join(errs...))
	}

	return nil
}

// VerifyAll verifies all enabled managers.
func (r *Registry) VerifyAll(ctx context.Context) error {
	defer perf.Track(nil, "filemanager.Registry.VerifyAll")()

	var errs []error

	for _, mgr := range r.managers {
		if !mgr.Enabled() {
			continue
		}

		if err := mgr.Verify(ctx); err != nil {
			errs = append(errs, fmt.Errorf(errFmtNamedError, mgr.Name(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(errFmtWrappedError, ErrVerificationFailed, errors.Join(errs...))
	}

	return nil
}
