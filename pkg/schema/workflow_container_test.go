package schema

import (
	"encoding/json"
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

// TestWorkflowContainerUnmarshalRejectsUnknownField verifies a typo'd/nonexistent
// field in a `container:` mapping (e.g. `imgae` instead of `image`) is rejected
// rather than silently discarded. Before this fix, the mapping branch used plain
// yaml.Node.Decode, which has no KnownFields/strict mode.
func TestWorkflowContainerUnmarshalRejectsUnknownField(t *testing.T) {
	var c WorkflowContainer
	err := yaml.Unmarshal([]byte("imgae: alpine\n"), &c)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidWorkflowContainer))
	assert.Contains(t, err.Error(), "imgae")
}

// TestWorkflowContainerJSONRoundTripPreservesEnabled reproduces the third leg
// of a field-test finding sibling to https://github.com/cloudposse/atmos/issues/2876:
// cmd/cmd_utils.go's cloneCommand deep-copies a schema.Command (including any
// step's Container) via json.Marshal/json.Unmarshal to give each custom
// command's Cobra closure an independent copy. Before MarshalJSON/UnmarshalJSON
// were added, WorkflowContainer's `Enabled *bool` field carried `json:"-"`
// (deliberately, since it's normally populated only by UnmarshalYAML's
// polymorphic bool-or-mapping decode) and was silently dropped by that
// generic JSON round-trip: a step's `container: false` opt-out came back as
// Enabled == nil, which IsEnabled() treats as *enabled*, inverting the
// opt-out and sending the step into a container it explicitly opted out of.
func TestWorkflowContainerJSONRoundTripPreservesEnabled(t *testing.T) {
	disabled := false
	original := &WorkflowContainer{
		Enabled:  &disabled,
		Image:    "alpine",
		Provider: "docker",
		Env:      map[string]string{"FOO": "bar"},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var round WorkflowContainer
	require.NoError(t, json.Unmarshal(data, &round))

	require.NotNil(t, round.Enabled, "Enabled must survive a JSON round-trip, not come back nil")
	assert.False(t, round.IsEnabled(), "container: false opt-out must survive a JSON round-trip")
	assert.Equal(t, original, &round)
}

// TestWorkflowContainerJSONRoundTripEnabledOmittedWhenNil confirms the
// "enabled" key is entirely absent (not just omitted-as-false) when Enabled
// is unset, matching `omitempty` on a *bool.
func TestWorkflowContainerJSONRoundTripEnabledOmittedWhenNil(t *testing.T) {
	original := &WorkflowContainer{Image: "alpine"}

	data, err := json.Marshal(original)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"enabled"`)

	var round WorkflowContainer
	require.NoError(t, json.Unmarshal(data, &round))
	assert.Nil(t, round.Enabled)
	assert.True(t, round.IsEnabled())
}

// TestWorkflowContainerUnmarshalJSONWrapsDecodeError verifies a
// type-mismatched field (syntactically valid JSON that still fails to
// decode into workflowContainerJSON) is wrapped in the static
// ErrInvalidWorkflowContainer sentinel (per this repo's error-handling
// conventions) rather than returned as a raw, unclassifiable
// json.Unmarshal error. A JSON syntax error (e.g. malformed input) never
// reaches UnmarshalJSON at all -- encoding/json rejects it during its own
// tokenizing pass before dispatching to any custom Unmarshaler -- so this
// must exercise the type-mismatch path specifically to reach the code
// under test.
func TestWorkflowContainerUnmarshalJSONWrapsDecodeError(t *testing.T) {
	var c WorkflowContainer
	err := json.Unmarshal([]byte(`{"image": 123}`), &c)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidWorkflowContainer))
}
