package schema

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"gopkg.in/yaml.v3"
)

// Task type constants.
const (
	// TaskTypeShell is the default task type for shell commands.
	TaskTypeShell = "shell"
	// TaskTypeAtmos is the task type for atmos commands.
	TaskTypeAtmos = "atmos"
	// TaskTypeParallel is the task type for running nested steps concurrently.
	TaskTypeParallel = "parallel"
	// TaskTypeMatrix is the task type for expanding and running nested steps concurrently.
	TaskTypeMatrix = "matrix"
	// TaskTypeExec is the task type for commands that replace the Atmos
	// process entirely (shell exec semantics). Must be the final step.
	TaskTypeExec = "exec"
	// TaskTypeWait is the action step that blocks until the background step(s)
	// named in `for:` are ready (a service's health check) or complete.
	TaskTypeWait = "wait"
	// TaskTypeWaitAll is the action step that blocks until all background steps
	// in scope are ready/complete.
	TaskTypeWaitAll = "wait-all"
	// TaskTypeCancel is the action step that gracefully tears down the background
	// step(s) named in `for:`.
	TaskTypeCancel = "cancel"
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
	// Type specifies the command type: TaskTypeShell, TaskTypeAtmos, or TaskTypeExec. Defaults to TaskTypeShell.
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
	// Needs lists sibling task names that must complete before this task can run.
	Needs []string `yaml:"needs,omitempty" json:"needs,omitempty" mapstructure:"needs"`
	// When controls whether the task runs.
	When Condition `yaml:"when,omitempty" json:"when,omitempty" mapstructure:"when"`
	// Interactive attaches host stdin to the step and lets the step handle Ctrl-C (like docker -i).
	Interactive bool `yaml:"interactive,omitempty" json:"interactive,omitempty" mapstructure:"interactive"`
	// Tty allocates a pseudo-terminal for the step (like docker -t). Combine with interactive for full terminal sessions.
	Tty bool `yaml:"tty,omitempty" json:"tty,omitempty" mapstructure:"tty"`

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
	Output         string                `yaml:"output,omitempty" json:"output,omitempty" mapstructure:"output"`       // Output mode: viewport, raw, log, none.
	ParallelOutput *ParallelOutputConfig `yaml:"-" json:"parallel_output,omitempty" mapstructure:"parallel_output"`    // Structured output for parallel/matrix.
	Height         int                   `yaml:"height,omitempty" json:"height,omitempty" mapstructure:"height"`       // Height for write type (editor lines).
	Viewport       *ViewportConfig       `yaml:"viewport,omitempty" json:"viewport,omitempty" mapstructure:"viewport"` // Viewport settings for output mode.
	Count          int                   `yaml:"count,omitempty" json:"count,omitempty" mapstructure:"count"`          // Count for linebreak type.

	// Style step fields (like gum style).
	Foreground string `yaml:"foreground,omitempty" json:"foreground,omitempty" mapstructure:"foreground"` // Foreground color.
	// Background is the style background color. The YAML key `background:` is polymorphic
	// (see UnmarshalYAML): a string value sets this color; a boolean value sets BackgroundAsync.
	Background       string `yaml:"-" json:"background,omitempty" mapstructure:"background"`                                         // Background color (string-valued `background:`).
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

	// Say step fields.
	Voice []string `yaml:"voice,omitempty" json:"voice,omitempty" mapstructure:"voice"` // Ordered voice candidates; first one installed on the host wins.
	Rate  string   `yaml:"rate,omitempty" json:"rate,omitempty" mapstructure:"rate"`    // Speech rate: slow, normal, fast.
	Print string   `yaml:"print,omitempty" json:"print,omitempty" mapstructure:"print"` // Print policy: fallback, always, never.

	// Environment variables (supports templates).
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`

	// Env step type fields.
	Vars map[string]string `yaml:"vars,omitempty" json:"vars,omitempty" mapstructure:"vars"` // Variables to set for env step type.

	// Exit step type fields.
	Code int `yaml:"code,omitempty" json:"code,omitempty" mapstructure:"code"` // Exit code for exit step type.

	// HTTP step type fields (type: http; also accepts the alias type: webhook).
	URL     string            `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url"`             // Request URL (required, supports templates).
	Method  string            `yaml:"method,omitempty" json:"method,omitempty" mapstructure:"method"`    // HTTP method/verb: GET (default), POST, PUT, PATCH, DELETE, HEAD, OPTIONS.
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty" mapstructure:"headers"` // Request headers (supports templates).
	Query   map[string]string `yaml:"query,omitempty" json:"query,omitempty" mapstructure:"query"`       // Query-string parameters (supports templates).
	Body    string            `yaml:"body,omitempty" json:"body,omitempty" mapstructure:"body"`          // Raw request body (supports templates); mutually exclusive with form.
	Form    map[string]string `yaml:"form,omitempty" json:"form,omitempty" mapstructure:"form"`          // Form/JSON body params; mutually exclusive with body.
	Expect  *HTTPExpect       `yaml:"expect,omitempty" json:"expect,omitempty" mapstructure:"expect"`    // Success criteria; defaults to any 2xx.

	// Container step fields.
	//
	// Action selects the container verb; its parameters are supplied under the single
	// `with:` key. Build/Push/Run/Inspect are populated from `with:` by UnmarshalYAML
	// based on Action, so they carry no YAML key (see decodeContainerWith).
	//
	// Only cross-cutting execution modifiers stay top-level (provider, runtime_auto_start,
	// container). All action parameters — image, command, ports, mounts, healthcheck, etc. —
	// live under `with:` (decoded into Build/Run/Push/Inspect).
	Action           string                `yaml:"action,omitempty" json:"action,omitempty" mapstructure:"action"` // build, push, run, inspect.
	Build            *ContainerBuildStep   `yaml:"-" json:"build,omitempty" mapstructure:"build"`
	Push             *ContainerPushStep    `yaml:"-" json:"push,omitempty" mapstructure:"push"`
	Run              *ContainerRunStep     `yaml:"-" json:"run,omitempty" mapstructure:"run"`
	Inspect          *ContainerInspectStep `yaml:"-" json:"inspect,omitempty" mapstructure:"inspect"`
	RuntimeAutoStart bool                  `yaml:"runtime_auto_start,omitempty" json:"runtime_auto_start,omitempty" mapstructure:"runtime_auto_start"`
	Provider         string                `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`    // docker, podman, or empty for auto-detect.
	Container        *WorkflowContainer    `yaml:"container,omitempty" json:"container,omitempty" mapstructure:"container"` // Workflow container override or false to run on host.

	// Outputs declares named outputs derived from the step result.
	Outputs map[string]string `yaml:"outputs,omitempty" json:"outputs,omitempty" mapstructure:"outputs"`

	// Show configuration for this step (overrides workflow-level show settings).
	Show *ShowConfig `yaml:"show,omitempty" json:"show,omitempty" mapstructure:"show"`

	// Control step fields.
	Steps          []WorkflowStep      `yaml:"steps,omitempty" json:"steps,omitempty" mapstructure:"steps"`
	MaxConcurrency int                 `yaml:"max_concurrency,omitempty" json:"max_concurrency,omitempty" mapstructure:"max_concurrency"`
	Matrix         map[string][]string `yaml:"matrix,omitempty" json:"matrix,omitempty" mapstructure:"matrix"`
	Fail           *ParallelFailConfig `yaml:"fail,omitempty" json:"fail,omitempty" mapstructure:"fail"`

	// BackgroundAsync marks a container step to run asynchronously (decoded from a
	// boolean-valued `background:` key); a string-valued `background:` sets the style color.
	// In v1 the validator accepts `background: true` only on `type: container` steps.
	BackgroundAsync bool `yaml:"-" json:"background_async,omitempty" mapstructure:"background_async"`
	// For lists the background step name(s) a `wait`/`cancel` action step targets.
	For []string `yaml:"-" json:"for,omitempty" mapstructure:"for"`

	// DryRun is set by executors and is not read from user configuration.
	DryRun bool `yaml:"-" json:"-" mapstructure:"-"`
}

// UnmarshalYAML handles keys whose meaning depends on shape or a sibling field:
//   - `output`     : scalar mode string or a structured ParallelOutputConfig.
//   - `with`       : the container action's parameters, decoded into Build/Run/Push/Inspect by `action`.
//   - `background` : boolean async marker, or a string style color.
//   - `for`        : scalar or sequence of target step names (wait/cancel).
func (task *Task) UnmarshalYAML(value *yaml.Node) error {
	type plain Task
	// Decode into a zero-value temp first so a reused receiver does not retain
	// fields omitted from this YAML node (Decode merges into the destination).
	var fresh plain
	nodes, sanitized := splitStepPolymorphicNodes(value)
	if err := sanitized.Decode(&fresh); err != nil {
		return err
	}
	*task = Task(fresh)
	return applyStepPolymorphicNodes(nodes, task.Action, stepPolyTargets{
		output:    &task.Output,
		parallel:  &task.ParallelOutput,
		async:     &task.BackgroundAsync,
		color:     &task.Background,
		forList:   &task.For,
		container: containerActionTargets{Build: &task.Build, Run: &task.Run, Push: &task.Push, Inspect: &task.Inspect},
	})
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
		Needs:            task.Needs,
		When:             task.When,
		Interactive:      task.Interactive,
		Tty:              task.Tty,

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
		Output:         task.Output,
		ParallelOutput: task.ParallelOutput,
		Height:         task.Height,
		Viewport:       task.Viewport,
		Timeout:        timeoutStr,
		Count:          task.Count,

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

		// Say step fields.
		Voice: task.Voice,
		Rate:  task.Rate,
		Print: task.Print,

		// Environment variables.
		Env: task.Env,

		// Env step type fields.
		Vars: task.Vars,

		// Exit step type fields.
		Code: task.Code,

		// HTTP step type fields.
		URL:     task.URL,
		Method:  task.Method,
		Headers: task.Headers,
		Query:   task.Query,
		Body:    task.Body,
		Form:    task.Form,
		Expect:  task.Expect,

		// Container step fields.
		Action:           task.Action,
		Build:            task.Build,
		Push:             task.Push,
		Run:              task.Run,
		Inspect:          task.Inspect,
		RuntimeAutoStart: task.RuntimeAutoStart,
		Provider:         task.Provider,
		Container:        task.Container,

		Outputs: task.Outputs,

		// Show configuration.
		Show: task.Show,

		// Control step fields.
		Steps:           task.Steps,
		MaxConcurrency:  task.MaxConcurrency,
		Matrix:          task.Matrix,
		Fail:            task.Fail,
		BackgroundAsync: task.BackgroundAsync,
		For:             task.For,

		DryRun: task.DryRun,
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
		Needs:            step.Needs,
		When:             step.When,
		Interactive:      step.Interactive,
		Tty:              step.Tty,
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
		Output:         step.Output,
		ParallelOutput: step.ParallelOutput,
		Height:         step.Height,
		Viewport:       step.Viewport,
		Count:          step.Count,

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

		// Say step fields.
		Voice: step.Voice,
		Rate:  step.Rate,
		Print: step.Print,

		// Environment variables.
		Env: step.Env,

		// Env step type fields.
		Vars: step.Vars,

		// Exit step type fields.
		Code: step.Code,

		// HTTP step type fields.
		URL:     step.URL,
		Method:  step.Method,
		Headers: step.Headers,
		Query:   step.Query,
		Body:    step.Body,
		Form:    step.Form,
		Expect:  step.Expect,

		// Container step fields.
		Action:           step.Action,
		Build:            step.Build,
		Push:             step.Push,
		Run:              step.Run,
		Inspect:          step.Inspect,
		RuntimeAutoStart: step.RuntimeAutoStart,
		Provider:         step.Provider,
		Container:        step.Container,

		Outputs: step.Outputs,

		// Show configuration.
		Show: step.Show,

		// Control step fields.
		Steps:           step.Steps,
		MaxConcurrency:  step.MaxConcurrency,
		Matrix:          step.Matrix,
		Fail:            step.Fail,
		BackgroundAsync: step.BackgroundAsync,
		For:             step.For,

		DryRun: step.DryRun,
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
	m, err := normalizeTaskOutputMap(m, &task)
	if err != nil {
		return Task{}, fmt.Errorf("failed to decode task output at index %d: %w", index, err)
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &task,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			ConditionDecodeHook(),
		),
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

func normalizeTaskOutputMap(m map[string]any, task *Task) (map[string]any, error) {
	output, ok := m["output"]
	if !ok {
		return m, nil
	}
	switch v := output.(type) {
	case string:
		return m, nil
	case map[string]any:
		var cfg ParallelOutputConfig
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:           &cfg,
			TagName:          "mapstructure",
			WeaklyTypedInput: true,
		})
		if err != nil {
			return nil, err
		}
		if err := decoder.Decode(v); err != nil {
			return nil, err
		}
		task.Output = cfg.Mode
		task.ParallelOutput = &cfg
		copied := make(map[string]any, len(m)-1)
		for key, val := range m {
			if key == "output" {
				continue
			}
			copied[key] = val
		}
		return copied, nil
	default:
		return m, nil
	}
}
