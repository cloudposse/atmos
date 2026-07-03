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

func TestAtmosDecodeHook_CommandEnvMap(t *testing.T) {
	type config struct {
		Commands []schema.Command `mapstructure:"commands"`
	}

	yamlContent := `
commands:
  - name: map-env
    env:
      CGO_ENABLED: "0"
      GOTOOLCHAIN: auto
      FROM_COMMAND:
        valueCommand: printf dynamic
    steps:
      - "echo ok"
`

	v := viper.New()
	v.SetConfigType("yaml")
	err := v.ReadConfig(bytes.NewReader([]byte(yamlContent)))
	require.NoError(t, err)

	var result config
	err = v.Unmarshal(&result, atmosDecodeHook())
	require.NoError(t, err)

	require.Len(t, result.Commands, 1)
	// Viper lowercases map keys before the decode hook sees them. LoadConfig restores
	// command env key case from CaseMaps after unmarshaling.
	assert.ElementsMatch(t, []schema.CommandEnv{
		{Key: "cgo_enabled", Value: "0"},
		{Key: "from_command", ValueCommand: "printf dynamic"},
		{Key: "gotoolchain", Value: "auto"},
	}, result.Commands[0].Env)
}

func TestCommandEnvFromMapEntryVariants(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   any
		want    schema.CommandEnv
		wantErr error
	}{
		{
			name:  "nil value preserves key",
			key:   "EMPTY",
			value: nil,
			want:  schema.CommandEnv{Key: "EMPTY"},
		},
		{
			name:  "numeric value is stringified",
			key:   "PORT",
			value: 8080,
			want:  schema.CommandEnv{Key: "PORT", Value: "8080"},
		},
		{
			name:  "map string any value command",
			key:   "DYNAMIC",
			value: map[string]any{"value_command": "printf ok"},
			want:  schema.CommandEnv{Key: "DYNAMIC", ValueCommand: "printf ok"},
		},
		{
			name:  "map any any with string keys",
			key:   "FROM_MAP",
			value: map[any]any{"value": true, "valueCommand": "printf ignored"},
			want:  schema.CommandEnv{Key: "FROM_MAP", Value: "true", ValueCommand: "printf ignored"},
		},
		{
			name:    "map any any rejects non-string key",
			key:     "BAD_KEY",
			value:   map[any]any{1: "value"},
			want:    schema.CommandEnv{Key: "BAD_KEY"},
			wantErr: errUnsupportedCommandEnvValueKey,
		},
		{
			name:    "unsupported value type",
			key:     "BAD_VALUE",
			value:   []string{"value"},
			want:    schema.CommandEnv{Key: "BAD_VALUE"},
			wantErr: errUnsupportedCommandEnvValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := commandEnvFromMapEntry(tt.key, tt.value)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeCommandArrayVariants(t *testing.T) {
	assert.Nil(t, normalizeCommandArray(nil))
	assert.Nil(t, normalizeCommandArray("invalid"))

	fromMaps := normalizeCommandArray([]map[string]interface{}{{"name": "mapped"}})
	require.Len(t, fromMaps, 1)
	name, ok := commandName(fromMaps[0])
	require.True(t, ok)
	assert.Equal(t, "mapped", name)

	fromSchema := normalizeCommandArray([]schema.Command{{Name: "typed"}})
	require.Len(t, fromSchema, 1)
	name, ok = commandName(fromSchema[0])
	require.True(t, ok)
	assert.Equal(t, "typed", name)

	name, ok = commandName(map[interface{}]interface{}{"name": "legacy"})
	require.True(t, ok)
	assert.Equal(t, "legacy", name)

	_, ok = commandName(schema.Command{})
	assert.False(t, ok)
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
