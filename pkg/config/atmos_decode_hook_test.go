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
