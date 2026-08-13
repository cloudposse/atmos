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
	export := false
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
		Continue:         MustCondition(ConditionPredicateAlways),
		Retry: &RetryConfig{
			MaxAttempts: &maxAttempts,
		},
		Timeout: 30 * time.Second,
		Export:  &export,
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
	forgiven, err := step.Continue.EvaluateContinueE(ConditionContext{Status: ConditionPredicateFailure})
	require.NoError(t, err)
	assert.True(t, forgiven, "Continue must survive Task -> WorkflowStep conversion")
	assert.Equal(t, task.Retry, step.Retry)
	assert.Same(t, task.Export, step.Export)
	// Note: Timeout is not in WorkflowStep.
}

func TestTaskFromWorkflowStep(t *testing.T) {
	maxAttempts := 5
	export := false
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
		Continue:         MustCondition(ConditionPredicateAlways),
		Retry: &RetryConfig{
			MaxAttempts: &maxAttempts,
		},
		Export: &export,
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
	forgiven, err := task.Continue.EvaluateContinueE(ConditionContext{Status: ConditionPredicateFailure})
	require.NoError(t, err)
	assert.True(t, forgiven, "Continue must survive WorkflowStep -> Task conversion")
	assert.Equal(t, step.Retry, task.Retry)
	assert.Same(t, step.Export, task.Export)
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

// TestTask_InputsArtifactsPreconditionsRoundTrip exercises every combination of the three
// freshness sibling fields (Inputs/Artifacts/Preconditions) through both conversion directions,
// since Inputs is a general container (not narrowed to sources only) and Artifacts/Preconditions
// are deliberately separate sibling fields, not nested inside Inputs.
func TestTask_InputsArtifactsPreconditionsRoundTrip(t *testing.T) {
	cases := []struct {
		name          string
		inputs        *Inputs
		artifacts     *Artifacts
		preconditions *Preconditions
	}{
		{name: "none"},
		{name: "inputs only", inputs: &Inputs{Sources: []string{"*.go"}}},
		{name: "artifacts only", artifacts: &Artifacts{Paths: []string{"bin/app"}}},
		{name: "preconditions only", preconditions: &Preconditions{Tools: []string{"stringer"}}},
		{
			name:      "inputs and artifacts",
			inputs:    &Inputs{Sources: []string{"*.go", "go.sum"}},
			artifacts: &Artifacts{Paths: []string{"bin/app"}},
		},
		{
			name:          "all three",
			inputs:        &Inputs{Sources: []string{"*.go"}},
			artifacts:     &Artifacts{Paths: []string{"bin/app"}},
			preconditions: &Preconditions{Tools: []string{"stringer", "protoc"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+" Task->WorkflowStep", func(t *testing.T) {
			task := Task{Name: "t", Inputs: tc.inputs, Artifacts: tc.artifacts, Preconditions: tc.preconditions}
			step := task.ToWorkflowStep()
			assert.Equal(t, tc.inputs, step.Inputs)
			assert.Equal(t, tc.artifacts, step.Artifacts)
			assert.Equal(t, tc.preconditions, step.Preconditions)
		})

		t.Run(tc.name+" WorkflowStep->Task", func(t *testing.T) {
			step := WorkflowStep{Name: "s", Inputs: tc.inputs, Artifacts: tc.artifacts, Preconditions: tc.preconditions}
			task := TaskFromWorkflowStep(&step)
			assert.Equal(t, tc.inputs, task.Inputs)
			assert.Equal(t, tc.artifacts, task.Artifacts)
			assert.Equal(t, tc.preconditions, task.Preconditions)
		})
	}
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

// TestDecodeTaskFromMap_InvalidWithBlock verifies decodeTaskFromMap wraps a
// decodeStepWithFromMapValue error (an unsupported container `action` rejecting
// a non-empty `with:` block, mirroring decodeContainerWith's default case in
// workflow.go) with the "failed to decode task with-block at index N" context
// and the ErrWorkflowControlStepInvalid sentinel.
func TestDecodeTaskFromMap_InvalidWithBlock(t *testing.T) {
	m := map[string]any{
		"type":   TaskTypeShell,
		"action": "not-a-real-action",
		"with": map[string]any{
			"context": "app",
		},
	}

	_, err := decodeTaskFromMap(m, 6)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode task with-block at index 6")
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

// containerStepWithBlockYAML is a single `type: container, action: build`
// step with a full `with:` block, in the exact shape a user writes it in
// either a workflow file or a commands.yaml custom command.
const containerStepWithBlockYAML = `
- name: build
  type: container
  action: build
  provider: docker
  with:
    engine: buildx
    context: app
    dockerfile: Dockerfile
    tags:
      - "example.invalid/demo:sha-test"
    driver:
      name: atmos-native-ci
      provider: docker-container
    cache:
      from:
        - type: registry
          ref: "example.invalid/demo:buildcache"
      to:
        - type: registry
          ref: "example.invalid/demo:buildcache"
          mode: max
`

// TestContainerStepWithBlock_WorkflowAndCustomCommandDecodeIdentically confirms
// the public contract cited by https://github.com/cloudposse/atmos/issues/2876:
// a `type: container` step's `with:` block must decode into the same Build
// struct whether it's parsed as a standalone workflow file (direct
// yaml.Node.Decode -> Task.UnmarshalYAML) or as a commands.yaml custom
// command merged into Viper's config tree (mapstructure -> TasksDecodeHook ->
// decodeTaskFromMap). Both call paths are exercised here exactly as their
// real callers do: yaml.Unmarshal for the workflow-file path (see
// pkg/utils.UnmarshalYAMLFromFile, used to load workflows/*.yaml), and
// mapstructure.NewDecoder with TasksDecodeHook for the Viper path (see
// pkg/config's atmosDecodeHook, used to decode atmos.yaml's `commands:`).
func TestContainerStepWithBlock_WorkflowAndCustomCommandDecodeIdentically(t *testing.T) {
	// Workflow-file path: direct YAML decode, invoking Task.UnmarshalYAML.
	var fromYAML Tasks
	require.NoError(t, yaml.Unmarshal([]byte(containerStepWithBlockYAML), &fromYAML))
	require.Len(t, fromYAML, 1)

	// Custom-command / Viper path: decode into a generic tree first (as Viper
	// does when it reads the YAML file), then mapstructure-decode that tree
	// into Tasks via the real TasksDecodeHook, mirroring atmosDecodeHook.
	var generic []any
	require.NoError(t, yaml.Unmarshal([]byte(containerStepWithBlockYAML), &generic))

	var fromMapstructure Tasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &fromMapstructure,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			ConditionDecodeHook(),
			WorkflowStepDecodeHook(),
			TasksDecodeHook(),
		),
	})
	require.NoError(t, err)
	require.NoError(t, decoder.Decode(generic))
	require.Len(t, fromMapstructure, 1)

	workflowStep := fromYAML[0]
	commandStep := fromMapstructure[0]

	require.NotNil(t, workflowStep.Build, "workflow-file path must decode with: into Build")
	require.NotNil(t, commandStep.Build, "custom-command path must decode with: into Build")
	assert.Equal(t, workflowStep.Build, commandStep.Build,
		"a type: container step's with: block must decode identically for workflow files and custom commands")
	assert.Nil(t, workflowStep.With, "with: must not also leak into the generic With map for a container step")
	assert.Nil(t, commandStep.With, "with: must not also leak into the generic With map for a container step")
}

// containerStepWithUnknownFieldYAML is the same shape as
// containerStepWithBlockYAML but with a plausible-sounding, nonexistent
// field (`platforms:` -- not a field on ContainerBuildStep) mixed into the
// `with:` block, the way a user coming from Docker Compose might type it.
const containerStepWithUnknownFieldYAML = `
- name: build
  type: container
  action: build
  provider: docker
  with:
    context: app
    dockerfile: Dockerfile
    platforms:
      - linux/amd64
      - linux/arm64
`

// TestContainerStepWithBlock_RejectsUnknownField confirms a typo'd/nonexistent
// `with:` field (e.g. `platforms:`, which sounds plausible but isn't a
// ContainerBuildStep field) is rejected rather than silently dropped, for
// both the workflow-file path (yaml.Unmarshal) and the custom-command path
// (mapstructure + TasksDecodeHook) -- the same two paths
// TestContainerStepWithBlock_WorkflowAndCustomCommandDecodeIdentically
// exercises for the happy path. Before this fix, decodeYAMLInto used plain
// yaml.Node.Decode, which has no KnownFields/strict mode, so `platforms:`
// was silently discarded with no error and no trace in the decoded struct.
func TestContainerStepWithBlock_RejectsUnknownField(t *testing.T) {
	t.Run("workflow file path", func(t *testing.T) {
		var fromYAML Tasks
		err := yaml.Unmarshal([]byte(containerStepWithUnknownFieldYAML), &fromYAML)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "platforms")
	})

	t.Run("custom command path", func(t *testing.T) {
		var generic []any
		require.NoError(t, yaml.Unmarshal([]byte(containerStepWithUnknownFieldYAML), &generic))

		var fromMapstructure Tasks
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:           &fromMapstructure,
			TagName:          "mapstructure",
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				ConditionDecodeHook(),
				WorkflowStepDecodeHook(),
				TasksDecodeHook(),
			),
		})
		require.NoError(t, err)
		err = decoder.Decode(generic)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "platforms")
	})
}

// containerStepDriverUnknownFieldYAML nests the typo inside the `driver:`
// mapping (a field with its own custom UnmarshalYAML, ContainerDriverConfig),
// not the top-level `with:` mapping -- a distinct code path from the
// top-level case above since ContainerDriverConfig.UnmarshalYAML runs its
// own nested decode.
const containerStepDriverUnknownFieldYAML = `
- name: build
  type: container
  action: build
  with:
    context: app
    driver:
      name: atmos-native-ci
      bogus_field: x
`

// TestContainerStepWithBlock_RejectsUnknownNestedDriverField confirms the
// same unknown-field rejection reaches a typo inside a nested `driver:`
// mapping, not just the top-level `with:` mapping.
func TestContainerStepWithBlock_RejectsUnknownNestedDriverField(t *testing.T) {
	var fromYAML Tasks
	err := yaml.Unmarshal([]byte(containerStepDriverUnknownFieldYAML), &fromYAML)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus_field")
}

// containerStepOverrideYAML has two ordinary `type: shell` (default) steps:
// one with a mapping-form step-level `container:` override, and one that
// opts out of an ambient container sandbox with the bare boolean form
// `container: false`. Both forms are handled by WorkflowContainer.UnmarshalYAML
// (workflow.go) for the workflow-file path; decodeTaskFromMap must reproduce
// the exact same result for the custom-command/mapstructure path.
const containerStepOverrideYAML = `
- name: run-in-sandbox
  command: echo hello
  container:
    image: alpine
    provider: docker
    workspace: /workspace
- name: run-on-host
  command: echo skip
  container: false
`

// TestContainerStepOverride_WorkflowAndCustomCommandDecodeIdentically confirms
// a step's `container:` override (mapping config or bare boolean opt-out)
// decodes identically whether parsed as a standalone workflow file (direct
// yaml.Node.Decode -> Task.UnmarshalYAML -> WorkflowContainer.UnmarshalYAML)
// or as a commands.yaml custom command merged into Viper's config tree
// (mapstructure -> TasksDecodeHook -> decodeTaskFromMap). Before the fix,
// the mapstructure path either failed the boolean form outright ("'container'
// expected a map or struct, got bool") -- breaking the ENTIRE atmos.yaml load,
// not just this one step/command -- or, for the mapping form, decoded without
// error but left Container fields un-typed relative to the workflow path.
func TestContainerStepOverride_WorkflowAndCustomCommandDecodeIdentically(t *testing.T) {
	// Workflow-file path: direct YAML decode, invoking Task.UnmarshalYAML.
	var fromYAML Tasks
	require.NoError(t, yaml.Unmarshal([]byte(containerStepOverrideYAML), &fromYAML))
	require.Len(t, fromYAML, 2)

	// Custom-command / Viper path: decode into a generic tree first (as Viper
	// does when it reads the YAML file), then mapstructure-decode that tree
	// into Tasks via the real TasksDecodeHook, mirroring atmosDecodeHook.
	var generic []any
	require.NoError(t, yaml.Unmarshal([]byte(containerStepOverrideYAML), &generic))

	var fromMapstructure Tasks
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &fromMapstructure,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			ConditionDecodeHook(),
			WorkflowStepDecodeHook(),
			TasksDecodeHook(),
		),
	})
	require.NoError(t, err)
	// Before the fix, this failed outright for the boolean-opt-out step:
	// mapstructure has no notion of WorkflowContainer.UnmarshalYAML and
	// rejects a bare `false` for a struct-typed field.
	require.NoError(t, decoder.Decode(generic))
	require.Len(t, fromMapstructure, 2)

	// Mapping-form container override.
	workflowOverride := fromYAML[0]
	commandOverride := fromMapstructure[0]
	require.NotNil(t, workflowOverride.Container, "workflow-file path must decode container: into a WorkflowContainer")
	require.NotNil(t, commandOverride.Container, "custom-command path must decode container: into a WorkflowContainer")
	assert.Equal(t, workflowOverride.Container, commandOverride.Container,
		"a step's mapping-form container: block must decode identically for workflow files and custom commands")
	assert.Equal(t, "alpine", commandOverride.Container.Image)
	assert.True(t, commandOverride.Container.IsEnabled())

	// Boolean-false opt-out form.
	workflowDisabled := fromYAML[1]
	commandDisabled := fromMapstructure[1]
	require.NotNil(t, workflowDisabled.Container)
	require.NotNil(t, commandDisabled.Container)
	assert.False(t, workflowDisabled.Container.IsEnabled())
	assert.False(t, commandDisabled.Container.IsEnabled())
	assert.Equal(t, workflowDisabled.Container, commandDisabled.Container,
		"a step's boolean container: false opt-out must decode identically for workflow files and custom commands")
}

// containerStepOverrideUnknownFieldYAML mirrors containerStepOverrideYAML's
// mapping-form override but with a typo'd field (`imgae` instead of `image`)
// that WorkflowContainer has no field for.
const containerStepOverrideUnknownFieldYAML = `
- name: run-in-sandbox
  command: echo hello
  container:
    imgae: alpine
    provider: docker
`

// TestContainerStepOverride_RejectsUnknownField confirms a typo'd/nonexistent
// field in a step-level `container:` mapping (e.g. `imgae` instead of
// `image`) is rejected rather than silently dropped, for both the
// workflow-file path (yaml.Unmarshal -> WorkflowContainer.UnmarshalYAML) and
// the custom-command path (mapstructure + TasksDecodeHook ->
// decodeTaskContainerFromMapValue -> the same UnmarshalYAML). Before this
// fix, WorkflowContainer.UnmarshalYAML's mapping branch used plain
// value.Decode, which has no KnownFields/strict mode, so `imgae:` was
// silently discarded with no error and no trace in the decoded struct.
func TestContainerStepOverride_RejectsUnknownField(t *testing.T) {
	t.Run("workflow file path", func(t *testing.T) {
		var fromYAML Tasks
		err := yaml.Unmarshal([]byte(containerStepOverrideUnknownFieldYAML), &fromYAML)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidWorkflowContainer))
		assert.Contains(t, err.Error(), "imgae")
	})

	t.Run("custom command path", func(t *testing.T) {
		var generic []any
		require.NoError(t, yaml.Unmarshal([]byte(containerStepOverrideUnknownFieldYAML), &generic))

		var fromMapstructure Tasks
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:           &fromMapstructure,
			TagName:          "mapstructure",
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				ConditionDecodeHook(),
				WorkflowStepDecodeHook(),
				TasksDecodeHook(),
			),
		})
		require.NoError(t, err)
		err = decoder.Decode(generic)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidWorkflowContainer))
		assert.Contains(t, err.Error(), "imgae")
	})
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

// withValueMarshalError implements yaml.Marshaler and always fails, so
// decodeStepWithFromMapValue's yaml.Marshal(withValue) call surfaces a real
// error instead of silently dropping it or panicking.
type withValueMarshalError struct{}

var errWithValueMarshal = errors.New("with-value marshal failed")

func (withValueMarshalError) MarshalYAML() (any, error) {
	return nil, errWithValueMarshal
}

// TestDecodeStepWithFromMapValue_MarshalErrorPropagates verifies that when the
// `with:` value round-tripped through YAML (see decodeStepWithFromMapValue's
// doc comment) cannot itself be marshaled, the error is returned rather than
// swallowed. A mapstructure/Viper-sourced `with:` value can implement
// yaml.Marshaler indirectly (e.g. via an embedded custom type), so this is a
// real, reachable failure mode of the round-trip, not a contrived one.
func TestDecodeStepWithFromMapValue_MarshalErrorPropagates(t *testing.T) {
	err := decodeStepWithFromMapValue(withValueMarshalError{}, TaskTypeShell, "", &stepPolyTargets{generic: &map[string]any{}})
	require.Error(t, err)
	assert.ErrorIs(t, err, errWithValueMarshal)
}

// withValueDanglingAlias implements yaml.Marshaler by returning a *yaml.Node
// alias with no corresponding anchor. Marshaling happily serializes this
// (it does not validate that aliases resolve) as the literal text `*x`, but
// unmarshaling that text back into a *yaml.Node fails with "unknown anchor
// 'x' referenced" -- a genuine, non-contrived way the second round-trip step
// (yaml.Unmarshal(withBytes, &doc)) in decodeStepWithFromMapValue can fail
// even though the preceding yaml.Marshal succeeded.
type withValueDanglingAlias struct{}

func (withValueDanglingAlias) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.AliasNode, Value: "x"}, nil
}

// TestDecodeStepWithFromMapValue_UnmarshalErrorPropagates verifies that when
// the intermediate YAML bytes produced by yaml.Marshal cannot themselves be
// parsed back by yaml.Unmarshal, decodeStepWithFromMapValue returns that error
// rather than swallowing it.
func TestDecodeStepWithFromMapValue_UnmarshalErrorPropagates(t *testing.T) {
	err := decodeStepWithFromMapValue(withValueDanglingAlias{}, TaskTypeShell, "", &stepPolyTargets{generic: &map[string]any{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown anchor")
}
