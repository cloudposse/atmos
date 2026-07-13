package schema

import (
	"errors"
	"reflect"
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

func TestTasksDecodeHook_StructuredSimulatePrompt(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"type":   TaskTypeSimulate,
				"mode":   "typed",
				"cursor": true,
				"jitter": 0.25,
				"prompt": map[string]any{
					"text":  "> ",
					"style": "command",
				},
				"text": "atmos version",
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
	require.NotNil(t, result.Steps[0].SimulatePrompt)
	assert.Equal(t, "> ", result.Steps[0].SimulatePrompt.Text)
	assert.Equal(t, "command", result.Steps[0].SimulatePrompt.Style)
	assert.True(t, result.Steps[0].Cursor)
	assert.Equal(t, 0.25, result.Steps[0].Jitter)
	assert.Equal(t, "atmos version", result.Steps[0].Text)
}

func TestTasksDecodeHook_CastSimulateDefaults(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"type": TaskTypeCast,
				"defaults": map[string]any{
					"cast": map[string]any{
						"rate":   "12ms",
						"width":  120,
						"height": 36,
					},
					"simulate": map[string]any{
						"mode":   "typed",
						"cursor": true,
						"rate":   "35ms",
						"prompt": map[string]any{
							"text":  "> ",
							"style": "command",
						},
					},
				},
				"steps": []any{
					map[string]any{
						"type":   TaskTypeSimulate,
						"cursor": false,
						"text":   "atmos version",
					},
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
	defaults := result.Steps[0].Defaults
	require.NotNil(t, defaults)
	require.NotNil(t, defaults.Cast)
	assert.Equal(t, "12ms", defaults.Cast.Rate)
	assert.Equal(t, 120, defaults.Cast.Width)
	assert.Equal(t, 36, defaults.Cast.Height)
	require.NotNil(t, defaults.Simulate)
	require.NotNil(t, defaults.Simulate.Cursor)
	assert.True(t, *defaults.Simulate.Cursor)
	assert.Equal(t, "35ms", defaults.Simulate.Rate)
	require.NotNil(t, defaults.Simulate.Prompt)
	assert.Equal(t, "> ", defaults.Simulate.Prompt.Text)
	assert.Equal(t, "command", defaults.Simulate.Prompt.Style)
	require.Len(t, result.Steps[0].Steps, 1)
	assert.False(t, result.Steps[0].Steps[0].Cursor)
	assert.True(t, result.Steps[0].Steps[0].CursorSet)
}

func TestTasksDecodeHook_NestedCastSimulatePrompt(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"type": TaskTypeCast,
				"mode": "steps",
				"steps": []any{
					map[string]any{
						"type": TaskTypeSimulate,
						"mode": "typed",
						"prompt": map[string]any{
							"text":  "> ",
							"style": "command",
						},
						"text": "atmos secret list --stack dev --component api",
					},
					map[string]any{
						"type":    TaskTypeShell,
						"command": "atmos secret list --stack dev --component api",
					},
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
	require.Len(t, result.Steps[0].Steps, 2)
	require.NotNil(t, result.Steps[0].Steps[0].SimulatePrompt)
	assert.Equal(t, "> ", result.Steps[0].Steps[0].SimulatePrompt.Text)
	assert.Equal(t, "command", result.Steps[0].Steps[0].SimulatePrompt.Style)
	assert.Equal(t, "atmos secret list --stack dev --component api", result.Steps[0].Steps[1].Command)
}

func TestTasksDecodeHook_NestedCastSimulatePromptFromTypedSlice(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"type": TaskTypeCast,
				"mode": "steps",
				"steps": []map[string]any{
					{
						"type": TaskTypeSimulate,
						"mode": "typed",
						"prompt": map[string]any{
							"text":  "> ",
							"style": "command",
						},
						"text": "atmos secret list --stack dev --component api",
					},
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
	require.Len(t, result.Steps[0].Steps, 1)
	require.NotNil(t, result.Steps[0].Steps[0].SimulatePrompt)
	assert.Equal(t, "> ", result.Steps[0].Steps[0].SimulatePrompt.Text)
	assert.Equal(t, "command", result.Steps[0].Steps[0].SimulatePrompt.Style)
}

func TestTasksDecodeHook_NestedCastSimulatePromptFromAnyMap(t *testing.T) {
	input := map[string]any{
		"steps": []any{
			map[string]any{
				"type": TaskTypeCast,
				"mode": "steps",
				"steps": []any{
					map[any]any{
						"type": TaskTypeSimulate,
						"mode": "typed",
						"prompt": map[string]any{
							"text":  "> ",
							"style": "command",
						},
						"text": "atmos secret list --stack dev --component api",
					},
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
	require.Len(t, result.Steps[0].Steps, 1)
	require.NotNil(t, result.Steps[0].Steps[0].SimulatePrompt)
	assert.Equal(t, "> ", result.Steps[0].Steps[0].SimulatePrompt.Text)
	assert.Equal(t, "command", result.Steps[0].Steps[0].SimulatePrompt.Style)
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

func TestTasksDecodeHook_TypedRootSlice(t *testing.T) {
	input := map[string]any{
		"steps": []map[string]any{
			{
				"name":    "typed",
				"command": "echo typed",
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
	assert.Equal(t, "typed", result.Steps[0].Name)
	assert.Equal(t, "echo typed", result.Steps[0].Command)
	assert.Equal(t, TaskTypeShell, result.Steps[0].Type)
}

func TestWorkflowStepDecodeHookRejectsStructuredPromptForShellStep(t *testing.T) {
	input := map[string]any{
		"step": map[string]any{
			"type": TaskTypeShell,
			"prompt": map[string]any{
				"text":  "> ",
				"style": "command",
			},
			"command": "echo no",
		},
	}

	var result struct {
		Step WorkflowStep `mapstructure:"step"`
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       WorkflowStepDecodeHook(),
	})
	require.NoError(t, err)

	err = decoder.Decode(input)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkflowControlStepInvalid)
}

func TestWorkflowStepDecodeHookNormalizesNestedTypedSlices(t *testing.T) {
	input := map[string]any{
		"steps": []map[string]any{
			{
				"type": TaskTypeCast,
				"mode": "steps",
				"steps": []map[string]any{
					{
						"type": TaskTypeSimulate,
						"mode": "typed",
						"prompt": map[string]any{
							"text":  "$ ",
							"style": "command",
						},
						"text": "atmos version",
					},
				},
			},
		},
	}

	var result struct {
		Steps []WorkflowStep `mapstructure:"steps"`
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       WorkflowStepDecodeHook(),
	})
	require.NoError(t, err)

	require.NoError(t, decoder.Decode(input))
	require.Len(t, result.Steps, 1)
	require.Len(t, result.Steps[0].Steps, 1)
	require.NotNil(t, result.Steps[0].Steps[0].SimulatePrompt)
	assert.Equal(t, "$ ", result.Steps[0].Steps[0].SimulatePrompt.Text)
	assert.Equal(t, "command", result.Steps[0].Steps[0].SimulatePrompt.Style)
	assert.Equal(t, "atmos version", result.Steps[0].Steps[0].Text)
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

// TestDecodeTaskFromMap_InvalidStructuredPrompt verifies decodeTaskFromMap wraps a
// normalizeTaskPromptMap error (structured prompt on a non-simulate type) with the
// "failed to decode task prompt at index N" context and the ErrWorkflowControlStepInvalid
// sentinel.
func TestDecodeTaskFromMap_InvalidStructuredPrompt(t *testing.T) {
	m := map[string]any{
		"type":    TaskTypeShell,
		"command": "echo hello",
		"prompt": map[string]any{
			"text": "> ",
		},
	}

	_, err := decodeTaskFromMap(m, 4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode task prompt at index 4")
	assert.True(t, errors.Is(err, ErrWorkflowControlStepInvalid))
}

// TestDecodeTaskFromMap_InvalidStepsMap verifies decodeTaskFromMap wraps a
// normalizeTaskStepsMap error (propagated from a nested step's structured prompt
// mismatch) with the "failed to decode task steps at index N" context.
func TestDecodeTaskFromMap_InvalidStepsMap(t *testing.T) {
	m := map[string]any{
		"type": TaskTypeParallel,
		"steps": []any{
			map[string]any{
				"type": TaskTypeShell,
				"prompt": map[string]any{
					"text": "> ",
				},
			},
		},
	}

	_, err := decodeTaskFromMap(m, 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode task steps at index 5")
	assert.True(t, errors.Is(err, ErrWorkflowControlStepInvalid))
}

func TestDecodeTaskFromMap_EmptyMap(t *testing.T) {
	m := map[string]any{}

	task, err := decodeTaskFromMap(m, 0)
	require.NoError(t, err)
	// Empty command is allowed, defaults to shell type.
	assert.Equal(t, "", task.Command)
	assert.Equal(t, TaskTypeShell, task.Type)
}

// TestTasksDecodeHook_IgnoresNonTasksTarget verifies the hook's early-out guards:
// it must not touch data unless converting to the Tasks type from a slice.
func TestTasksDecodeHook_IgnoresNonTasksTarget(t *testing.T) {
	hook := TasksDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))

	// Wrong target type: passthrough regardless of source kind.
	out, err := hook(reflect.TypeOf([]any{}), reflect.TypeOf(""), []any{"echo hi"})
	require.NoError(t, err)
	assert.Equal(t, []any{"echo hi"}, out)

	// Correct target type but source is not a slice: passthrough.
	out, err = hook(reflect.TypeOf(""), reflect.TypeOf(Tasks{}), "not-a-slice")
	require.NoError(t, err)
	assert.Equal(t, "not-a-slice", out)
}

// TestWorkflowStepDecodeHook_IgnoresWrongSourceKind verifies each of the hook's
// per-target Kind guards return data unchanged when the source Kind doesn't match.
func TestWorkflowStepDecodeHook_IgnoresWrongSourceKind(t *testing.T) {
	hook := WorkflowStepDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))
	stepType := reflect.TypeOf(WorkflowStep{})
	stepsType := reflect.TypeOf([]WorkflowStep{})

	// Target is WorkflowStep but source is not a map.
	out, err := hook(reflect.TypeOf(""), stepType, "not-a-map")
	require.NoError(t, err)
	assert.Equal(t, "not-a-map", out)

	// Target is WorkflowStep, source is a map, but not stringifiable (e.g. map[int]any).
	badMap := map[int]any{1: "x"}
	out, err = hook(reflect.TypeOf(badMap), stepType, badMap)
	require.NoError(t, err)
	assert.Equal(t, badMap, out)

	// Target is []WorkflowStep but source is not a slice.
	out, err = hook(reflect.TypeOf(""), stepsType, "not-a-slice")
	require.NoError(t, err)
	assert.Equal(t, "not-a-slice", out)

	// Target is neither WorkflowStep nor []WorkflowStep: passthrough.
	out, err = hook(reflect.TypeOf(""), reflect.TypeOf(0), "anything")
	require.NoError(t, err)
	assert.Equal(t, "anything", out)
}

// TestWorkflowStepDecodeHook_AcceptsMapAnyAnyStep verifies the map[any]any branch of
// stringifyTaskMap is exercised via the hook for a single WorkflowStep target.
func TestWorkflowStepDecodeHook_AcceptsMapAnyAnyStep(t *testing.T) {
	hook := WorkflowStepDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))
	stepType := reflect.TypeOf(WorkflowStep{})

	data := map[any]any{"name": "typed", "command": "echo hi"}
	out, err := hook(reflect.TypeOf(data), stepType, data)
	require.NoError(t, err)

	normalized, ok := out.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "typed", normalized["name"])
	assert.Equal(t, "echo hi", normalized["command"])
}

// TestSliceToAny_RejectsNonSlice verifies the reflect fallback path returns false for
// non-slice, non-[]any inputs.
func TestSliceToAny_RejectsNonSlice(t *testing.T) {
	_, ok := sliceToAny("not-a-slice")
	assert.False(t, ok)

	_, ok = sliceToAny(nil)
	assert.False(t, ok)
}

// TestSliceToAny_ConvertsTypedSlice verifies the reflect-based conversion path (used
// when mapstructure hands over a concretely-typed slice like []map[string]any instead
// of []any).
func TestSliceToAny_ConvertsTypedSlice(t *testing.T) {
	typed := []map[string]any{{"command": "echo a"}, {"command": "echo b"}}
	slice, ok := sliceToAny(typed)
	require.True(t, ok)
	require.Len(t, slice, 2)
	assert.Equal(t, typed[0], slice[0])
	assert.Equal(t, typed[1], slice[1])
}

// TestDecodeTaskItem_MapAnyAny verifies the default branch of decodeTaskItem that
// stringifies a map[any]any item before decoding it as a task map.
func TestDecodeTaskItem_MapAnyAny(t *testing.T) {
	task, err := decodeTaskItem(map[any]any{"command": "echo hi", "type": "atmos"}, 0)
	require.NoError(t, err)
	assert.Equal(t, "echo hi", task.Command)
	assert.Equal(t, TaskTypeAtmos, task.Type)
}

// TestNormalizeWorkflowStepMaps_NonSlice verifies the reflect Kind guard returns the
// original value unchanged (with a nil error) for non-slice input.
func TestNormalizeWorkflowStepMaps_NonSlice(t *testing.T) {
	out, err := normalizeWorkflowStepMaps("not-a-slice")
	require.NoError(t, err)
	assert.Equal(t, "not-a-slice", out)

	out, err = normalizeWorkflowStepMaps(nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

// TestNormalizeWorkflowStepMaps_SkipsNonMapItems verifies items that cannot be
// stringified as a map pass through unchanged in the normalized slice.
func TestNormalizeWorkflowStepMaps_SkipsNonMapItems(t *testing.T) {
	out, err := normalizeWorkflowStepMaps([]any{"echo hi", 42})
	require.NoError(t, err)
	normalized, ok := out.([]any)
	require.True(t, ok)
	require.Len(t, normalized, 2)
	assert.Equal(t, "echo hi", normalized[0])
	assert.Equal(t, 42, normalized[1])
}

// TestNormalizeWorkflowStepMaps_PropagatesNestedError verifies an error from decoding
// a nested step map (structured prompt on a non-simulate type) propagates up through
// the per-item loop.
func TestNormalizeWorkflowStepMaps_PropagatesNestedError(t *testing.T) {
	_, err := normalizeWorkflowStepMaps([]any{
		map[string]any{
			"type": TaskTypeShell,
			"prompt": map[string]any{
				"text": "> ",
			},
		},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkflowControlStepInvalid))
}

// TestNormalizeTaskStepsMap_PropagatesNestedError verifies normalizeTaskStepsMap
// surfaces an error from normalizing a nested step's structured prompt.
func TestNormalizeTaskStepsMap_PropagatesNestedError(t *testing.T) {
	m := map[string]any{
		"steps": []any{
			map[string]any{
				"type": TaskTypeShell,
				"prompt": map[string]any{
					"text": "> ",
				},
			},
		},
	}
	_, err := normalizeTaskStepsMap(m)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkflowControlStepInvalid))
}

// TestNormalizeWorkflowStepMap_PromptDefaultBreaksOnNonStringifiableValue verifies the
// `default` case of the prompt-kind switch: a prompt value that is neither a string
// nor a stringifiable map (e.g. an int) is left untouched (break, not error).
func TestNormalizeWorkflowStepMap_PromptDefaultBreaksOnNonStringifiableValue(t *testing.T) {
	m := map[string]any{
		"type":   TaskTypeShell,
		"prompt": 42,
	}
	out, err := normalizeWorkflowStepMap(m)
	require.NoError(t, err)
	assert.Equal(t, 42, out["prompt"])
}

// TestNormalizeTaskPromptMap_ScalarStringPassthrough verifies a scalar string prompt
// value is returned unchanged (the "case string" branch).
func TestNormalizeTaskPromptMap_ScalarStringPassthrough(t *testing.T) {
	var task Task
	m := map[string]any{"prompt": "Continue?"}
	out, err := normalizeTaskPromptMap(m, &task)
	require.NoError(t, err)
	assert.Equal(t, "Continue?", out["prompt"])
	assert.Nil(t, task.SimulatePrompt)
}

// TestNormalizeTaskPromptMap_NonStringifiableValuePassesThrough verifies a prompt
// value that stringifyTaskMap rejects (e.g. an int) returns the map unchanged.
func TestNormalizeTaskPromptMap_NonStringifiableValuePassesThrough(t *testing.T) {
	var task Task
	m := map[string]any{"prompt": 42}
	out, err := normalizeTaskPromptMap(m, &task)
	require.NoError(t, err)
	assert.Equal(t, 42, out["prompt"])
	assert.Nil(t, task.SimulatePrompt)
}

// TestNormalizeTaskPromptMap_RejectsStructuredPromptOnNonSimulateType verifies the
// structured-prompt/non-simulate-type mismatch error.
func TestNormalizeTaskPromptMap_RejectsStructuredPromptOnNonSimulateType(t *testing.T) {
	var task Task
	m := map[string]any{
		"type":   TaskTypeShell,
		"prompt": map[string]any{"text": "> "},
	}
	_, err := normalizeTaskPromptMap(m, &task)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkflowControlStepInvalid))
}

// TestNormalizeTaskPromptMap_DecodeErrorPropagates verifies a decode failure while
// building the SimulatePrompt (wrong field type) surfaces as an error.
func TestNormalizeTaskPromptMap_DecodeErrorPropagates(t *testing.T) {
	var task Task
	m := map[string]any{
		"type": TaskTypeSimulate,
		"prompt": map[string]any{
			"text": []string{"not", "a", "string"},
		},
	}
	_, err := normalizeTaskPromptMap(m, &task)
	require.Error(t, err)
}

// TestNormalizeTaskOutputMap_ScalarStringPassthrough verifies a scalar string output
// value is returned unchanged (the "case string" branch).
func TestNormalizeTaskOutputMap_ScalarStringPassthrough(t *testing.T) {
	var task Task
	m := map[string]any{"output": "raw"}
	out, err := normalizeTaskOutputMap(m, &task)
	require.NoError(t, err)
	assert.Equal(t, "raw", out["output"])
	assert.Nil(t, task.CastOutput)
	assert.Nil(t, task.ParallelOutput)
}

// TestNormalizeTaskOutputMap_CastDecodeErrorPropagates verifies a decode failure while
// building the CastOutput (wrong field type) surfaces as an error.
func TestNormalizeTaskOutputMap_CastDecodeErrorPropagates(t *testing.T) {
	var task Task
	m := map[string]any{
		"type": TaskTypeCast,
		"output": map[string]any{
			"mode": []string{"not", "a", "string"},
		},
	}
	_, err := normalizeTaskOutputMap(m, &task)
	require.Error(t, err)
}

// TestNormalizeTaskOutputMap_ParallelOutputDecodeErrorPropagates verifies a decode
// failure while building the ParallelOutputConfig (wrong field type) surfaces as an
// error for non-cast step types.
func TestNormalizeTaskOutputMap_ParallelOutputDecodeErrorPropagates(t *testing.T) {
	var task Task
	m := map[string]any{
		"type": TaskTypeParallel,
		"output": map[string]any{
			"mode": []string{"not", "a", "string"},
		},
	}
	_, err := normalizeTaskOutputMap(m, &task)
	require.Error(t, err)
}
