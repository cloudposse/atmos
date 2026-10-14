package workflow

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a rename of the field these tests rely on must fail the build.
var _ = schema.WorkflowStep{LiteralFields: nil}

func TestLoadManifestRecordsLiteralFields(t *testing.T) {
	base, manifest := manifestProject(t)
	content := `
workflows:
  demo:
    steps:
      - name: script
        type: script
        interpreter: starlark
        script: !literal |
          print("{{ x }}")
      - name: shell
        type: shell
        command: !literal echo "{{ y }}"
      - name: fields
        type: script
        interpreter: !literal "{{ i }}"
        working_directory: !literal "{{ w }}"
        env:
          GREETING: !literal "{{ g }}"
          PLAIN: "{{ p }}"
        script: print("{{ z }}")
      - name: group
        type: parallel
        steps:
          - name: child
            type: script
            interpreter: starlark
            script: !literal 'print("{{ child }}")'
          - name: plain-child
            type: shell
            command: echo "{{ plain }}"
      - name: plain
        type: shell
        command: echo "{{ plain }}"
      - name: handwritten
        type: shell
        literal_fields: [command]
        command: echo "{{ nope }}"
`
	parsed, err := loadTestManifest(t, base, manifest, content)
	require.NoError(t, err)
	steps := parsed.Workflows["demo"].Steps
	require.Len(t, steps, 6)

	assert.Equal(t, []string{"script"}, steps[0].LiteralFields)
	assert.Equal(t, "print(\"{{ x }}\")\n", steps[0].Script, "the value is kept exactly")
	assert.Equal(t, []string{"command"}, steps[1].LiteralFields)
	assert.Equal(t, `echo "{{ y }}"`, steps[1].Command)
	assert.Equal(t, []string{"interpreter", "working_directory", "env.GREETING"}, steps[2].LiteralFields)
	assert.False(t, steps[2].IsLiteral("script"), "a sibling field without the tag still renders")
	assert.Equal(t, "{{ g }}", steps[2].Env["GREETING"])

	require.Len(t, steps[3].Steps, 2)
	assert.Equal(t, []string{"script"}, steps[3].Steps[0].LiteralFields)
	assert.Empty(t, steps[3].Steps[1].LiteralFields)

	assert.Empty(t, steps[4].LiteralFields)
	assert.Empty(t, steps[5].LiteralFields, "a user-written literal_fields is never honored")
}

func TestLoadManifestRecordsContainerRunWorkingDirectoryLiteral(t *testing.T) {
	base, manifest := manifestProject(t)
	content := `
workflows:
  demo:
    steps:
      - name: run
        type: container
        action: run
        working_directory: !literal "{{ w }}"
        run:
          image: alpine
          command: echo
      - name: implicit
        type: container
        working_directory: !literal "{{ w }}"
        run:
          image: alpine
      - name: plain
        type: container
        action: run
        working_directory: "{{ w }}"
        literal_fields: [working_directory]
        run:
          image: alpine
`
	parsed, err := loadTestManifest(t, base, manifest, content)
	require.NoError(t, err)
	steps := parsed.Workflows["demo"].Steps
	require.Len(t, steps, 3)

	assert.Equal(t, []string{"working_directory"}, steps[0].LiteralFields)
	assert.Equal(t, "{{ w }}", steps[0].WorkingDirectory, "the value is kept exactly")
	assert.Equal(t, []string{"working_directory"}, steps[1].LiteralFields)
	assert.Empty(t, steps[2].LiteralFields, "a user-written literal_fields is never honored")
}

func TestLoadManifestLeavesPlainWorkflowsUnmarked(t *testing.T) {
	base, manifest := manifestProject(t)
	parsed, err := loadTestManifest(t, base, manifest, "workflows:\n  w:\n    steps:\n      - {name: a, type: shell, command: 'echo \"{{ y }}\"'}\n")
	require.NoError(t, err)
	assert.Empty(t, parsed.Workflows["w"].Steps[0].LiteralFields)
}

func TestControlStepKeepsLiteralFieldsUnrendered(t *testing.T) {
	templateData := func(string, map[string]string) map[string]any {
		return map[string]any{"custom": "value"}
	}
	parent := &schema.WorkflowStep{
		Name: "checks",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: boolPtr(false),
		},
		Steps: []schema.WorkflowStep{
			{
				Name:             "mixed",
				Type:             schema.TaskTypeScript,
				Interpreter:      "{{ .custom }}-interp",
				Script:           "print('{{ .custom }}')",
				Command:          "echo {{ .custom }}",
				Stack:            "{{ .custom }}-stack",
				WorkingDirectory: "{{ .custom }}-dir",
				Env:              map[string]string{"LIT": "{{ .custom }}", "TPL": "{{ .custom }}"},
				LiteralFields:    []string{"script", "command", "working_directory", "env.LIT"},
			},
			{
				Name:    "unmarked",
				Type:    schema.TaskTypeShell,
				Command: "echo {{ .custom }}",
			},
		},
	}

	// Parallel children run concurrently, so the recorder serializes its writes.
	var mu sync.Mutex
	got := map[string]schema.WorkflowStep{}
	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		mu.Lock()
		defer mu.Unlock()
		got[child.Step.Name] = child.Step
		return &ControlChildResult{}, nil
	}, ControlExecutionOptions{TemplateData: templateData})
	require.NoError(t, err)

	mixed := got["mixed"]
	assert.Equal(t, "print('{{ .custom }}')", mixed.Script, "literal script is kept")
	assert.Equal(t, "echo {{ .custom }}", mixed.Command, "literal command is kept")
	assert.Equal(t, "{{ .custom }}-dir", mixed.WorkingDirectory, "literal working_directory is kept")
	assert.Equal(t, map[string]string{"LIT": "{{ .custom }}", "TPL": "value"}, mixed.Env, "only the literal env value is kept")
	assert.Equal(t, "value-interp", mixed.Interpreter, "a field without the tag still renders")
	assert.Equal(t, "value-stack", mixed.Stack, "a field without the tag still renders")
	assert.Equal(t, []string{"script", "command", "working_directory", "env.LIT"}, mixed.LiteralFields)

	assert.Equal(t, "echo value", got["unmarked"].Command, "an unmarked child still renders")
}

func TestMatrixChildKeepsLiteralScriptUnrendered(t *testing.T) {
	parent := &schema.WorkflowStep{
		Name:   "plans",
		Type:   schema.TaskTypeMatrix,
		Matrix: map[string][]string{"component": {"vpc", "eks"}},
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: boolPtr(false),
		},
		Steps: []schema.WorkflowStep{{
			Name:          "plan",
			Type:          schema.TaskTypeScript,
			Interpreter:   "starlark",
			Script:        "print('{{ .matrix.component }}')",
			Stack:         "{{ .matrix.component }}-stack",
			LiteralFields: []string{"script"},
		}},
	}

	// Matrix children run concurrently, so the recorder serializes its appends.
	var mu sync.Mutex
	var scripts, stacks []string
	err := ExecuteControlStep(context.Background(), parent, func(_ context.Context, child *ControlChild, _ ControlChildOutput) (*ControlChildResult, error) {
		mu.Lock()
		defer mu.Unlock()
		scripts = append(scripts, child.Step.Script)
		stacks = append(stacks, child.Step.Stack)
		return &ControlChildResult{}, nil
	}, ControlExecutionOptions{})
	require.NoError(t, err)

	assert.Equal(t, []string{"print('{{ .matrix.component }}')", "print('{{ .matrix.component }}')"}, scripts)
	assert.ElementsMatch(t, []string{"vpc-stack", "eks-stack"}, stacks)
}

func TestControlExecutorRunsLiteralScriptSourceVerbatim(t *testing.T) {
	engine := registerRecordingEngine(t, "recording-literal-script")
	parent := &schema.WorkflowStep{
		Name: "group",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: boolPtr(false),
		},
		Steps: []schema.WorkflowStep{
			{Name: "literal", Type: schema.TaskTypeScript, Interpreter: "recording-literal-script", Script: `print("{{ x }}")`, LiteralFields: []string{"script"}},
		},
	}
	executor := &ControlCommandExecutor{}
	err := ExecuteControlStep(context.Background(), parent, executor.Execute, ControlExecutionOptions{})
	require.NoError(t, err)

	executed := engine.executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `print("{{ x }}")`, executed[0].Source)
}

func TestControlStepNonLiteralScriptWithBracesStillFailsToRender(t *testing.T) {
	// Negative path: without the marker the same script is rendered as a template and fails,
	// which is exactly what !literal opts out of.
	parent := &schema.WorkflowStep{
		Name: "group",
		Type: schema.TaskTypeParallel,
		ParallelOutput: &schema.ParallelOutputConfig{
			Mode:        ControlOutputNone,
			ShowSummary: boolPtr(false),
		},
		Steps: []schema.WorkflowStep{
			{Name: "plain", Type: schema.TaskTypeScript, Interpreter: "starlark", Script: `print("{{ x }}")`},
		},
	}
	err := ExecuteControlStep(context.Background(), parent, func(context.Context, *ControlChild, ControlChildOutput) (*ControlChildResult, error) {
		t.Fatal("the executor must not run when rendering fails")
		return nil, nil
	}, ControlExecutionOptions{})
	require.Error(t, err)
}

func TestRenderScriptInterpreterSkipsLiteralInterpreter(t *testing.T) {
	render := func(string) (string, error) { return "rendered", nil }

	literal := &schema.WorkflowStep{Type: schema.TaskTypeScript, Interpreter: "{{ i }}", LiteralFields: []string{"interpreter"}}
	require.NoError(t, RenderScriptInterpreter(literal, render))
	assert.Equal(t, "{{ i }}", literal.Interpreter)

	plain := &schema.WorkflowStep{Type: schema.TaskTypeScript, Interpreter: "{{ i }}"}
	require.NoError(t, RenderScriptInterpreter(plain, render))
	assert.Equal(t, "rendered", plain.Interpreter)

	failing := &schema.WorkflowStep{Type: schema.TaskTypeScript, Interpreter: "{{ i }}"}
	err := RenderScriptInterpreter(failing, func(string) (string, error) { return "", context.Canceled })
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
}
