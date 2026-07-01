package schema

import (
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTasks_UnmarshalYAML_SimpleStrings(t *testing.T) {
	input := `
- "echo hello"
- "echo world"
- ls -la
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 3)

	assert.Equal(t, "echo hello", tasks[0].Command)
	assert.Equal(t, TaskTypeShell, tasks[0].Type)
	assert.Empty(t, tasks[0].Name)

	assert.Equal(t, "echo world", tasks[1].Command)
	assert.Equal(t, TaskTypeShell, tasks[1].Type)

	assert.Equal(t, "ls -la", tasks[2].Command)
	assert.Equal(t, TaskTypeShell, tasks[2].Type)
}

func TestTasks_UnmarshalYAML_StructuredSyntax(t *testing.T) {
	input := `
- name: validate
  command: terraform validate
  type: shell
  timeout: 30s
- name: plan
  command: terraform plan vpc
  type: atmos
  stack: dev-us-east-1
  timeout: 5m
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 2)

	assert.Equal(t, "validate", tasks[0].Name)
	assert.Equal(t, "terraform validate", tasks[0].Command)
	assert.Equal(t, TaskTypeShell, tasks[0].Type)
	assert.Equal(t, 30*time.Second, tasks[0].Timeout)

	assert.Equal(t, "plan", tasks[1].Name)
	assert.Equal(t, "terraform plan vpc", tasks[1].Command)
	assert.Equal(t, TaskTypeAtmos, tasks[1].Type)
	assert.Equal(t, "dev-us-east-1", tasks[1].Stack)
	assert.Equal(t, 5*time.Minute, tasks[1].Timeout)
}

func TestTasks_UnmarshalYAML_MixedSyntax(t *testing.T) {
	input := `
- "echo simple string"
- name: structured
  command: echo with timeout
  timeout: 10s
- another simple command
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 3)

	// First: simple string.
	assert.Equal(t, "echo simple string", tasks[0].Command)
	assert.Equal(t, TaskTypeShell, tasks[0].Type)
	assert.Empty(t, tasks[0].Name)
	assert.Zero(t, tasks[0].Timeout)

	// Second: structured.
	assert.Equal(t, "structured", tasks[1].Name)
	assert.Equal(t, "echo with timeout", tasks[1].Command)
	assert.Equal(t, TaskTypeShell, tasks[1].Type) // defaults to shell.
	assert.Equal(t, 10*time.Second, tasks[1].Timeout)

	// Third: simple string.
	assert.Equal(t, "another simple command", tasks[2].Command)
	assert.Equal(t, TaskTypeShell, tasks[2].Type)
}

func TestTasks_UnmarshalYAML_DefaultsTypeToShell(t *testing.T) {
	input := `
- name: no-type-specified
  command: echo hello
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 1)
	assert.Equal(t, TaskTypeShell, tasks[0].Type)
}

func TestTasks_UnmarshalYAML_WithRetry(t *testing.T) {
	input := `
- name: flaky-task
  command: curl http://example.com
  retry:
    max_attempts: 3
    initial_delay: 1s
    max_delay: 10s
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].Retry)
	require.NotNil(t, tasks[0].Retry.MaxAttempts)
	assert.Equal(t, 3, *tasks[0].Retry.MaxAttempts)
	require.NotNil(t, tasks[0].Retry.InitialDelay)
	assert.Equal(t, time.Second, *tasks[0].Retry.InitialDelay)
	require.NotNil(t, tasks[0].Retry.MaxDelay)
	assert.Equal(t, 10*time.Second, *tasks[0].Retry.MaxDelay)
}

func TestTasks_UnmarshalYAML_WithWorkingDirectory(t *testing.T) {
	input := `
- name: build
  command: make build
  working_directory: /app/src
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 1)
	assert.Equal(t, "/app/src", tasks[0].WorkingDirectory)
}

func TestTasks_UnmarshalYAML_WithIdentity(t *testing.T) {
	input := `
- name: deploy
  command: terraform apply
  type: atmos
  identity: production-deployer
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 1)
	assert.Equal(t, "production-deployer", tasks[0].Identity)
}

func TestTasks_UnmarshalYAML_WithInteractiveAndTty(t *testing.T) {
	input := `
- command: aws ssm start-session --target i-1234567890
  interactive: true
  tty: true
- command: echo plain
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	require.Len(t, tasks, 2)
	assert.Equal(t, "aws ssm start-session --target i-1234567890", tasks[0].Command)
	assert.True(t, tasks[0].Interactive)
	assert.True(t, tasks[0].Tty)
	// Defaults are false for both fields.
	assert.Equal(t, "echo plain", tasks[1].Command)
	assert.False(t, tasks[1].Interactive)
	assert.False(t, tasks[1].Tty)
}

func TestTasksDecodeHook_InteractiveAndTtyWeaklyTyped(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"command":     "top",
				"interactive": "true",
				"tty":         "true",
			},
		},
	}

	var result testConfigWithTasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       TasksDecodeHook(),
	})
	require.NoError(t, err)

	err = decoder.Decode(input)
	require.NoError(t, err)

	require.Len(t, result.Steps, 1)
	assert.True(t, result.Steps[0].Interactive)
	assert.True(t, result.Steps[0].Tty)
}

func TestTasks_UnmarshalYAML_EmptyList(t *testing.T) {
	input := `[]`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.NoError(t, err)

	assert.Len(t, tasks, 0)
}

func TestTasks_UnmarshalYAML_InvalidNotSequence(t *testing.T) {
	input := `command: echo hello`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTaskInvalidFormat)
}

func TestTasks_UnmarshalYAML_InvalidNestedSequence(t *testing.T) {
	input := `
- - nested
  - sequence
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTaskUnexpectedNodeKind)
}

func TestTasks_UnmarshalYAML_InvalidStructuredDecode(t *testing.T) {
	// This tests line 93-94: error case when node.Decode fails.
	input := `
- command: valid
  timeout: not-a-duration
`
	var tasks Tasks
	err := yaml.Unmarshal([]byte(input), &tasks)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode task at index 0")
}

func TestTask_ToWorkflowStep(t *testing.T) {
	maxAttempts := 3
	task := Task{
		Name:             "test-task",
		Command:          "echo hello",
		Type:             TaskTypeShell,
		Stack:            "dev",
		WorkingDirectory: "/app",
		Identity:         "test-identity",
		Interactive:      true,
		Tty:              true,
		Voice:            []string{"Alex", "Samantha"},
		Rate:             "fast",
		Print:            "always",
		When:             MustCondition("ci"),
		Retry: &RetryConfig{
			MaxAttempts: &maxAttempts,
		},
		Timeout: 30 * time.Second,
	}

	step := task.ToWorkflowStep()

	assert.Equal(t, task.Name, step.Name)
	assert.Equal(t, task.Command, step.Command)
	assert.Equal(t, task.Type, step.Type)
	assert.Equal(t, task.Stack, step.Stack)
	assert.Equal(t, task.WorkingDirectory, step.WorkingDirectory)
	assert.Equal(t, task.Identity, step.Identity)
	assert.Equal(t, task.Interactive, step.Interactive)
	assert.Equal(t, task.Tty, step.Tty)
	assert.Equal(t, task.Voice, step.Voice)
	assert.Equal(t, task.Rate, step.Rate)
	assert.Equal(t, task.Print, step.Print)
	assert.True(t, step.When.Evaluate(ConditionContext{CI: true}))
	assert.False(t, step.When.Evaluate(ConditionContext{CI: false}))
	assert.Equal(t, task.Retry, step.Retry)
	// Note: Timeout is not in WorkflowStep.
}

func TestTaskFromWorkflowStep(t *testing.T) {
	maxAttempts := 5
	step := WorkflowStep{
		Name:             "workflow-step",
		Command:          "terraform apply",
		Type:             TaskTypeAtmos,
		Stack:            "prod",
		WorkingDirectory: "/infra",
		Identity:         "prod-identity",
		Interactive:      true,
		Tty:              true,
		Voice:            []string{"Moira", "Alex"},
		Rate:             "slow",
		Print:            "fallback",
		When:             MustCondition("local"),
		Retry: &RetryConfig{
			MaxAttempts: &maxAttempts,
		},
	}

	task := TaskFromWorkflowStep(&step)

	assert.Equal(t, step.Name, task.Name)
	assert.Equal(t, step.Command, task.Command)
	assert.Equal(t, step.Type, task.Type)
	assert.Equal(t, step.Stack, task.Stack)
	assert.Equal(t, step.WorkingDirectory, task.WorkingDirectory)
	assert.Equal(t, step.Identity, task.Identity)
	assert.Equal(t, step.Interactive, task.Interactive)
	assert.Equal(t, step.Tty, task.Tty)
	assert.Equal(t, step.Voice, task.Voice)
	assert.Equal(t, step.Rate, task.Rate)
	assert.Equal(t, step.Print, task.Print)
	assert.True(t, task.When.Evaluate(ConditionContext{CI: false}))
	assert.False(t, task.When.Evaluate(ConditionContext{CI: true}))
	assert.Equal(t, step.Retry, task.Retry)
	assert.Zero(t, task.Timeout) // WorkflowStep doesn't have Timeout.
}

func TestTaskWorkflowStepControlFieldsRoundTrip(t *testing.T) {
	showSummary := false
	task := Task{
		Name:             "matrix",
		Type:             TaskTypeMatrix,
		Needs:            []string{"prepare"},
		Output:           "grouped",
		ParallelOutput:   &ParallelOutputConfig{Mode: "grouped", Order: "definition", ShowSummary: &showSummary, Prefix: "{{ .step.name }}"},
		Timeout:          2 * time.Minute,
		Steps:            []WorkflowStep{{Name: "plan", Type: TaskTypeAtmos, Command: "terraform plan"}},
		MaxConcurrency:   3,
		Matrix:           map[string][]string{"stack": {"dev", "prod"}},
		Fail:             &ParallelFailConfig{Mode: "fail_fast", MaxFailures: 2},
		Viewport:         &ViewportConfig{Height: 10, Width: 80},
		Env:              map[string]string{"ENV": "test"},
		Vars:             map[string]string{"VAR": "value"},
		Fields:           map[string]string{"level": "debug"},
		Data:             []map[string]any{{"key": "value"}},
		Extensions:       []string{".yaml"},
		Columns:          []string{"name"},
		Options:          []string{"yes", "no"},
		Interactive:      true,
		Tty:              true,
		Password:         true,
		Multiple:         true,
		Show:             &ShowConfig{},
		Retry:            &RetryConfig{},
		WorkingDirectory: "/work",
		Identity:         "id",
		Stack:            "dev",
		Command:          "run",
		Script:           "print('ok')",
		Interpreter:      "python3",
	}

	step := task.ToWorkflowStep()
	assert.Equal(t, "2m0s", step.Timeout)
	assert.Equal(t, task.ParallelOutput, step.ParallelOutput)
	assert.Equal(t, task.Steps, step.Steps)
	assert.Equal(t, task.MaxConcurrency, step.MaxConcurrency)
	assert.Equal(t, task.Matrix, step.Matrix)
	assert.Equal(t, task.Fail, step.Fail)

	roundTripped := TaskFromWorkflowStep(&step)
	assert.Equal(t, task.Name, roundTripped.Name)
	assert.Equal(t, task.Needs, roundTripped.Needs)
	assert.Equal(t, task.Output, roundTripped.Output)
	assert.Equal(t, task.Script, roundTripped.Script)
	assert.Equal(t, task.Interpreter, roundTripped.Interpreter)
	assert.Equal(t, task.ParallelOutput, roundTripped.ParallelOutput)
	assert.Equal(t, task.Timeout, roundTripped.Timeout)
	assert.Equal(t, task.Steps, roundTripped.Steps)
	assert.Equal(t, task.MaxConcurrency, roundTripped.MaxConcurrency)
	assert.Equal(t, task.Matrix, roundTripped.Matrix)
	assert.Equal(t, task.Fail, roundTripped.Fail)
	assert.Equal(t, task.Viewport, roundTripped.Viewport)
	assert.Equal(t, task.Env, roundTripped.Env)
	assert.Equal(t, task.Vars, roundTripped.Vars)
	assert.Equal(t, task.Fields, roundTripped.Fields)
	assert.Equal(t, task.Data, roundTripped.Data)
	assert.Equal(t, task.Extensions, roundTripped.Extensions)
	assert.Equal(t, task.Columns, roundTripped.Columns)
	assert.Equal(t, task.Options, roundTripped.Options)
	assert.Equal(t, task.WorkingDirectory, roundTripped.WorkingDirectory)
}

func TestTaskFromWorkflowStepIgnoresInvalidTimeout(t *testing.T) {
	task := TaskFromWorkflowStep(&WorkflowStep{Timeout: "not-a-duration"})
	assert.Zero(t, task.Timeout)
}

func TestTasksDecodeHook_StructuredParallelOutput(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "checks",
				"type": TaskTypeParallel,
				"output": map[string]any{
					"mode":         "grouped",
					"order":        "definition",
					"show_summary": false,
					"prefix":       "{{ .step.name }}",
				},
				"steps": []any{
					map[string]any{"name": "test", "command": "make test"},
				},
			},
		},
	}

	var result testConfigWithTasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       TasksDecodeHook(),
	})
	require.NoError(t, err)

	require.NoError(t, decoder.Decode(input))
	require.Len(t, result.Steps, 1)
	assert.Equal(t, "grouped", result.Steps[0].Output)
	require.NotNil(t, result.Steps[0].ParallelOutput)
	assert.Equal(t, "definition", result.Steps[0].ParallelOutput.Order)
	require.NotNil(t, result.Steps[0].ParallelOutput.ShowSummary)
	assert.False(t, *result.Steps[0].ParallelOutput.ShowSummary)
	assert.Equal(t, "{{ .step.name }}", result.Steps[0].ParallelOutput.Prefix)
}

func TestTasksDecodeHook_StructuredCastOutputMode(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"name": "demo",
				"type": TaskTypeCast,
				"output": map[string]any{
					"mode": "raw",
					"cast": "demo.cast",
				},
				"steps": []any{
					map[string]any{"name": "list", "command": "atmos list stacks"},
				},
			},
		},
	}

	var result testConfigWithTasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       TasksDecodeHook(),
	})
	require.NoError(t, err)

	require.NoError(t, decoder.Decode(input))
	require.Len(t, result.Steps, 1)
	assert.Equal(t, "raw", result.Steps[0].Output)
	require.NotNil(t, result.Steps[0].CastOutput)
	assert.Equal(t, "raw", result.Steps[0].CastOutput.Mode)
	assert.Equal(t, "demo.cast", result.Steps[0].CastOutput.Cast)
}

func TestTasksDecodeHook_InvalidOutputType(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"name":   "checks",
				"type":   TaskTypeParallel,
				"output": []any{"grouped"},
			},
		},
	}

	var result testConfigWithTasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       TasksDecodeHook(),
	})
	require.NoError(t, err)

	require.Error(t, decoder.Decode(input))
}

// Tests for TasksDecodeHook and related functions.
// These tests use mapstructure.Decode with the TasksDecodeHook to test the hook behavior.

// testConfigWithTasks is a helper struct for testing TasksDecodeHook via mapstructure.
type testConfigWithTasks struct {
	Steps Tasks `mapstructure:"steps"`
}

func TestTasksDecodeHook_SimpleStrings(t *testing.T) {
	input := map[string]any{
		"steps": []any{"echo hello", "echo world"},
	}

	var result testConfigWithTasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       TasksDecodeHook(),
	})
	require.NoError(t, err)

	err = decoder.Decode(input)
	require.NoError(t, err)

	assert.Len(t, result.Steps, 2)
	assert.Equal(t, "echo hello", result.Steps[0].Command)
	assert.Equal(t, TaskTypeShell, result.Steps[0].Type)
	assert.Equal(t, "echo world", result.Steps[1].Command)
	assert.Equal(t, TaskTypeShell, result.Steps[1].Type)
}

func TestTasksDecodeHook_StructuredMaps(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"name":    "test",
				"command": "echo test",
				"type":    "atmos",
				"timeout": "30s",
			},
		},
	}

	var result testConfigWithTasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			TasksDecodeHook(),
		),
	})
	require.NoError(t, err)

	err = decoder.Decode(input)
	require.NoError(t, err)

	assert.Len(t, result.Steps, 1)
	assert.Equal(t, "test", result.Steps[0].Name)
	assert.Equal(t, "echo test", result.Steps[0].Command)
	assert.Equal(t, TaskTypeAtmos, result.Steps[0].Type)
	assert.Equal(t, 30*time.Second, result.Steps[0].Timeout)
}

func TestTasksDecodeHook_MixedSyntax(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			"echo simple",
			map[string]any{
				"name":    "structured",
				"command": "echo structured",
			},
		},
	}

	var result testConfigWithTasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       TasksDecodeHook(),
	})
	require.NoError(t, err)

	err = decoder.Decode(input)
	require.NoError(t, err)

	assert.Len(t, result.Steps, 2)
	assert.Equal(t, "echo simple", result.Steps[0].Command)
	assert.Equal(t, TaskTypeShell, result.Steps[0].Type)
	assert.Equal(t, "structured", result.Steps[1].Name)
	assert.Equal(t, "echo structured", result.Steps[1].Command)
	assert.Equal(t, TaskTypeShell, result.Steps[1].Type) // Defaults to shell.
}

func TestTasksDecodeHook_InvalidItemType(t *testing.T) {
	input := map[string]any{
		"steps": []any{123}, // Integer is not valid.
	}

	var result testConfigWithTasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       TasksDecodeHook(),
	})
	require.NoError(t, err)

	err = decoder.Decode(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected task node kind")
}

func TestDecodeTasksFromSlice_EmptySlice(t *testing.T) {
	tasks, err := decodeTasksFromSlice([]any{})
	require.NoError(t, err)
	assert.Len(t, tasks, 0)
}

func TestDecodeTasksFromSlice_StringItems(t *testing.T) {
	tasks, err := decodeTasksFromSlice([]any{"cmd1", "cmd2"})
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
	assert.Equal(t, "cmd1", tasks[0].Command)
	assert.Equal(t, "cmd2", tasks[1].Command)
}

func TestDecodeTasksFromSlice_MapItems(t *testing.T) {
	tasks, err := decodeTasksFromSlice([]any{
		map[string]any{"command": "test", "type": "atmos"},
	})
	require.NoError(t, err)
	assert.Len(t, tasks, 1)
	assert.Equal(t, "test", tasks[0].Command)
	assert.Equal(t, TaskTypeAtmos, tasks[0].Type)
}

func TestDecodeTasksFromSlice_ErrorPropagation(t *testing.T) {
	// Test error propagation from decodeTaskItem.
	_, err := decodeTasksFromSlice([]any{3.14}) // Float is not valid.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTaskUnexpectedNodeKind)
}

func TestDecodeTaskItem_String(t *testing.T) {
	task, err := decodeTaskItem("echo hello", 0)
	require.NoError(t, err)
	assert.Equal(t, "echo hello", task.Command)
	assert.Equal(t, TaskTypeShell, task.Type)
}

func TestDecodeTaskItem_Map(t *testing.T) {
	task, err := decodeTaskItem(map[string]any{
		"name":    "test",
		"command": "do something",
		"timeout": "1m",
	}, 0)
	require.NoError(t, err)
	assert.Equal(t, "test", task.Name)
	assert.Equal(t, "do something", task.Command)
	assert.Equal(t, time.Minute, task.Timeout)
	assert.Equal(t, TaskTypeShell, task.Type) // Default.
}

func TestDecodeTaskItem_MapWithType(t *testing.T) {
	task, err := decodeTaskItem(map[string]any{
		"command": "terraform plan",
		"type":    "atmos",
	}, 0)
	require.NoError(t, err)
	assert.Equal(t, TaskTypeAtmos, task.Type)
}

func TestDecodeTaskItem_InvalidType(t *testing.T) {
	// Test with invalid type (not string or map).
	_, err := decodeTaskItem(42, 5)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTaskUnexpectedNodeKind)
	assert.Contains(t, err.Error(), "at index 5")
	assert.Contains(t, err.Error(), "int")
}

func TestDecodeTaskFromMap_ValidMap(t *testing.T) {
	m := map[string]any{
		"name":              "deploy",
		"command":           "terraform apply",
		"type":              "atmos",
		"stack":             "prod",
		"working_directory": "/app",
		"identity":          "admin",
		"timeout":           "5m",
	}

	task, err := decodeTaskFromMap(m, 0)
	require.NoError(t, err)
	assert.Equal(t, "deploy", task.Name)
	assert.Equal(t, "terraform apply", task.Command)
	assert.Equal(t, TaskTypeAtmos, task.Type)
	assert.Equal(t, "prod", task.Stack)
	assert.Equal(t, "/app", task.WorkingDirectory)
	assert.Equal(t, "admin", task.Identity)
	assert.Equal(t, 5*time.Minute, task.Timeout)
}

func TestDecodeTaskFromMap_DefaultsTypeToShell(t *testing.T) {
	m := map[string]any{
		"command": "echo hello",
	}

	task, err := decodeTaskFromMap(m, 0)
	require.NoError(t, err)
	assert.Equal(t, TaskTypeShell, task.Type)
}

func TestDecodeTaskFromMap_WithRetry(t *testing.T) {
	m := map[string]any{
		"command": "curl http://example.com",
		"retry": map[string]any{
			"max_attempts":  3,
			"initial_delay": "1s",
			"max_delay":     "10s",
		},
	}

	task, err := decodeTaskFromMap(m, 0)
	require.NoError(t, err)
	require.NotNil(t, task.Retry)
	require.NotNil(t, task.Retry.MaxAttempts)
	assert.Equal(t, 3, *task.Retry.MaxAttempts)
	require.NotNil(t, task.Retry.InitialDelay)
	assert.Equal(t, time.Second, *task.Retry.InitialDelay)
	require.NotNil(t, task.Retry.MaxDelay)
	assert.Equal(t, 10*time.Second, *task.Retry.MaxDelay)
}

func TestDecodeTaskFromMap_InvalidTimeout(t *testing.T) {
	m := map[string]any{
		"command": "echo hello",
		"timeout": "not-a-duration",
	}

	_, err := decodeTaskFromMap(m, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode task at index 2")
}

func TestDecodeTaskFromMap_InvalidStructuredOutput(t *testing.T) {
	m := map[string]any{
		"command": "echo hello",
		"output": map[string]any{
			"mode":         "grouped",
			"show_summary": []any{"not-a-bool"},
		},
	}

	_, err := decodeTaskFromMap(m, 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode task output at index 3")
}

func TestDecodeTaskFromMap_EmptyMap(t *testing.T) {
	m := map[string]any{}

	task, err := decodeTaskFromMap(m, 0)
	require.NoError(t, err)
	// Empty command is allowed, defaults to shell type.
	assert.Equal(t, "", task.Command)
	assert.Equal(t, TaskTypeShell, task.Type)
}
