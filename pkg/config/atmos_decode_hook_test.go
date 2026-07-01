package config

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestAtmosDecodeHook_StringToTimeDuration tests that the decode hook
// correctly handles string to time.Duration conversion.
func TestAtmosDecodeHook_StringToTimeDuration(t *testing.T) {
	type config struct {
		Timeout time.Duration `mapstructure:"timeout"`
	}

	yamlContent := `timeout: 30s`

	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewReader([]byte(yamlContent)))
	require.NoError(t, err)

	var result config
	err = v.Unmarshal(&result, atmosDecodeHook())
	require.NoError(t, err)

	assert.Equal(t, 30*time.Second, result.Timeout)
}

// TestAtmosDecodeHook_StringToSlice tests that the decode hook
// correctly handles string to slice conversion.
func TestAtmosDecodeHook_StringToSlice(t *testing.T) {
	type config struct {
		Tags []string `mapstructure:"tags"`
	}

	yamlContent := `tags: "tag1,tag2,tag3"`

	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewReader([]byte(yamlContent)))
	require.NoError(t, err)

	var result config
	err = v.Unmarshal(&result, atmosDecodeHook())
	require.NoError(t, err)

	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, result.Tags)
}

// TestAtmosDecodeHook_TasksDecodeHook tests that the decode hook
// correctly handles Tasks (flexible command steps) conversion.
func TestAtmosDecodeHook_TasksDecodeHook(t *testing.T) {
	type config struct {
		Steps schema.Tasks `mapstructure:"steps"`
	}

	yamlContent := `
steps:
  - "echo hello"
  - name: structured
    command: "echo world"
    timeout: 1m
`

	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewReader([]byte(yamlContent)))
	require.NoError(t, err)

	var result config
	err = v.Unmarshal(&result, atmosDecodeHook())
	require.NoError(t, err)

	require.Len(t, result.Steps, 2)
	assert.Equal(t, "echo hello", result.Steps[0].Command)
	assert.Equal(t, schema.TaskTypeShell, result.Steps[0].Type)
	assert.Equal(t, "structured", result.Steps[1].Name)
	assert.Equal(t, "echo world", result.Steps[1].Command)
	assert.Equal(t, time.Minute, result.Steps[1].Timeout)
}

func TestAtmosDecodeHook_NestedCastSimulatePromptInCommandSteps(t *testing.T) {
	type config struct {
		Commands []schema.Command `mapstructure:"commands"`
	}

	yamlContent := `
commands:
  - name: casts
    commands:
      - name: generate
        commands:
          - name: sops-secrets
            steps:
              - type: cast
                mode: steps
                steps:
                  - type: simulate
                    mode: typed
                    prompt: &demo_prompt
                      text: "> "
                      style: command
                    text: atmos secret list --stack dev --component api
                  - type: shell
                    command: atmos secret list --stack dev --component api
                  - type: simulate
                    mode: prompt
                    prompt: *demo_prompt
`

	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewReader([]byte(yamlContent)))
	require.NoError(t, err)

	var result config
	err = v.Unmarshal(&result, atmosDecodeHook())
	require.NoError(t, err)

	require.Len(t, result.Commands, 1)
	require.Len(t, result.Commands[0].Commands, 1)
	require.Len(t, result.Commands[0].Commands[0].Commands, 1)
	steps := result.Commands[0].Commands[0].Commands[0].Steps
	require.Len(t, steps, 1)
	require.Len(t, steps[0].Steps, 3)
	require.NotNil(t, steps[0].Steps[0].SimulatePrompt)
	assert.Equal(t, "> ", steps[0].Steps[0].SimulatePrompt.Text)
	assert.Equal(t, "command", steps[0].Steps[0].SimulatePrompt.Style)
	assert.Empty(t, steps[0].Steps[0].Prompt)
	require.NotNil(t, steps[0].Steps[2].SimulatePrompt)
	assert.Equal(t, "> ", steps[0].Steps[2].SimulatePrompt.Text)
	assert.Equal(t, "command", steps[0].Steps[2].SimulatePrompt.Style)
}

// TestAtmosDecodeHook_Combined tests that all decode hooks work together.
func TestAtmosDecodeHook_Combined(t *testing.T) {
	type config struct {
		Timeout time.Duration `mapstructure:"timeout"`
		Tags    []string      `mapstructure:"tags"`
		Steps   schema.Tasks  `mapstructure:"steps"`
	}

	yamlContent := `
timeout: 2m
tags: "dev,test,prod"
steps:
  - "echo simple"
  - command: "terraform plan"
    type: atmos
`

	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewReader([]byte(yamlContent)))
	require.NoError(t, err)

	var result config
	err = v.Unmarshal(&result, atmosDecodeHook())
	require.NoError(t, err)

	assert.Equal(t, 2*time.Minute, result.Timeout)
	assert.Equal(t, []string{"dev", "test", "prod"}, result.Tags)
	require.Len(t, result.Steps, 2)
	assert.Equal(t, "echo simple", result.Steps[0].Command)
	assert.Equal(t, schema.TaskTypeAtmos, result.Steps[1].Type)
}
