package duration

import (
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type hookTarget struct {
	Timeout time.Duration  `mapstructure:"timeout"`
	Delay   *time.Duration `mapstructure:"delay"`
	Name    string         `mapstructure:"name"`
}

func decodeWithHook(t *testing.T, input map[string]any) (hookTarget, error) {
	t.Helper()
	var out hookTarget
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &out,
		WeaklyTypedInput: true,
		DecodeHook:       DecodeHook(),
	})
	require.NoError(t, err)
	return out, decoder.Decode(input)
}

func TestDecodeHook(t *testing.T) {
	t.Run("go duration string", func(t *testing.T) {
		out, err := decodeWithHook(t, map[string]any{"timeout": "1h30m", "delay": "500ms"})
		require.NoError(t, err)
		assert.Equal(t, 90*time.Minute, out.Timeout)
		require.NotNil(t, out.Delay)
		assert.Equal(t, 500*time.Millisecond, *out.Delay)
	})
	t.Run("bare integer is seconds not nanoseconds", func(t *testing.T) {
		out, err := decodeWithHook(t, map[string]any{"timeout": 10, "delay": 2})
		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, out.Timeout)
		require.NotNil(t, out.Delay)
		assert.Equal(t, 2*time.Second, *out.Delay)
	})
	t.Run("numeric string is seconds", func(t *testing.T) {
		out, err := decodeWithHook(t, map[string]any{"timeout": "10"})
		require.NoError(t, err)
		assert.Equal(t, 10*time.Second, out.Timeout)
	})
	t.Run("float is seconds", func(t *testing.T) {
		out, err := decodeWithHook(t, map[string]any{"timeout": 1.5})
		require.NoError(t, err)
		assert.Equal(t, 1500*time.Millisecond, out.Timeout)
	})
	t.Run("days and keywords", func(t *testing.T) {
		out, err := decodeWithHook(t, map[string]any{"timeout": "2d", "delay": "hourly"})
		require.NoError(t, err)
		assert.Equal(t, 48*time.Hour, out.Timeout)
		assert.Equal(t, time.Hour, *out.Delay)
	})
	t.Run("zero allowed", func(t *testing.T) {
		out, err := decodeWithHook(t, map[string]any{"timeout": 0})
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), out.Timeout)
	})
	t.Run("negative rejected", func(t *testing.T) {
		_, err := decodeWithHook(t, map[string]any{"timeout": "-5s"})
		require.ErrorIs(t, err, ErrNegative)
	})
	t.Run("invalid string rejected", func(t *testing.T) {
		_, err := decodeWithHook(t, map[string]any{"timeout": "soon"})
		require.ErrorIs(t, err, ErrInvalid)
	})
	t.Run("non duration fields untouched", func(t *testing.T) {
		out, err := decodeWithHook(t, map[string]any{"name": "x"})
		require.NoError(t, err)
		assert.Equal(t, "x", out.Name)
		assert.Nil(t, out.Delay)
	})
}
