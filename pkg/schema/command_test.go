package schema

import (
	"errors"
	"reflect"
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

// TestCommandEnvDecodeError_Is verifies the custom Is method on commandEnvDecodeError:
// it matches any target whose Error() string equals the sentinel message (which is how
// ErrCommandEnvDecodeFailed compares equal to itself and to fmt.Errorf-wrapped copies
// carrying the same message), and rejects both nil and non-matching targets.
func TestCommandEnvDecodeError_Is(t *testing.T) {
	sentinel := commandEnvDecodeError{}

	assert.True(t, sentinel.Is(ErrCommandEnvDecodeFailed))
	assert.True(t, sentinel.Is(errors.New(commandEnvDecodeFailedMessage)))
	assert.False(t, sentinel.Is(nil))
	assert.False(t, sentinel.Is(errors.New("some other error")))
	assert.True(t, errors.Is(ErrCommandEnvDecodeFailed, ErrCommandEnvDecodeFailed))
}

// TestCommandEnvDecodeHook_IgnoresWrongTargetOrSourceKind verifies the hook's early-out
// guards: it only converts data when the target type is []CommandEnv, the source Kind
// is Map, and the map is a stringifiable map[string]any.
func TestCommandEnvDecodeHook_IgnoresWrongTargetOrSourceKind(t *testing.T) {
	hook := CommandEnvDecodeHook().(func(reflect.Type, reflect.Type, any) (any, error))

	// Wrong target type: passthrough regardless of source kind.
	out, err := hook(reflect.TypeOf(map[string]any{}), reflect.TypeOf(""), map[string]any{"KEY": "value"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"KEY": "value"}, out)

	// Correct target type but source is not a Map kind.
	out, err = hook(reflect.TypeOf(""), reflect.TypeOf([]CommandEnv{}), "not-a-map")
	require.NoError(t, err)
	assert.Equal(t, "not-a-map", out)

	// Correct target type, source Kind is Map, but not a map[string]any (e.g. map[int]any).
	badMap := map[int]any{1: "x"}
	out, err = hook(reflect.TypeOf(badMap), reflect.TypeOf([]CommandEnv{}), badMap)
	require.NoError(t, err)
	assert.Equal(t, badMap, out)
}

// TestDecodeCommandEnvMapValue_UnexpectedKind verifies the default branch of
// decodeCommandEnvMapValue returns ErrTaskUnexpectedNodeKind for values that are
// neither a string nor a map[string]any.
func TestDecodeCommandEnvMapValue_UnexpectedKind(t *testing.T) {
	_, err := decodeCommandEnvMapValue("BAD_KEY", 42)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTaskUnexpectedNodeKind))
	assert.Contains(t, err.Error(), "BAD_KEY")
}
