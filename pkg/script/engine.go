// Package script defines the registry for embedded script interpreters.
package script

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Spec describes one invocation. Env is explicit script input; ProcessEnv is
// the effective environment for child processes and is not exposed as globals.
type Spec struct {
	Name, Source, WorkingDirectory string
	// SourcePath is the file Source was read from, when it came from one. It names the program
	// in tracebacks and anchors the script's load() calls to that file's directory, instead of
	// WorkingDirectory. Name stays the step name used in hints.
	SourcePath string
	// ProjectRoot is the Atmos project base path. When set, paths under it are shown relative to
	// it (for example `scripts/boom.star:7:8`) in error messages and tracebacks. It only affects
	// display: load() resolution and ctx.script paths keep using absolute paths.
	ProjectRoot           string
	Env                   map[string]string
	Flags, Arguments      map[string]any
	ProcessEnv            []string
	Stdout, Stderr        io.Writer
	DryRun                bool
	Component             *ComponentRef
	ResolveComponent      ComponentResolver
	ProcessOverrides      map[string]string
	Hook                  *HookContext
	AtmosWorkingDirectory string
	File                  *File
	InstallTools          ToolInstaller
	ParseCommand          CommandParser
}

// ToolInstaller provisions pinned tools and returns their executable directories.
type ToolInstaller func(context.Context, map[string]string) ([]string, error)

// HookContext carries the component snapshot and lifecycle facts for one hook.
// It is host-owned and must not be mutated while an invocation is running.
type HookContext struct {
	Name, Event string
	Component   *Component
	Operation   Operation
	// ProcessOverrides preserves hook-level env precedence over component env.
	ProcessOverrides map[string]string
}

// Operation describes the parent lifecycle operation, not a hook subprocess.
// Nil result fields mean unavailable (including before-execution hooks).
type Operation struct {
	Command        string
	Status, Error  *string
	ExitCode       *int
	Stdout, Stderr *string
}

// ComponentRef identifies a logical component instance in a stack.
type ComponentRef struct{ Name, Stack, Type string }

// Component is resolved execution data, separate from the provider's command API.
type Component struct {
	ComponentRef
	Implementation, Path string
	Config               map[string]any
	Env                  map[string]string
}

// ComponentResolver resolves execution-time values, including YAML functions.
type ComponentResolver func(context.Context, ComponentRef) (*Component, error)

// Result holds the JSON-encoded top-level output, when one was assigned.
type Result struct {
	Value     string
	HasOutput bool
}

// Engine executes an embedded language without starting an interpreter process.
type Engine interface {
	Execute(context.Context, Spec) (Result, error)
}

var registry = struct {
	sync.RWMutex
	engines map[string]Engine
}{engines: make(map[string]Engine)}

// Register installs an interpreter during initialization.
func Register(name string, engine Engine) {
	defer perf.Track(nil, "script.Register")()

	registry.Lock()
	defer registry.Unlock()
	registry.engines[strings.TrimSpace(name)] = engine
}

// Get looks up an embedded interpreter; executable paths remain external.
func Get(name string) (Engine, bool) {
	defer perf.Track(nil, "script.Get")()

	registry.RLock()
	defer registry.RUnlock()
	engine, ok := registry.engines[strings.TrimSpace(name)]
	return engine, ok
}
