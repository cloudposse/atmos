// Package taskgraph resolves and executes named cross-unit dependencies
// (dependencies.commands / dependencies.workflows) declared on custom commands and
// workflows. It builds a pkg/dependency.Graph from the transitive closure of declared
// dependency references and runs it via the same generic pkg/scheduler DAG engine already
// used by parallel/matrix `needs:` (pkg/workflow/control.go) and Terraform's
// dependencies.components (pkg/scheduler/adapters/terraform.go) — concurrent by default,
// with automatic dedup of identical (kind, name, parameters) references.
package taskgraph

import "errors"

var (
	// ErrUnknownDependency is returned when a dependencies.commands/dependencies.workflows
	// entry references a name that does not resolve to any known command or workflow.
	ErrUnknownDependency = errors.New("unknown dependency")
	// ErrUnknownDependencyKind is returned when a Ref carries a Kind other than
	// KindCommand/KindWorkflow.
	ErrUnknownDependencyKind = errors.New("unknown dependency kind")
	// ErrMissingRunner is returned when Run is called without a CommandRunner/WorkflowRunner
	// configured for a Kind actually present in the dependency graph.
	ErrMissingRunner = errors.New("no runner configured for dependency kind")
	// ErrMissingLookup is returned when Run is called without a
	// CommandDependencyLookup/WorkflowDependencyLookup configured for a Kind actually
	// present in the dependency graph.
	ErrMissingLookup = errors.New("no dependency lookup configured for dependency kind")
)
