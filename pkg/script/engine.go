// Package script defines embedded interpreter registration and shared host services.
package script

import (
	"context"
	"io"

	"github.com/cloudposse/atmos/pkg/automation"
)

// Spec describes one invocation. Env is explicit script input; ProcessEnv is
// the effective environment for child processes and is not exposed as globals.
type Spec struct {
	Name, Source, WorkingDirectory string
	// SourcePath is the file Source was read from, when it came from one. It names the program
	// in diagnostics and anchors relative imports to that file's directory, instead of
	// WorkingDirectory. Name stays the step name used in hints.
	SourcePath string
	// ProjectRoot is the Atmos project base path. When set, paths under it are shown relative to
	// it (for example `scripts/boom.star:7:8`) in error messages and tracebacks. It only affects
	// display: import resolution and script context paths keep using absolute paths.
	ProjectRoot      string
	Env              map[string]string
	Flags, Arguments map[string]any
	// Args holds positional host arguments for embedded invocations.
	Args           []string
	ProcessEnv     []string
	Stdout, Stderr io.Writer
	DryRun         bool
	// Parallel marks execution inside a control runner that cannot grant exclusive terminal access.
	Parallel              bool
	Component             *ComponentRef
	ResolveComponent      ComponentResolver
	ProcessOverrides      map[string]string
	Hook                  *HookContext
	AtmosWorkingDirectory string
	File                  *File
	InstallTools          ToolInstaller
	ParseCommand          CommandParser
	Steps                 automation.StepLibrary
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

// Result holds an explicitly produced output. Strings pass through unchanged; other
// values are JSON-encoded by the interpreter. HasOutput distinguishes no output
// from an empty string or JSON null.
type Result struct {
	Value     string
	HasOutput bool
}

// Engine executes an embedded language without starting an interpreter process.
// Implementations isolate invocation state and honor context cancellation, including
// host operations. DryRun validates source without evaluating it or invoking host
// services; it does not promise recursive module validation. Streams carry process
// output separately from Result. Language values must not escape this boundary.
type Engine interface {
	Execute(context.Context, Spec) (Result, error)
}
