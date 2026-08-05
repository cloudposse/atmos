package taskgraph

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scheduler"
)

// Runner executes one resolved dependency Ref (a single command or workflow invocation).
type Runner func(ctx context.Context, ref Ref) error

// Lookup returns the direct dependency Refs declared by a named command or workflow, so the
// graph builder can walk the transitive closure. Found=false means the name does not resolve
// to any known command/workflow at all (a hard error, not "has no dependencies" — that case
// returns an empty, non-nil-error slice with found=true).
type Lookup func(ref Ref) (refs []Ref, found bool, err error)

// Options configures Run. Constructed via the With* functions below (Options pattern per
// project convention, since Run takes more than 2-3 logical dependencies).
type Options struct {
	maxConcurrency int
	commandRunner  Runner
	workflowRunner Runner
	commandLookup  Lookup
	workflowLookup Lookup
}

// Option configures Options.
type Option func(*Options)

// WithMaxConcurrency bounds how many dependency nodes run concurrently. Unset (zero) means
// unbounded -- concurrent by default, matching go-task's `deps:` default.
func WithMaxConcurrency(n int) Option {
	defer perf.Track(nil, "taskgraph.WithMaxConcurrency")()

	return func(o *Options) { o.maxConcurrency = n }
}

// WithCommandRunner supplies the callback that actually executes a KindCommand Ref.
func WithCommandRunner(fn Runner) Option {
	defer perf.Track(nil, "taskgraph.WithCommandRunner")()

	return func(o *Options) { o.commandRunner = fn }
}

// WithWorkflowRunner supplies the callback that actually executes a KindWorkflow Ref.
func WithWorkflowRunner(fn Runner) Option {
	defer perf.Track(nil, "taskgraph.WithWorkflowRunner")()

	return func(o *Options) { o.workflowRunner = fn }
}

// WithCommandLookup supplies the callback used to resolve a KindCommand Ref's own nested
// dependencies (and to confirm the referenced command exists at all).
func WithCommandLookup(fn Lookup) Option {
	defer perf.Track(nil, "taskgraph.WithCommandLookup")()

	return func(o *Options) { o.commandLookup = fn }
}

// WithWorkflowLookup supplies the callback used to resolve a KindWorkflow Ref's own nested
// dependencies (and to confirm the referenced workflow exists at all).
func WithWorkflowLookup(fn Lookup) Option {
	defer perf.Track(nil, "taskgraph.WithWorkflowLookup")()

	return func(o *Options) { o.workflowLookup = fn }
}

// Run resolves the transitive dependency closure of direct (a command's or workflow's own
// dependencies.commands/dependencies.workflows entries) and executes it via the generic
// pkg/scheduler DAG engine -- the same engine already used for parallel/matrix `needs:`
// (pkg/workflow/control.go) and Terraform's dependencies.components
// (pkg/scheduler/adapters/terraform.go). Concurrent by default; two Refs with identical
// Kind/Name/File/Flags/Args collapse into a single executed node.
//
// The overall fail mode is derived from direct entries' Fail field: best_effort if any direct
// entry sets it, else fail_fast if any direct entry sets it, else wait_all (the default --
// every dependency runs to completion regardless of siblings' failures, and Run returns the
// combined error at the end).
func Run(ctx context.Context, direct []Ref, opts ...Option) error {
	defer perf.Track(nil, "taskgraph.Run")()

	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	if len(direct) == 0 {
		return nil
	}

	graph, err := buildGraph(direct, options)
	if err != nil {
		return err
	}

	dispatcher := newDispatcher(options)
	failMode := effectiveFailMode(direct)

	schedOpts := []scheduler.Option{}
	if options.maxConcurrency > 0 {
		schedOpts = append(schedOpts, scheduler.WithMaxConcurrency(options.maxConcurrency))
	}
	if failMode == FailFast {
		schedOpts = append(schedOpts, scheduler.WithFailFast(true))
	}

	aggregate := scheduler.New(graph, dispatcher, schedOpts...).Run(ctx)
	if failMode == FailBestEffort {
		return nil
	}
	return aggregate.Err
}

// effectiveFailMode derives one run-wide fail mode from the direct refs' individual Fail
// fields: best_effort (most permissive) wins if any direct entry sets it, else fail_fast if
// any direct entry sets it, else the default, wait_all.
func effectiveFailMode(direct []Ref) string {
	mode := FailWaitAll
	for _, ref := range direct {
		switch ref.Fail {
		case FailBestEffort:
			return FailBestEffort
		case FailFast:
			mode = FailFast
		}
	}
	return mode
}

// graphVisitor walks the transitive closure of a set of direct Refs via a visited-set
// recursion, adding one graph node per unique Ref.NodeID() (automatic dedup) and one
// dependency edge per reference. Split out of buildGraph into its own type so each concern
// (dedup, node/edge bookkeeping, lookup/runner validation, recursion) is a small, separately
// readable method instead of one large function.
type graphVisitor struct {
	builder *dependency.GraphBuilder
	visited map[string]struct{}
	options *Options
}

func (v *graphVisitor) visit(ref *Ref) error {
	id := ref.NodeID()
	if _, ok := v.visited[id]; ok {
		return nil
	}
	v.visited[id] = struct{}{}

	if err := v.builder.AddNode(&dependency.Node{ID: id, Metadata: map[string]any{"ref": *ref}}); err != nil {
		return err
	}

	lookup, runner, err := lookupAndRunnerFor(ref.Kind, v.options)
	if err != nil {
		return err
	}
	if err := validateLookupAndRunner(ref, lookup, runner); err != nil {
		return err
	}

	children, found, err := lookup(*ref)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %s %q", ErrUnknownDependency, ref.Kind, ref.Name)
	}
	return v.visitChildren(id, children)
}

func (v *graphVisitor) visitChildren(id string, children []Ref) error {
	for i := range children {
		child := &children[i]
		if err := v.visit(child); err != nil {
			return err
		}
		if err := v.builder.AddDependency(id, child.NodeID()); err != nil {
			return err
		}
	}
	return nil
}

func validateLookupAndRunner(ref *Ref, lookup Lookup, runner Runner) error {
	if lookup == nil {
		return fmt.Errorf("%w: %s %q", ErrMissingLookup, ref.Kind, ref.Name)
	}
	if runner == nil {
		return fmt.Errorf("%w: %s %q", ErrMissingRunner, ref.Kind, ref.Name)
	}
	return nil
}

// buildGraph seeds a graphVisitor and visits every direct ref to build the full transitive
// dependency graph. Validation for cycles happens in dependency.GraphBuilder.Build, returning
// dependency.ErrCircularDependency -- ported nowhere else; that check is reused as-is.
func buildGraph(direct []Ref, options *Options) (*dependency.Graph, error) {
	v := &graphVisitor{
		builder: dependency.NewBuilder(),
		visited: make(map[string]struct{}),
		options: options,
	}
	for i := range direct {
		if err := v.visit(&direct[i]); err != nil {
			return nil, err
		}
	}
	return v.builder.Build()
}

func lookupAndRunnerFor(kind string, options *Options) (Lookup, Runner, error) {
	switch kind {
	case KindCommand:
		return options.commandLookup, options.commandRunner, nil
	case KindWorkflow:
		return options.workflowLookup, options.workflowRunner, nil
	default:
		return nil, nil, fmt.Errorf("%w: %q", ErrUnknownDependencyKind, kind)
	}
}

// newDispatcher adapts Options' Runner callbacks into a scheduler.Dispatcher. Mirrors
// pkg/workflow/control.go's newControlDispatcher convention: the scheduler fills in
// NodeID/Status from the returned error, so Dispatch only needs to return the error itself.
func newDispatcher(options *Options) scheduler.Dispatcher {
	return scheduler.DispatcherFunc(func(ctx context.Context, node *dependency.Node) (scheduler.Result, error) {
		ref, ok := node.Metadata["ref"].(Ref)
		if !ok {
			return scheduler.Result{}, fmt.Errorf("%w: node %q has no ref metadata", ErrUnknownDependencyKind, node.ID)
		}
		_, runner, err := lookupAndRunnerFor(ref.Kind, options)
		if err != nil {
			return scheduler.Result{Value: ref}, err
		}
		if runner == nil {
			return scheduler.Result{Value: ref}, fmt.Errorf("%w: %s %q", ErrMissingRunner, ref.Kind, ref.Name)
		}
		runErr := runner(ctx, ref)
		return scheduler.Result{Value: ref}, runErr
	})
}
