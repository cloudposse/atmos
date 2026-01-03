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
	// Core fields.
	Name string `yaml:"name,omitempty" json:"name,omitempty" mapstructure:"name"`
	// Command is the command to execute.
	Command string `yaml:"command,omitempty" json:"command,omitempty" mapstructure:"command"`
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

	// Interactive step fields.
	Prompt      string   `yaml:"prompt,omitempty" json:"prompt,omitempty" mapstructure:"prompt"`                // Prompt text for interactive types.
	Options     []string `yaml:"options,omitempty" json:"options,omitempty" mapstructure:"options"`             // Options for choose/filter.
	Default     string   `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`             // Default value.
	Placeholder string   `yaml:"placeholder,omitempty" json:"placeholder,omitempty" mapstructure:"placeholder"` // Input placeholder.
	Password    bool     `yaml:"password,omitempty" json:"password,omitempty" mapstructure:"password"`          // Mask input.
	Multiple    bool     `yaml:"multiple,omitempty" json:"multiple,omitempty" mapstructure:"multiple"`          // Allow multiple selection.
	Limit       int      `yaml:"limit,omitempty" json:"limit,omitempty" mapstructure:"limit"`                   // Selection limit.

	// Output/UI step fields.
	Content   string           `yaml:"content,omitempty" json:"content,omitempty" mapstructure:"content"`       // Content for output types (supports templates).
	Title     string           `yaml:"title,omitempty" json:"title,omitempty" mapstructure:"title"`             // Title for spin/pager.
	Data      []map[string]any `yaml:"data,omitempty" json:"data,omitempty" mapstructure:"data"`                // Data for table type.
	Columns   []string         `yaml:"columns,omitempty" json:"columns,omitempty" mapstructure:"columns"`       // Columns for table.
	Separator string           `yaml:"separator,omitempty" json:"separator,omitempty" mapstructure:"separator"` // Separator for join type (default: newline).

	// File picker fields.
	Path       string   `yaml:"path,omitempty" json:"path,omitempty" mapstructure:"path"`                   // Starting path for file picker.
	Extensions []string `yaml:"extensions,omitempty" json:"extensions,omitempty" mapstructure:"extensions"` // File extensions filter.

	// Display configuration.
	Output   string          `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`       // Output mode: viewport, raw, log, none.
	Height   int             `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"`       // Height for write type (editor lines).
	Viewport *ViewportConfig `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"` // Viewport settings for output mode.
	Count    int             `yaml:"count,omitempty" json:"count,omitempty" mapstructure:"count"`          // Count for linebreak type.

	// Style step fields (like gum style).
	Foreground       string `yaml:"foreground,omitempty" json:"foreground,omitempty" mapstructure:"foreground"`                      // Foreground color.
	Background       string `yaml:"background,omitempty" json:"background,omitempty" mapstructure:"background"`                      // Background color.
	Border           string `yaml:"border,omitempty" json:"border,omitempty" mapstructure:"border"`                                  // Border style: none, hidden, normal, rounded, thick, double.
	BorderForeground string `yaml:"border_foreground,omitempty" json:"border_foreground,omitempty" mapstructure:"border_foreground"` // Border foreground color.
	BorderBackground string `yaml:"border_background,omitempty" json:"border_background,omitempty" mapstructure:"border_background"` // Border background color.
	Padding          string `yaml:"padding,omitempty" json:"padding,omitempty" mapstructure:"padding"`                               // Padding: "1" or "1 2" or "1 2 1 2" (top, right, bottom, left).
	Margin           string `yaml:"margin,omitempty" json:"margin,omitempty" mapstructure:"margin"`                                  // Margin: "1" or "1 2" or "1 2 1 2" (top, right, bottom, left).
	Width            int    `yaml:"width,omitempty" json:"width,omitempty" mapstructure:"width"`                                     // Fixed width.
	Align            string `yaml:"align,omitempty" json:"align,omitempty" mapstructure:"align"`                                     // Text alignment: left, center, right.
	Bold             bool   `yaml:"bold,omitempty" json:"bold,omitempty" mapstructure:"bold"`                                        // Bold text.
	Italic           bool   `yaml:"italic,omitempty" json:"italic,omitempty" mapstructure:"italic"`                                  // Italic text.
	Underline        bool   `yaml:"underline,omitempty" json:"underline,omitempty" mapstructure:"underline"`                         // Underline text.
	Strikethrough    bool   `yaml:"strikethrough,omitempty" json:"strikethrough,omitempty" mapstructure:"strikethrough"`             // Strikethrough text.
	Faint            bool   `yaml:"faint,omitempty" json:"faint,omitempty" mapstructure:"faint"`                                     // Faint/dim text.
	Markdown         bool   `yaml:"markdown,omitempty" json:"markdown,omitempty" mapstructure:"markdown"`                            // Render content as markdown.

	// Log step fields.
	Level  string            `yaml:"level,omitempty" json:"level,omitempty" mapstructure:"level"`    // Log level: trace, debug, info, warn, error.
	Fields map[string]string `yaml:"fields,omitempty" json:"fields,omitempty" mapstructure:"fields"` // Structured log fields (key-value pairs).

	// Environment variables (supports templates).
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`

	// Env step type fields.
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty" mapstructure:"vars"` // Variables to set for env step type.

	// Exit step type fields.
	Code int `yaml:"code,omitempty" json:"code,omitempty" mapstructure:"code"` // Exit code for exit step type.

	// Show configuration for this step (overrides workflow-level show settings).
	Show *ShowConfig `yaml:"show,omitempty" json:"show,omitempty" mapstructure:"show"`
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
	// Convert time.Duration to string for WorkflowStep.
	var timeoutStr string
	if task.Timeout > 0 {
		timeoutStr = task.Timeout.String()
	}

	return WorkflowStep{
		// Core fields.
		Name:             task.Name,
		Command:          task.Command,
		Type:             task.Type,
		Stack:            task.Stack,
		WorkingDirectory: task.WorkingDirectory,
		Retry:            task.Retry,
		Identity:         task.Identity,

		// Interactive step fields.
		Prompt:      task.Prompt,
		Options:     task.Options,
		Default:     task.Default,
		Placeholder: task.Placeholder,
		Password:    task.Password,
		Multiple:    task.Multiple,
		Limit:       task.Limit,

		// Output/UI step fields.
		Content:   task.Content,
		Title:     task.Title,
		Data:      task.Data,
		Columns:   task.Columns,
		Separator: task.Separator,

		// File picker fields.
		Path:       task.Path,
		Extensions: task.Extensions,

		// Display configuration.
		Output:   task.Output,
		Height:   task.Height,
		Viewport: task.Viewport,
		Timeout:  timeoutStr,
		Count:    task.Count,

		// Style step fields.
		Foreground:       task.Foreground,
		Background:       task.Background,
		Border:           task.Border,
		BorderForeground: task.BorderForeground,
		BorderBackground: task.BorderBackground,
		Padding:          task.Padding,
		Margin:           task.Margin,
		Width:            task.Width,
		Align:            task.Align,
		Bold:             task.Bold,
		Italic:           task.Italic,
		Underline:        task.Underline,
		Strikethrough:    task.Strikethrough,
		Faint:            task.Faint,
		Markdown:         task.Markdown,

		// Log step fields.
		Level:  task.Level,
		Fields: task.Fields,

		// Environment variables.
		Env: task.Env,

		// Env step type fields.
		Vars: task.Vars,

		// Exit step type fields.
		Code: task.Code,

		// Show configuration.
		Show: task.Show,
	}
}

// TaskFromWorkflowStep creates a Task from a WorkflowStep.
func TaskFromWorkflowStep(step *WorkflowStep) Task {
	// Parse timeout string to time.Duration.
	var timeout time.Duration
	if step.Timeout != "" {
		if parsed, err := time.ParseDuration(step.Timeout); err == nil {
			timeout = parsed
		}
	}

	return Task{
		// Core fields.
		Name:             step.Name,
		Command:          step.Command,
		Type:             step.Type,
		Stack:            step.Stack,
		WorkingDirectory: step.WorkingDirectory,
		Retry:            step.Retry,
		Identity:         step.Identity,
		Timeout:          timeout,

		// Interactive step fields.
		Prompt:      step.Prompt,
		Options:     step.Options,
		Default:     step.Default,
		Placeholder: step.Placeholder,
		Password:    step.Password,
		Multiple:    step.Multiple,
		Limit:       step.Limit,

		// Output/UI step fields.
		Content:   step.Content,
		Title:     step.Title,
		Data:      step.Data,
		Columns:   step.Columns,
		Separator: step.Separator,

		// File picker fields.
		Path:       step.Path,
		Extensions: step.Extensions,

		// Display configuration.
		Output:   step.Output,
		Height:   step.Height,
		Viewport: step.Viewport,
		Count:    step.Count,

		// Style step fields.
		Foreground:       step.Foreground,
		Background:       step.Background,
		Border:           step.Border,
		BorderForeground: step.BorderForeground,
		BorderBackground: step.BorderBackground,
		Padding:          step.Padding,
		Margin:           step.Margin,
		Width:            step.Width,
		Align:            step.Align,
		Bold:             step.Bold,
		Italic:           step.Italic,
		Underline:        step.Underline,
		Strikethrough:    step.Strikethrough,
		Faint:            step.Faint,
		Markdown:         step.Markdown,

		// Log step fields.
		Level:  step.Level,
		Fields: step.Fields,

		// Environment variables.
		Env: step.Env,

		// Env step type fields.
		Vars: step.Vars,

		// Exit step type fields.
		Code: step.Code,

		// Show configuration.
		Show: step.Show,
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
