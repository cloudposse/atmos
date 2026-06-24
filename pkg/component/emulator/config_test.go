package emulator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestDefaultConfig(t *testing.T) {
	assert.Equal(t, Config{BasePath: defaultBasePath}, DefaultConfig())
	assert.Equal(t, "components/emulator", DefaultConfig().BasePath)
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want Config
	}{
		{
			name: "explicit base_path",
			raw:  map[string]any{"base_path": "custom/emulators"},
			want: Config{BasePath: "custom/emulators"},
		},
		{
			name: "empty map yields zero config",
			raw:  map[string]any{},
			want: Config{},
		},
		{
			name: "nil yields zero config",
			raw:  nil,
			want: Config{},
		},
		{
			name: "weakly-typed input is coerced",
			raw:  map[string]any{"base_path": 42},
			want: Config{BasePath: "42"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConfig(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseConfig_DecodeError(t *testing.T) {
	// A scalar (non-map) value cannot decode into a struct.
	_, err := parseConfig("not-a-struct")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrComponentConfigInvalid)
}
