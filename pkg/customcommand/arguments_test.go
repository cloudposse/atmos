package customcommand

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveArguments(t *testing.T) {
	declared := []schema.CommandArgument{
		{Name: "service", Required: true},
		{Name: "region"},
		{Name: "zone", Default: "z,1"},
	}

	tests := []struct {
		name     string
		declared []schema.CommandArgument
		supplied []string
		want     []string
		wantErr  error
	}{
		{
			name:     "all supplied",
			declared: declared,
			supplied: []string{"api", "us-east-1", "b"},
			want:     []string{"api", "us-east-1", "b"},
		},
		{
			name:     "optional argument without default resolves to an empty string",
			declared: declared,
			supplied: []string{"api"},
			want:     []string{"api", "", "z,1"},
		},
		{
			name:     "defaults apply to omitted trailing arguments",
			declared: declared,
			supplied: []string{"api", "eu"},
			want:     []string{"api", "eu", "z,1"},
		},
		{
			name:     "an empty supplied value is kept, not replaced by the default",
			declared: declared,
			supplied: []string{"api", "", ""},
			want:     []string{"api", "", ""},
		},
		{
			name:     "required argument with no default and no value fails",
			declared: declared,
			supplied: nil,
			wantErr:  errUtils.ErrCustomCommandArgumentMissing,
		},
		{
			name:     "no declared arguments",
			declared: nil,
			supplied: []string{"ignored"},
			want:     []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveArguments(tt.declared, tt.supplied)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestArgumentsRoundTripIsLossless(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "commas", args: []string{"api,v2", "us-east-1"}},
		{name: "empty string argument", args: []string{"", "x", ""}},
		{name: "unicode and quotes", args: []string{"ünï,cødé", `say "hi"`, "日本語,テスト"}},
		{name: "single empty argument", args: []string{""}},
		{name: "no arguments", args: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeArguments(tt.args)
			require.NoError(t, err)

			decoded, ok, err := DecodeArguments(encoded)
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, tt.args, decoded)
			if len(tt.args) > 0 {
				assert.Equal(t, tt.args[0], decoded[0])
				assert.Equal(t, tt.args[len(tt.args)-1], decoded[len(decoded)-1])
			}
		})
	}
}

func TestDecodeArguments(t *testing.T) {
	t.Run("an empty annotation means none were recorded", func(t *testing.T) {
		args, ok, err := DecodeArguments("")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Nil(t, args)
	})

	t.Run("a corrupt annotation is an error", func(t *testing.T) {
		_, ok, err := DecodeArguments("not-json")
		require.ErrorIs(t, err, errUtils.ErrFailedToProcessArgs)
		assert.False(t, ok)
	})

	t.Run("a nil slice encodes as an empty list", func(t *testing.T) {
		encoded, err := EncodeArguments(nil)
		require.NoError(t, err)
		assert.Equal(t, "[]", encoded)
	})
}
