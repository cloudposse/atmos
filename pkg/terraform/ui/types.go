// Package ui provides a streaming TUI for Terraform operations.
// It parses Terraform's JSON streaming output and displays real-time
// resource status in a Docker-build-style inline interface.
package ui

import (
	"time"
)

// MessageType represents the type of Terraform JSON message.
type MessageType string

// Message types from Terraform's machine-readable UI.
const (
	MessageTypeVersion         MessageType = "version"
	MessageTypePlannedChange   MessageType = "planned_change"
	MessageTypeChangeSummary   MessageType = "change_summary"
	MessageTypeApplyStart      MessageType = "apply_start"
	MessageTypeApplyProgress   MessageType = "apply_progress"
	MessageTypeApplyComplete   MessageType = "apply_complete"
	MessageTypeApplyErrored    MessageType = "apply_errored"
	MessageTypeRefreshStart    MessageType = "refresh_start"
	MessageTypeRefreshComplete MessageType = "refresh_complete"
	MessageTypeDiagnostic      MessageType = "diagnostic"
	MessageTypeOutputs         MessageType = "outputs"
	MessageTypeResourceDrift   MessageType = "resource_drift"
	MessageTypeInitOutput      MessageType = "init_output"
	MessageTypeLog             MessageType = "log"
)

// BaseMessage contains fields common to all Terraform JSON messages.
type BaseMessage struct {
	Level     string      `json:"@level"`
	Message   string      `json:"@message"`
	Module    string      `json:"@module"`
	Timestamp string      `json:"@timestamp"`
	Type      MessageType `json:"type"`
}

// VersionMessage represents the initial version message.
type VersionMessage struct {
	BaseMessage
	Terraform string `json:"terraform"`
	UI        string `json:"ui"`
}

// ResourceAddr contains common resource identification fields.
type ResourceAddr struct {
	Addr            string `json:"addr"`
	Module          string `json:"module"`
	Resource        string `json:"resource"`
	ResourceType    string `json:"resource_type"`
	ResourceName    string `json:"resource_name"`
	ResourceKey     string `json:"resource_key,omitempty"`
	ImpliedProvider string `json:"implied_provider"`
}

// PlannedChange contains the change details for a planned resource.
type PlannedChange struct {
	Resource       ResourceAddr `json:"resource"`
	Action         string       `json:"action"`
	PreviousAction string       `json:"previous_action,omitempty"`
	Reason         string       `json:"reason,omitempty"`
}

// PlannedChangeMessage represents a planned resource change.
type PlannedChangeMessage struct {
	BaseMessage
	Change PlannedChange `json:"change"`
}

// ApplyHook contains the hook details for apply operations.
type ApplyHook struct {
	Resource    ResourceAddr `json:"resource"`
	Action      string       `json:"action"`
	IDKey       string       `json:"id_key,omitempty"`
	IDValue     string       `json:"id_value,omitempty"`
	ElapsedSecs int          `json:"elapsed_secs,omitempty"`
}

// ApplyStartMessage represents the start of a resource apply operation.
type ApplyStartMessage struct {
	BaseMessage
	Hook ApplyHook `json:"hook"`
}

// ApplyProgressMessage represents progress during apply.
type ApplyProgressMessage struct {
	BaseMessage
	Hook ApplyHook `json:"hook"`
}

// ApplyCompleteMessage represents successful completion of a resource apply.
type ApplyCompleteMessage struct {
	BaseMessage
	Hook ApplyHook `json:"hook"`
}

// ApplyErroredMessage represents a failed resource apply.
type ApplyErroredMessage struct {
	BaseMessage
	Hook ApplyHook `json:"hook"`
}

// RefreshHook contains the hook details for refresh operations.
type RefreshHook struct {
	Resource ResourceAddr `json:"resource"`
	IDKey    string       `json:"id_key,omitempty"`
	IDValue  string       `json:"id_value,omitempty"`
}

// RefreshStartMessage represents the start of a resource refresh.
type RefreshStartMessage struct {
	BaseMessage
	Hook RefreshHook `json:"hook"`
}

// RefreshCompleteMessage represents successful completion of a resource refresh.
type RefreshCompleteMessage struct {
	BaseMessage
	Hook RefreshHook `json:"hook"`
}

// DiagnosticLocation represents a source code location.
type DiagnosticLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Byte   int `json:"byte"`
}

// DiagnosticRange represents a source code range.
type DiagnosticRange struct {
	Filename string             `json:"filename"`
	Start    DiagnosticLocation `json:"start"`
	End      DiagnosticLocation `json:"end"`
}

// Diagnostic contains the diagnostic details.
type Diagnostic struct {
	Severity string           `json:"severity"` // error, warning.
	Summary  string           `json:"summary"`
	Detail   string           `json:"detail,omitempty"`
	Address  string           `json:"address,omitempty"`
	Range    *DiagnosticRange `json:"range,omitempty"`
}

// DiagnosticMessage represents warnings and errors.
type DiagnosticMessage struct {
	BaseMessage
	Diagnostic Diagnostic `json:"diagnostic"`
}

// Changes contains the change counts for a summary.
type Changes struct {
	Add       int    `json:"add"`
	Change    int    `json:"change"`
	Remove    int    `json:"remove"`
	Import    int    `json:"import,omitempty"`
	Operation string `json:"operation"` // plan, apply.
}

// ChangeSummaryMessage represents the summary of changes.
type ChangeSummaryMessage struct {
	BaseMessage
	Changes Changes `json:"changes"`
}

// OutputValue represents a single output value.
type OutputValue struct {
	Sensitive bool   `json:"sensitive"`
	Type      any    `json:"type"`
	Value     any    `json:"value"`
	Action    string `json:"action,omitempty"`
}

// OutputsMessage represents output values after successful operations.
type OutputsMessage struct {
	BaseMessage
	Outputs map[string]OutputValue `json:"outputs"`
}

// InitOutputMessage represents init command output.
type InitOutputMessage struct {
	BaseMessage
	InitOutput struct {
		// Init has various message subtypes.
		MessageType string `json:"message_type,omitempty"`
	} `json:"init_output,omitempty"`
}

// LogMessage represents unstructured log output.
type LogMessage struct {
	BaseMessage
	// Additional key-value pairs may be present.
}

// ResourceState represents the current state of a resource operation.
type ResourceState int

// Resource operation states.
const (
	ResourceStatePending ResourceState = iota
	ResourceStateRefreshing
	ResourceStateInProgress
	ResourceStateComplete
	ResourceStateError
)

// String returns the string representation of the resource state.
func (s ResourceState) String() string {
	switch s {
	case ResourceStatePending:
		return "pending"
	case ResourceStateRefreshing:
		return "refreshing"
	case ResourceStateInProgress:
		return "in_progress"
	case ResourceStateComplete:
		return "complete"
	case ResourceStateError:
		return "error"
	default:
		return "unknown"
	}
}

// ResourceOperation tracks the lifecycle of a single resource.
type ResourceOperation struct {
	Address      string        // e.g., "aws_instance.example".
	Module       string        // Module path if applicable.
	ResourceType string        // e.g., "aws_instance".
	ResourceName string        // e.g., "example".
	Action       string        // create, update, delete, read, no-op.
	State        ResourceState // Current state.
	StartTime    time.Time     // When operation started.
	EndTime      time.Time     // When operation completed.
	ElapsedSecs  int           // Elapsed time from terraform.
	Error        string        // Error message if failed.
	IDKey        string        // For existing resources.
	IDValue      string        // For existing resources.
	LastUpdate   time.Time     // For progress updates.
}

// Phase represents the terraform operation phase.
type Phase int

// Terraform operation phases.
const (
	PhaseInitializing Phase = iota
	PhaseRefreshing
	PhasePlanning
	PhaseApplying
	PhaseComplete
	PhaseError
)

// String returns the string representation of the phase.
func (p Phase) String() string {
	switch p {
	case PhaseInitializing:
		return "initializing"
	case PhaseRefreshing:
		return "refreshing"
	case PhasePlanning:
		return "planning"
	case PhaseApplying:
		return "applying"
	case PhaseComplete:
		return "complete"
	case PhaseError:
		return "error"
	default:
		return "unknown"
	}
}
