package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestWorkflowStep_DecodeWith verifies that the container `action:`/`with:` vocabulary
// decodes into the typed action struct selected by `action` (defaulting to run).
func TestWorkflowStep_DecodeWith(t *testing.T) {
	t.Run("run is the default action", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte(`
type: container
with:
  image: alpine:latest
  command: echo hi
  ports:
    - { host: 8080, container: 80 }
`), &step))
		require.NotNil(t, step.Run)
		assert.Equal(t, "alpine:latest", step.Run.Image)
		assert.Equal(t, "echo hi", step.Run.Command)
		require.Len(t, step.Run.Ports, 1)
		assert.Equal(t, 8080, step.Run.Ports[0].Host)
		assert.Equal(t, 80, step.Run.Ports[0].Container)
		assert.Nil(t, step.Build)
		assert.Nil(t, step.Inspect)
	})

	t.Run("explicit build action", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte(`
type: container
action: build
with:
  context: .
  dockerfile: Dockerfile
  tags: [app:local]
`), &step))
		require.NotNil(t, step.Build)
		assert.Equal(t, "Dockerfile", step.Build.Dockerfile)
		require.Len(t, step.Build.Tags, 1)
		assert.Equal(t, "app:local", step.Build.Tags[0])
		assert.Nil(t, step.Run)
	})

	t.Run("verb-named blocks are no longer decoded", func(t *testing.T) {
		// Hard-cut: a `run:` block (the old syntax) must NOT populate the run config.
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte(`
type: container
action: run
run:
  image: alpine:latest
`), &step))
		assert.Nil(t, step.Run, "legacy run: block must be ignored after the with: hard-cut")
	})

	t.Run("non-container with is preserved as generic payload", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte(`
type: tflint
with:
  component: vpc
  stack: plat-ue2-dev
  args: [--minimum-failure-severity=error]
`), &step))
		assert.Equal(t, "tflint", step.Type)
		require.NotNil(t, step.With)
		assert.Equal(t, "vpc", step.With["component"])
		assert.Equal(t, "plat-ue2-dev", step.With["stack"])
		assert.Nil(t, step.Run)
	})

	t.Run("explicit push action", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte(`
type: container
action: push
with:
  image: alpine:latest
  tag: v1
`), &step))
		require.NotNil(t, step.Push)
		assert.Equal(t, "alpine:latest", step.Push.Image)
		assert.Nil(t, step.Run)
		assert.Nil(t, step.Build)
	})

	t.Run("explicit inspect action", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte(`
type: container
action: inspect
with:
  image: alpine:latest
`), &step))
		require.NotNil(t, step.Inspect)
		assert.Equal(t, "alpine:latest", step.Inspect.Image)
	})

	t.Run("unknown action with a with block errors", func(t *testing.T) {
		var step WorkflowStep
		err := yaml.Unmarshal([]byte(`
type: container
action: teleport
with:
  image: alpine:latest
`), &step)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrWorkflowControlStepInvalid)
	})

	t.Run("with block decode error propagates", func(t *testing.T) {
		var step WorkflowStep
		err := yaml.Unmarshal([]byte(`
type: container
action: build
with:
  tags: "not-a-list"
`), &step)
		require.Error(t, err)
	})

	t.Run("non-container with decode error propagates", func(t *testing.T) {
		// `with:` for a non-container step must be a mapping; a scalar cannot
		// decode into map[string]any and the error must surface, not be
		// swallowed.
		var step WorkflowStep
		err := yaml.Unmarshal([]byte(`
type: tflint
with: not-a-map
`), &step)
		require.Error(t, err)
	})
}

// TestDecodeStepWith_GenericNilGuard exercises decodeStepWith's defensive
// guard directly: every real caller wires stepPolyTargets.generic (see
// UnmarshalYAML for WorkflowStep and Task), so this branch is unreachable
// through the public YAML API. It still must not panic if a future caller
// forgets to wire it.
func TestDecodeStepWith_GenericNilGuard(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("component: vpc\n"), &node))
	// yaml.Unmarshal into a Node wraps content in a DocumentNode; unwrap to
	// the mapping node decodeStepWith expects.
	mapping := node.Content[0]

	err := decodeStepWith(mapping, "tflint", "", &stepPolyTargets{})
	require.NoError(t, err)
}

// TestWorkflowStep_DecodeBackground verifies the polymorphic `background:` key:
// a boolean sets the async marker; a string sets the style color.
func TestWorkflowStep_DecodeBackground(t *testing.T) {
	t.Run("boolean sets async marker", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte("type: container\nbackground: true\n"), &step))
		assert.True(t, step.BackgroundAsync)
		assert.Empty(t, step.Background)
	})

	t.Run("string sets style color", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte("type: style\nbackground: \"#1e1e2e\"\n"), &step))
		assert.False(t, step.BackgroundAsync)
		assert.Equal(t, "#1e1e2e", step.Background)
	})
}

// TestWorkflowStep_DecodeFor verifies that `for:` accepts both a scalar and a sequence.
func TestWorkflowStep_DecodeFor(t *testing.T) {
	t.Run("scalar", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte("type: cancel\nfor: emulator\n"), &step))
		assert.Equal(t, []string{"emulator"}, step.For)
	})

	t.Run("sequence", func(t *testing.T) {
		var step WorkflowStep
		require.NoError(t, yaml.Unmarshal([]byte("type: wait\nfor: [a, b]\n"), &step))
		assert.Equal(t, []string{"a", "b"}, step.For)
	})
}

// TestTask_DecodeWith confirms the custom-command Task flavor shares the same vocabulary.
func TestTask_DecodeWith(t *testing.T) {
	t.Run("container with decodes into selected action", func(t *testing.T) {
		var tasks Tasks
		require.NoError(t, yaml.Unmarshal([]byte(`
- type: container
  action: run
  background: true
  with:
    image: postgres:16
    command: ./migrate.sh
`), &tasks))
		require.Len(t, tasks, 1)
		assert.True(t, tasks[0].BackgroundAsync)
		require.NotNil(t, tasks[0].Run)
		assert.Equal(t, "postgres:16", tasks[0].Run.Image)
		assert.Equal(t, "./migrate.sh", tasks[0].Run.Command)
	})

	t.Run("non-container with is preserved", func(t *testing.T) {
		var tasks Tasks
		require.NoError(t, yaml.Unmarshal([]byte(`
- type: tflint
  with:
    component: vpc
    stack: plat-ue2-dev
`), &tasks))
		require.Len(t, tasks, 1)
		assert.Equal(t, "vpc", tasks[0].With["component"])
		assert.Nil(t, tasks[0].Run)
		step := tasks[0].ToWorkflowStep()
		assert.Equal(t, "vpc", step.With["component"])
	})
}
