package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWorkflowContainerUnmarshalMappingAndStepFalse(t *testing.T) {
	input := `
workflows:
  test:
    container:
      image: alpine:latest
      env:
        SHARED: workflow
      mounts:
        - source: ~/.aws
          target: /home/app/.aws
          read_only: true
    steps:
      - name: host
        type: shell
        command: echo host
        container: false
`
	var manifest WorkflowManifest
	require.NoError(t, yaml.Unmarshal([]byte(input), &manifest))

	workflow := manifest.Workflows["test"]
	require.NotNil(t, workflow.Container)
	assert.True(t, workflow.Container.IsEnabled())
	assert.Equal(t, "alpine:latest", workflow.Container.Image)
	assert.Equal(t, "workflow", workflow.Container.Env["SHARED"])
	require.Len(t, workflow.Container.Mounts, 1)
	assert.True(t, workflow.Container.Mounts[0].ReadOnly)

	require.Len(t, workflow.Steps, 1)
	require.NotNil(t, workflow.Steps[0].Container)
	assert.False(t, workflow.Steps[0].Container.IsEnabled())
}

func TestWorkflowContainerUnmarshalStepOverride(t *testing.T) {
	input := `
workflows:
  test:
    container:
      image: alpine:latest
    steps:
      - name: isolated
        type: shell
        command: echo isolated
        container:
          image: node:22
          env:
            NODE_ENV: test
`
	var manifest WorkflowManifest
	require.NoError(t, yaml.Unmarshal([]byte(input), &manifest))

	step := manifest.Workflows["test"].Steps[0]
	require.NotNil(t, step.Container)
	assert.True(t, step.Container.IsEnabled())
	assert.Equal(t, "node:22", step.Container.Image)
	assert.Equal(t, "test", step.Container.Env["NODE_ENV"])
}

// TestWorkflowContainerUnmarshalRejectsInvalidScalar verifies a non-boolean scalar
// value for `container:` fails to decode, rather than being silently coerced.
func TestWorkflowContainerUnmarshalRejectsInvalidScalar(t *testing.T) {
	var c WorkflowContainer
	err := yaml.Unmarshal([]byte("not-a-bool"), &c)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidWorkflowContainer))
}

// TestWorkflowContainerUnmarshalRejectsInvalidMapping verifies a mapping whose fields
// fail typed decoding (e.g. image is a list, not a string) surfaces a wrapped error.
func TestWorkflowContainerUnmarshalRejectsInvalidMapping(t *testing.T) {
	var c WorkflowContainer
	err := yaml.Unmarshal([]byte("image: [not, a, string]\n"), &c)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidWorkflowContainer))
}

// TestWorkflowContainerUnmarshalRejectsSequence verifies the default-kind branch
// rejects a YAML sequence value for `container:`.
func TestWorkflowContainerUnmarshalRejectsSequence(t *testing.T) {
	var c WorkflowContainer
	err := yaml.Unmarshal([]byte("- a\n- b\n"), &c)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidWorkflowContainer))
}
