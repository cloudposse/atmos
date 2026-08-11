package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWorkflowStep_UnmarshalYAML_StructuredParallelOutput(t *testing.T) {
	input := `
type: parallel
output:
  mode: grouped
  order: definition
  show_summary: false
  prefix: "{{ .step.name }}"
steps:
  - name: test
    type: shell
    command: make test
`
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	require.NotNil(t, step.ParallelOutput)
	assert.Equal(t, "grouped", step.Output)
	assert.Equal(t, "grouped", step.ParallelOutput.Mode)
	assert.Equal(t, "definition", step.ParallelOutput.Order)
	require.NotNil(t, step.ParallelOutput.ShowSummary)
	assert.False(t, *step.ParallelOutput.ShowSummary)
	assert.Equal(t, "{{ .step.name }}", step.ParallelOutput.Prefix)
}

func TestWorkflowStep_UnmarshalYAML_StructuredCastOutputMode(t *testing.T) {
	input := `
name: demo
type: cast
output:
  mode: raw
  cast: demo.cast
  gif: demo.gif
steps:
  - name: list
    command: atmos list stacks
`
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	assert.Equal(t, "raw", step.Output)
	require.NotNil(t, step.CastOutput)
	assert.Equal(t, "raw", step.CastOutput.Mode)
	assert.Equal(t, "demo.cast", step.CastOutput.Cast)
	assert.Equal(t, "demo.gif", step.CastOutput.GIF)
}

func TestWorkflowStep_UnmarshalYAML_AliasedStructuredCastOutput(t *testing.T) {
	input := `
output: &demo_output
  mode: raw
  cast: demo.cast
type: cast
steps:
  - name: nested
    type: cast
    output: *demo_output
`
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	require.Len(t, step.Steps, 1)
	assert.Equal(t, "raw", step.Steps[0].Output)
	require.NotNil(t, step.Steps[0].CastOutput)
	assert.Equal(t, "raw", step.Steps[0].CastOutput.Mode)
	assert.Equal(t, "demo.cast", step.Steps[0].CastOutput.Cast)
}

func TestWorkflowStep_UnmarshalYAML_StructuredSimulatePromptAndCommandAnchor(t *testing.T) {
	input := `
type: cast
mode: steps
steps:
  - type: simulate
    mode: typed
    cursor: true
    jitter: 0.25
    prompt: &demo_prompt
      text: "> "
      style: command
    text: &list_cmd atmos secret list --stack dev --component api
  - type: shell
    name: list-secrets
    command: *list_cmd
  - type: simulate
    mode: prompt
    prompt: *demo_prompt
`
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	require.Len(t, step.Steps, 3)
	require.NotNil(t, step.Steps[0].SimulatePrompt)
	assert.Equal(t, "> ", step.Steps[0].SimulatePrompt.Text)
	assert.Equal(t, "command", step.Steps[0].SimulatePrompt.Style)
	assert.True(t, step.Steps[0].Cursor)
	assert.Equal(t, 0.25, step.Steps[0].Jitter)
	assert.Equal(t, "atmos secret list --stack dev --component api", step.Steps[0].Text)
	assert.Equal(t, "atmos secret list --stack dev --component api", step.Steps[1].Command)
	require.NotNil(t, step.Steps[2].SimulatePrompt)
	assert.Equal(t, "> ", step.Steps[2].SimulatePrompt.Text)
	assert.Equal(t, "command", step.Steps[2].SimulatePrompt.Style)
}

func TestWorkflowStep_UnmarshalYAML_CastSimulateDefaults(t *testing.T) {
	input := `
type: cast
defaults:
  cast:
    rate: 12ms
    width: 120
    height: 36
  simulate:
    mode: typed
    cursor: true
    rate: 35ms
    jitter: 0.25
    duration: 10ms
    interval: 20ms
    prompt:
      text: "> "
      style: command
steps:
  - type: simulate
    cursor: false
    text: atmos version
`
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	require.NotNil(t, step.Defaults)
	require.NotNil(t, step.Defaults.Cast)
	assert.Equal(t, "12ms", step.Defaults.Cast.Rate)
	assert.Equal(t, 120, step.Defaults.Cast.Width)
	assert.Equal(t, 36, step.Defaults.Cast.Height)
	require.NotNil(t, step.Defaults.Simulate)
	assert.Equal(t, "typed", step.Defaults.Simulate.Mode)
	require.NotNil(t, step.Defaults.Simulate.Cursor)
	assert.True(t, *step.Defaults.Simulate.Cursor)
	assert.Equal(t, "35ms", step.Defaults.Simulate.Rate)
	assert.Equal(t, 0.25, step.Defaults.Simulate.Jitter)
	assert.Equal(t, "10ms", step.Defaults.Simulate.Duration)
	assert.Equal(t, "20ms", step.Defaults.Simulate.Interval)
	require.NotNil(t, step.Defaults.Simulate.Prompt)
	assert.Equal(t, "> ", step.Defaults.Simulate.Prompt.Text)
	assert.Equal(t, "command", step.Defaults.Simulate.Prompt.Style)
	require.Len(t, step.Steps, 1)
	assert.False(t, step.Steps[0].Cursor)
	assert.True(t, step.Steps[0].CursorSet)
}

func TestWorkflowStep_UnmarshalYAML_ScalarPromptStillDecodesForInteractiveStep(t *testing.T) {
	input := `
type: input
prompt: Continue?
`
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	assert.Equal(t, "Continue?", step.Prompt)
	assert.Nil(t, step.SimulatePrompt)
}

// TestWorkflowStep_UnmarshalYAML_RejectsStructuredPromptForNonSimulateType verifies
// decodeStepPrompt's mapping-node branch rejects a structured prompt for any type
// other than TaskTypeSimulate.
func TestWorkflowStep_UnmarshalYAML_RejectsStructuredPromptForNonSimulateType(t *testing.T) {
	input := `
type: input
prompt:
  text: "> "
  style: command
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkflowControlStepInvalid))
}

// TestWorkflowStep_UnmarshalYAML_PromptDecodeErrorPropagates verifies a structured
// prompt with a field of the wrong type surfaces the underlying decode error.
func TestWorkflowStep_UnmarshalYAML_PromptDecodeErrorPropagates(t *testing.T) {
	input := `
type: simulate
prompt:
  text: [not, a, string]
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
}

// TestWorkflowStep_UnmarshalYAML_PromptDefaultNodeDecode verifies the default branch
// of decodeStepPrompt (a non-scalar, non-mapping prompt node, e.g. a sequence) is
// passed through to node.Decode, which fails for a *string destination.
func TestWorkflowStep_UnmarshalYAML_PromptDefaultNodeDecode(t *testing.T) {
	input := `
type: input
prompt:
  - a
  - b
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
}

// TestWorkflowStep_UnmarshalYAML_StepsSequenceDecodeErrorPropagates verifies that an
// error decoding a nested step in the `steps:` sequence (via the recursive
// step.UnmarshalYAML call) propagates up through decodeWorkflowStepList.
func TestWorkflowStep_UnmarshalYAML_StepsSequenceDecodeErrorPropagates(t *testing.T) {
	input := `
type: parallel
steps:
  - type: input
    prompt:
      text: "> "
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkflowControlStepInvalid))
}

// TestWorkflowStep_UnmarshalYAML_StepsNonSequenceDecodeError verifies decodeWorkflowStepList's
// non-sequence branch delegates to node.Decode, which fails when `steps:` is a scalar.
func TestWorkflowStep_UnmarshalYAML_StepsNonSequenceDecodeError(t *testing.T) {
	input := `
type: parallel
steps: not-a-sequence
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
}

// TestWorkflowStep_UnmarshalYAML_SanitizedDecodeErrorPropagates verifies that a decode
// failure on the sanitized (non-polymorphic) portion of the mapping - here,
// max_concurrency set to a mapping instead of an int - surfaces as an error from the
// plain-struct decode step, before any polymorphic-field handling runs.
func TestWorkflowStep_UnmarshalYAML_SanitizedDecodeErrorPropagates(t *testing.T) {
	input := `
type: parallel
max_concurrency: {a: b}
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
}

// TestWorkflowStep_UnmarshalYAML_BackgroundDecodeErrorPropagates verifies that an
// error from decodeStepBackground (a non-scalar `background:` value) propagates up
// through applyStepPolymorphicNodes.
func TestWorkflowStep_UnmarshalYAML_BackgroundDecodeErrorPropagates(t *testing.T) {
	input := `
type: container
background: [true, false]
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrWorkflowControlStepInvalid))
}

func TestValidateWorkflowSteps_ControlSteps(t *testing.T) {
	tests := []struct {
		name    string
		steps   []WorkflowStep
		wantErr string
	}{
		{
			name: "valid parallel needs",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{
					{Name: "lint", Type: TaskTypeShell, Command: "make lint"},
					{Name: "test", Type: TaskTypeShell, Command: "make test", Needs: []string{"lint"}},
				},
			}},
		},
		{
			name: "valid parallel script child",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{
					{Name: "script", Type: TaskTypeScript, Interpreter: "python3", Script: "print('ok')"},
				},
			}},
		},
		{
			name: "missing need",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test", Needs: []string{"lint"}}},
			}},
			wantErr: "needs unknown step",
		},
		{
			name: "cycle",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{
					{Name: "a", Type: TaskTypeShell, Command: "a", Needs: []string{"b"}},
					{Name: "b", Type: TaskTypeShell, Command: "b", Needs: []string{"a"}},
				},
			}},
			wantErr: "cyclic needs dependency",
		},
		{
			name: "unsupported child type disallowed",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Steps: []WorkflowStep{{Name: "prompt", Type: "input", Prompt: "Continue?"}},
			}},
			wantErr: "cannot run inside concurrent step",
		},
		{
			name: "interactive child disallowed",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{{
					Name:        "prompt",
					Type:        TaskTypeShell,
					Command:     "read answer",
					Interactive: true,
				}},
			}},
			wantErr: "cannot set tty or interactive",
		},
		{
			name: "top-level duplicate names allowed",
			steps: []WorkflowStep{
				{Name: "deploy", Type: TaskTypeShell, Command: "echo first"},
				{Name: "deploy", Type: TaskTypeShell, Command: "echo second"},
			},
		},
		{
			name: "top-level needs disallowed",
			steps: []WorkflowStep{
				{Name: "build", Type: TaskTypeShell, Command: "make build"},
				{Name: "test", Type: TaskTypeShell, Command: "make test", Needs: []string{"build"}},
			},
			wantErr: "sets needs outside a concurrent control step",
		},
		{
			name: "duplicate child names disallowed",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{
					{Name: "lint", Type: TaskTypeShell, Command: "make lint"},
					{Name: "lint", Type: TaskTypeShell, Command: "make lint-again"},
				},
			}},
			wantErr: "duplicate step name",
		},
		{
			// The failing child omits Name, so workflowStepName falls back to
			// "step<index+1>" ("step2") when reporting the unknown `needs` target.
			name: "unnamed child uses fallback name in needs error",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{
					{Name: "lint", Type: TaskTypeShell, Command: "make lint"},
					{Type: TaskTypeShell, Command: "make test", Needs: []string{"missing"}},
				},
			}},
			wantErr: `step "step2" in control step "checks" needs unknown step "missing"`,
		},
		{
			name: "invalid output mode",
			steps: []WorkflowStep{{
				Name:   "checks",
				Type:   TaskTypeParallel,
				Output: "raw",
				Steps:  []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "unsupported output mode",
		},
		{
			name: "invalid fail mode",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Fail:  &ParallelFailConfig{Mode: "sometimes"},
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "unsupported fail.mode",
		},
		{
			name: "matrix needs axes",
			steps: []WorkflowStep{{
				Name:  "plans",
				Type:  TaskTypeMatrix,
				Steps: []WorkflowStep{{Name: "plan", Type: TaskTypeShell, Command: "plan"}},
			}},
			wantErr: "requires at least one matrix axis",
		},
		{
			name: "unnamed control step needs children",
			steps: []WorkflowStep{{
				Type: TaskTypeParallel,
			}},
			wantErr: "step at index 0 requires at least one nested step",
		},
		{
			name: "negative max concurrency",
			steps: []WorkflowStep{{
				Name:           "checks",
				Type:           TaskTypeParallel,
				MaxConcurrency: -1,
				Steps:          []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "sets negative max_concurrency",
		},
		{
			name: "negative fail max failures",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Fail:  &ParallelFailConfig{MaxFailures: -1},
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "sets negative fail.max_failures",
		},
		{
			name: "matrix empty axis name",
			steps: []WorkflowStep{{
				Name:   "plans",
				Type:   TaskTypeMatrix,
				Matrix: map[string][]string{"": {"dev"}},
				Steps:  []WorkflowStep{{Name: "plan", Type: TaskTypeShell, Command: "plan"}},
			}},
			wantErr: "contains an empty matrix axis name",
		},
		{
			name: "matrix empty axis values",
			steps: []WorkflowStep{{
				Name:   "plans",
				Type:   TaskTypeMatrix,
				Matrix: map[string][]string{"stack": {}},
				Steps:  []WorkflowStep{{Name: "plan", Type: TaskTypeShell, Command: "plan"}},
			}},
			wantErr: "must contain at least one value",
		},
		{
			name: "invalid output order",
			steps: []WorkflowStep{{
				Name:           "checks",
				Type:           TaskTypeParallel,
				ParallelOutput: &ParallelOutputConfig{Mode: "grouped", Order: "random"},
				Steps:          []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
			wantErr: "sets unsupported output.order",
		},
		{
			name: "default child type allowed",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Steps: []WorkflowStep{{Name: "plan", Command: "terraform plan"}},
			}},
		},
		{
			name: "valid fail config allowed",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Fail:  &ParallelFailConfig{Mode: "fail_fast", MaxFailures: 1},
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test"}},
			}},
		},
		{
			name: "valid matrix allowed",
			steps: []WorkflowStep{{
				Name:   "plans",
				Type:   TaskTypeMatrix,
				Matrix: map[string][]string{"stack": {"dev", "prod"}},
				Steps:  []WorkflowStep{{Name: "plan", Type: TaskTypeShell, Command: "plan"}},
			}},
		},
		{
			name: "child output mode disallowed",
			steps: []WorkflowStep{{
				Name:  "checks",
				Type:  TaskTypeParallel,
				Steps: []WorkflowStep{{Name: "test", Type: TaskTypeShell, Command: "make test", Output: "raw"}},
			}},
			wantErr: "cannot set child output mode",
		},
		{
			name: "child nested steps disallowed",
			steps: []WorkflowStep{{
				Name: "checks",
				Type: TaskTypeParallel,
				Steps: []WorkflowStep{{
					Name:  "nested",
					Type:  TaskTypeShell,
					Steps: []WorkflowStep{{Name: "inner", Type: TaskTypeShell, Command: "echo inner"}},
				}},
			}},
			wantErr: "cannot declare nested steps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflowSteps(tt.steps)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestWorkflowStep_UnmarshalYAML_InvalidStructuredOutput(t *testing.T) {
	input := `
type: parallel
output:
  - grouped
steps:
  - name: test
    type: shell
    command: make test
`
	var step WorkflowStep
	require.Error(t, yaml.Unmarshal([]byte(input), &step))
}

// TestWorkflowStep_UnmarshalYAML_InvalidCastOutputMapping verifies decode errors from
// a mapping-shaped `output:` on a `type: cast` step (the CastOutput decode-error
// branch, distinct from the ParallelOutputConfig decode-error branch used by other
// step types).
func TestWorkflowStep_UnmarshalYAML_InvalidCastOutputMapping(t *testing.T) {
	input := `
type: cast
output:
  mode: [not, a, string]
steps:
  - name: list
    command: atmos list stacks
`
	var step WorkflowStep
	require.Error(t, yaml.Unmarshal([]byte(input), &step))
}

// TestWorkflowStep_UnmarshalYAML_OutputSequenceDefaultDecode verifies a non-scalar,
// non-mapping `output:` node (a plain sequence, with no cast/parallel keys involved)
// falls through to the default `node.Decode(scalar)` branch, which fails because a
// sequence cannot decode into a string.
func TestWorkflowStep_UnmarshalYAML_OutputSequenceDefaultDecode(t *testing.T) {
	input := `
type: shell
command: echo hi
output: [raw, grouped]
`
	var step WorkflowStep
	require.Error(t, yaml.Unmarshal([]byte(input), &step))
}

// TestWorkflowStep_UnmarshalYAML_ResetsReusedReceiver verifies that decoding into a
// reused WorkflowStep clears fields omitted from the second document (Decode merges
// into the destination, so without a reset the first step's fields would leak).
func TestWorkflowStep_UnmarshalYAML_ResetsReusedReceiver(t *testing.T) {
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte("name: first\ncommand: echo hi\ntype: shell\n"), &step))
	assert.Equal(t, "echo hi", step.Command)
	assert.Equal(t, "shell", step.Type)

	// Second decode into the SAME value omits command/type.
	require.NoError(t, yaml.Unmarshal([]byte("name: second\n"), &step))
	assert.Equal(t, "second", step.Name)
	assert.Empty(t, step.Command, "omitted command must be cleared, not retained from the first decode")
	assert.Empty(t, step.Type, "omitted type must be cleared, not retained from the first decode")
}

// TestWorkflowStep_UnmarshalYAML_ForRejectsNonStringScalar verifies `for:` accepts
// string scalars / sequences but rejects coerced scalars like booleans and numbers.
func TestWorkflowStep_UnmarshalYAML_ForRejectsNonStringScalar(t *testing.T) {
	// Valid: unquoted identifier resolves to !!str.
	var ok WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte("type: cancel\nfor: cache\n"), &ok))
	assert.Equal(t, []string{"cache"}, ok.For)

	// Valid: sequence of strings.
	var okList WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte("type: cancel\nfor: [redis, web]\n"), &okList))
	assert.Equal(t, []string{"redis", "web"}, okList.For)

	for _, bad := range []string{"type: cancel\nfor: true\n", "type: cancel\nfor: 1\n"} {
		var step WorkflowStep
		err := yaml.Unmarshal([]byte(bad), &step)
		require.Error(t, err, "input %q must be rejected", bad)
		assert.ErrorIs(t, err, ErrWorkflowControlStepInvalid)
	}
}

// TestWorkflowStep_UnmarshalYAML_ScalarOutputMode verifies decodeWorkflowStepOutput's
// ScalarNode branch: a bare `output: raw` value (not a mapping) sets Output directly
// without allocating CastOutput or ParallelOutput.
func TestWorkflowStep_UnmarshalYAML_ScalarOutputMode(t *testing.T) {
	var step WorkflowStep
	require.NoError(t, yaml.Unmarshal([]byte("type: shell\ncommand: echo hi\noutput: raw\n"), &step))
	assert.Equal(t, "raw", step.Output)
	assert.Nil(t, step.CastOutput)
	assert.Nil(t, step.ParallelOutput)
}

// TestWorkflowStep_UnmarshalYAML_StructuredCastOutputDecodeErrorPropagates verifies
// decodeWorkflowStepOutput's cast-mode decode-error branch surfaces the underlying
// yaml decode error (a field with the wrong type).
func TestWorkflowStep_UnmarshalYAML_StructuredCastOutputDecodeErrorPropagates(t *testing.T) {
	input := `
type: cast
output:
  mode: [not, a, string]
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
}

// TestWorkflowStep_UnmarshalYAML_StructuredParallelOutputDecodeErrorPropagates
// verifies decodeWorkflowStepOutput's non-cast structured-output decode-error branch
// surfaces the underlying yaml decode error.
func TestWorkflowStep_UnmarshalYAML_StructuredParallelOutputDecodeErrorPropagates(t *testing.T) {
	input := `
type: parallel
output:
  mode: [not, a, string]
`
	var step WorkflowStep
	err := yaml.Unmarshal([]byte(input), &step)
	require.Error(t, err)
}

// TestSplitMappingField_NonMappingNodePassesThrough verifies splitMappingField returns
// a nil field node and the original value unchanged for a nil or non-mapping node.
func TestSplitMappingField_NonMappingNodePassesThrough(t *testing.T) {
	fieldNode, rest := splitMappingField(nil, "output")
	assert.Nil(t, fieldNode)
	assert.Nil(t, rest)

	scalar := &yaml.Node{Kind: yaml.ScalarNode, Value: "raw"}
	fieldNode, rest = splitMappingField(scalar, "output")
	assert.Nil(t, fieldNode)
	assert.Same(t, scalar, rest)
}

// TestMappingHasField_NonMappingNodeReturnsFalse verifies mappingHasField returns
// false for a nil or non-mapping node instead of panicking.
func TestMappingHasField_NonMappingNodeReturnsFalse(t *testing.T) {
	assert.False(t, mappingHasField(nil, "cursor"))

	scalar := &yaml.Node{Kind: yaml.ScalarNode, Value: "raw"}
	assert.False(t, mappingHasField(scalar, "cursor"))
}
