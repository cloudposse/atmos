// Package approval defines the contract between AI providers that run their own
// tools (CLI providers such as Claude Code) and the Atmos permission system.
//
// A provider asks an Approver whether a tool invocation may proceed and reports
// progress events (tool started/finished) so the UI can show what is happening.
package approval

import (
	"context"
	"time"
)

// Request describes a tool invocation that an AI provider wants to run.
type Request struct {
	// ToolName is the provider's name for the tool (for example "Bash").
	ToolName string
	// ToolUseID identifies the tool call within the provider's session.
	ToolUseID string
	// Input holds the tool parameters (for Bash: "command" and "description").
	Input map[string]any
}

// Decision is the outcome of an approval request.
type Decision struct {
	// Allow is true when the tool may run.
	Allow bool
	// Message is returned to the model when the tool is denied.
	Message string
	// Interrupt asks the provider to stop the whole run (user pressed Ctrl-C).
	Interrupt bool
	// NonInteractive is true when approval was required but no terminal was
	// available to ask. The provider reports this as an error with a hint
	// instead of surfacing the model's prose.
	NonInteractive bool
}

// Approver decides whether a provider may run a tool.
type Approver interface {
	Approve(ctx context.Context, req Request) (Decision, error)
}

// EventKind identifies a provider progress event.
type EventKind int

const (
	// ToolStart is emitted when the provider begins running a tool.
	ToolStart EventKind = iota + 1
	// ToolDone is emitted when the tool result has been received.
	ToolDone
)

// Event is a provider progress notification.
type Event struct {
	Kind EventKind
	// Tool is the provider's tool name (for example "Bash").
	Tool string
	// Summary is a short human-readable description (for example "atmos list stacks").
	Summary string
}

// ProgressReporter is implemented by providers that can report tool progress.
type ProgressReporter interface {
	SetProgressHandler(handler func(Event))
}

// Approvable is implemented by providers that can ask an Approver before running tools.
type Approvable interface {
	SetApprover(approver Approver)
}

// TimeoutManaged is implemented by providers that enforce the run timeout
// themselves so time spent waiting for a person (approval prompts) does not count.
// Callers must then use a cancel-only context.
type TimeoutManaged interface {
	SetTimeout(timeout time.Duration)
}
