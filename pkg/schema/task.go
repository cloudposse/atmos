package schema

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

// Task type constants.
const (
	// TaskTypeShell is the default task type for shell commands.
	TaskTypeShell = "shell"
	// TaskTypeAtmos is the task type for atmos commands.
	TaskTypeAtmos = "atmos"
)

// Sentinel errors for task validation.
var (
	// ErrTaskInvalidFormat is returned when a task has an invalid format.
	ErrTaskInvalidFormat = errors.New("invalid task format")
	// ErrTaskUnexpectedNodeKind is returned when a task node has an unexpected kind.
	ErrTaskUnexpectedNodeKind = errors.New("unexpected task node kind")
)

// Task represents a unit of work that can be executed.
// This type unifies workflow steps and custom command steps,
// supporting both simple string syntax and structured syntax.
type Task struct {
	// Name is an optional identifier for the task.
	Name string `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	// Command is the command to execute.
	Command string `yaml:"command" json:"command" mapstructure:"command"`
	// Type specifies the command type: TaskTypeShell or TaskTypeAtmos. Defaults to TaskTypeShell.
	Type string `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
	// Timeout specifies the maximum duration for the task. Zero means no timeout.
	Timeout time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`
	// Stack specifies the stack to use for atmos commands.
	Stack string `yaml:"stack,omitempty" json:"stack,omitempty" mapstructure:"stack"`
	// WorkingDirectory specifies the working directory for the command.
	WorkingDirectory string `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	// Retry specifies retry configuration for failed tasks.
	Retry *RetryConfig `yaml:"retry,omitempty" json:"retry,omitempty" mapstructure:"retry"`
	// Identity specifies the authentication identity to use.
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}

// Tasks is a slice of Task that supports flexible YAML unmarshaling.
// It can parse both simple string syntax and structured syntax:
//
// Simple syntax:
//
//	steps:
//	  - "echo hello"
//	  - "echo world"
//
// Structured syntax:
//
//	steps:
//	  - name: greet
//	    command: echo hello
//	    timeout: 30s
//
// Mixed syntax:
//
//	steps:
//	  - "echo simple"
//	  - name: complex
//	    command: echo structured
//	    timeout: 5m
type Tasks []Task

// UnmarshalYAML implements custom YAML unmarshaling for Tasks.
// It handles both string elements (simple syntax) and mapping elements (structured syntax).
func (t *Tasks) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("%w: expected sequence, got %v", ErrTaskInvalidFormat, value.Kind)
	}

	tasks := make([]Task, 0, len(value.Content))
	for i, node := range value.Content {
		var task Task
		switch node.Kind {
		case yaml.ScalarNode:
			// Simple string syntax: "echo hello" -> shell command.
			task.Command = node.Value
			task.Type = TaskTypeShell
		case yaml.MappingNode:
			// Structured syntax: {command: "echo hello", timeout: "30s"}.
			if err := node.Decode(&task); err != nil {
				return fmt.Errorf("failed to decode task at index %d: %w", i, err)
			}
			// Default type to TaskTypeShell if not specified.
			if task.Type == "" {
				task.Type = TaskTypeShell
			}
		default:
			return fmt.Errorf("%w at index %d: got %v (expected string or mapping)", ErrTaskUnexpectedNodeKind, i, node.Kind)
		}
		tasks = append(tasks, task)
	}
	*t = tasks
	return nil
}

// ToWorkflowStep converts a Task to a WorkflowStep for backward compatibility.
func (task *Task) ToWorkflowStep() WorkflowStep {
	return WorkflowStep{
		Name:             task.Name,
		Command:          task.Command,
		Type:             task.Type,
		Stack:            task.Stack,
		WorkingDirectory: task.WorkingDirectory,
		Retry:            task.Retry,
		Identity:         task.Identity,
	}
}

// TaskFromWorkflowStep creates a Task from a WorkflowStep.
func TaskFromWorkflowStep(step *WorkflowStep) Task {
	return Task{
		Name:             step.Name,
		Command:          step.Command,
		Type:             step.Type,
		Stack:            step.Stack,
		WorkingDirectory: step.WorkingDirectory,
		Retry:            step.Retry,
		Identity:         step.Identity,
		// Note: WorkflowStep doesn't have Timeout, so it remains zero.
	}
}

// TasksDecodeHook is a mapstructure decode hook that handles flexible Tasks parsing.
// It converts both string elements and map elements in a slice to Tasks.
// This hook should be used when unmarshaling configuration that contains Tasks fields.
func TasksDecodeHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		// Only handle conversion to Tasks type.
		if t != reflect.TypeOf(Tasks{}) {
			return data, nil
		}

		// Input must be a slice.
		if f.Kind() != reflect.Slice {
			return data, nil
		}

		// Get the slice data.
		slice, ok := data.([]any)
		if !ok {
			return data, nil
		}

		return decodeTasksFromSlice(slice)
	}
}

// decodeTasksFromSlice converts a slice of interface{} values into Tasks.
// Each element can be either a string (simple syntax) or a map (structured syntax).
func decodeTasksFromSlice(slice []any) (Tasks, error) {
	tasks := make(Tasks, 0, len(slice))
	for i, item := range slice {
		task, err := decodeTaskItem(item, i)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// decodeTaskItem converts a single item (string or map) into a Task.
func decodeTaskItem(item any, index int) (Task, error) {
	switch v := item.(type) {
	case string:
		// Simple string syntax: "echo hello" -> shell command.
		return Task{Command: v, Type: TaskTypeShell}, nil
	case map[string]any:
		return decodeTaskFromMap(v, index)
	default:
		return Task{}, fmt.Errorf("%w at index %d: got %T (expected string or map)", ErrTaskUnexpectedNodeKind, index, item)
	}
}

// decodeTaskFromMap decodes a map into a Task using mapstructure.
func decodeTaskFromMap(m map[string]any, index int) (Task, error) {
	var task Task
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &task,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
	})
	if err != nil {
		return Task{}, fmt.Errorf("failed to create decoder for task at index %d: %w", index, err)
	}
	if err := decoder.Decode(m); err != nil {
		return Task{}, fmt.Errorf("failed to decode task at index %d: %w", index, err)
	}
	// Default type to TaskTypeShell if not specified.
	if task.Type == "" {
		task.Type = TaskTypeShell
	}
	return task, nil
}
