package schema

import (
	"errors"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfigWithCommandEnv struct {
	Env []CommandEnv `mapstructure:"env"`
}

func TestCommandEnvDecodeHook_MapValues(t *testing.T) {
	input := map[string]any{
		"env": map[string]any{
			"PATH":         "{{ env \"PWD\" }}/bin:{{ env \"PATH\" }}",
			"GOBIN":        "{{ env \"PWD\" }}/bin",
			"FROM_COMMAND": map[string]any{"valueCommand": "printf value"},
		},
	}

	var result testConfigWithCommandEnv
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &result,
		WeaklyTypedInput: true,
		DecodeHook:       CommandEnvDecodeHook(),
	})
	require.NoError(t, err)

	err = decoder.Decode(input)
	require.NoError(t, err)

	require.Len(t, result.Env, 3)
	assert.Equal(t, CommandEnv{Key: "FROM_COMMAND", ValueCommand: "printf value"}, result.Env[0])
	assert.Equal(t, CommandEnv{Key: "GOBIN", Value: "{{ env \"PWD\" }}/bin"}, result.Env[1])
	assert.Equal(t, CommandEnv{Key: "PATH", Value: "{{ env \"PWD\" }}/bin:{{ env \"PATH\" }}"}, result.Env[2])
}

func TestDecodeCommandEnvMapValueDecodeErrorUsesSentinel(t *testing.T) {
	_, err := decodeCommandEnvMapValue("BROKEN", map[string]any{"value": []string{"not", "a", "string"}})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCommandEnvDecodeFailed))
}
