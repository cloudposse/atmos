package script

import (
	"context"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Tools owns pinned tool state for one invocation. Interpreters control when
// declarations are permitted (for example, before starting parallel tasks).
// Installation and environment snapshots are serialized to avoid partial state.
type Tools struct {
	gate     chan struct{}
	mu       sync.Mutex
	install  ToolInstaller
	versions map[string]string
	dirs     []string
}

// NewTools creates invocation-local tool state around the host installer.
func NewTools(install ToolInstaller) *Tools {
	defer perf.Track(nil, "script.NewTools")()

	return &Tools{install: install, versions: make(map[string]string), gate: make(chan struct{}, 1)}
}

// Install provisions a pin once and rejects conflicting versions. Failed or
// canceled installations do not alter the invocation's pins or PATH.
func (t *Tools) Install(ctx context.Context, name, version string) error {
	defer perf.Track(nil, "script.Tools.Install")()
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return serviceFailure(errUtils.ErrScriptInvalidArgument, nil, "tool name and version must be nonempty")
	}
	select {
	case t.gate <- struct{}{}:
		defer func() { <-t.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	t.mu.Lock()
	previous, pinned := t.versions[name]
	t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if pinned {
		if previous != version {
			return serviceFailure(errUtils.ErrScriptInvalidArgument, nil, "tool %q is already pinned to %q in this script", name, previous)
		}
		return nil
	}
	return t.provision(ctx, name, version)
}

// provision runs only while the installation gate is held.
func (t *Tools) provision(ctx context.Context, name, version string) error {
	if t.install == nil {
		return serviceFailure(errUtils.ErrScript, nil, "tool installation is unavailable in this execution context")
	}
	dirs, err := t.install(ctx, map[string]string{name: version})
	if err != nil {
		return serviceFailure(errUtils.ErrScript, err, "install dependency %s@%s: %s", name, version, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.versions[name] = version
	t.dirs = append(t.dirs, dirs...)
	return nil
}

// Environment returns an isolated environment with installed tool directories
// ahead of the caller's PATH, preserving declaration order.
func (t *Tools) Environment(base []string) []string {
	defer perf.Track(nil, "script.Tools.Environment")()
	t.mu.Lock()
	defer t.mu.Unlock()
	env := ProcessEnvironment(base, nil)
	for i := len(t.dirs) - 1; i >= 0; i-- {
		env = envpkg.UpdateEnvironmentPath(env, t.dirs[i])
	}
	return env
}
