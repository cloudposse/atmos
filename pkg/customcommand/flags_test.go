package customcommand

import (
	"math"
	"strings"
	"testing"

	cerrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEffectiveFlagType(t *testing.T) {
	assert.Equal(t, FlagTypeString, EffectiveFlagType(&schema.CommandFlag{}))
	assert.Equal(t, FlagTypeString, EffectiveFlagType(&schema.CommandFlag{Type: "string"}))
	assert.Equal(t, FlagTypeBool, EffectiveFlagType(&schema.CommandFlag{Type: "bool"}))
	assert.Equal(t, FlagTypeInt, EffectiveFlagType(&schema.CommandFlag{Type: "int"}))
}

func TestValidateFlag(t *testing.T) {
	tests := []struct {
		name    string
		flag    schema.CommandFlag
		wantErr error
	}{
		{name: "default type", flag: schema.CommandFlag{Name: "a"}},
		{name: "string", flag: schema.CommandFlag{Name: "a", Type: "string"}},
		{name: "bool", flag: schema.CommandFlag{Name: "a", Type: "bool", Default: true}},
		{name: "int with integer default", flag: schema.CommandFlag{Name: "a", Type: "int", Default: 3}},
		{name: "int with no default", flag: schema.CommandFlag{Name: "a", Type: "int"}},
		{name: "string with values", flag: schema.CommandFlag{Name: "a", Values: []string{"x", "y"}}},
		{name: "unknown type", flag: schema.CommandFlag{Name: "a", Type: "float"}, wantErr: errUtils.ErrCustomCommandFlagType},
		{name: "types are case sensitive", flag: schema.CommandFlag{Name: "a", Type: "Int"}, wantErr: errUtils.ErrCustomCommandFlagType},
		{name: "int with a non-numeric default", flag: schema.CommandFlag{Name: "a", Type: "int", Default: "many"}, wantErr: errUtils.ErrCustomCommandFlagDefault},
		{name: "int with a fractional default", flag: schema.CommandFlag{Name: "a", Type: "int", Default: 2.5}, wantErr: errUtils.ErrCustomCommandFlagDefault},
		{name: "int with a boolean default", flag: schema.CommandFlag{Name: "a", Type: "int", Default: true}, wantErr: errUtils.ErrCustomCommandFlagDefault},
		{name: "string with a string default", flag: schema.CommandFlag{Name: "a", Default: "2"}},
		{name: "string with an unquoted integer default", flag: schema.CommandFlag{Name: "a", Default: 2}, wantErr: errUtils.ErrCustomCommandFlagDefault},
		{name: "string with a boolean default", flag: schema.CommandFlag{Name: "a", Type: "string", Default: true}, wantErr: errUtils.ErrCustomCommandFlagDefault},
		{name: "bool with a quoted default", flag: schema.CommandFlag{Name: "a", Type: "bool", Default: "true"}, wantErr: errUtils.ErrCustomCommandFlagDefault},
		{name: "bool with an integer default", flag: schema.CommandFlag{Name: "a", Type: "bool", Default: 1}, wantErr: errUtils.ErrCustomCommandFlagDefault},
		{name: "int with values", flag: schema.CommandFlag{Name: "a", Type: "int", Values: []string{"1"}}, wantErr: errUtils.ErrCustomCommandFlagType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFlag("deploy", &tt.flag)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateFlagListsSupportedTypes(t *testing.T) {
	err := ValidateFlag("deploy", &schema.CommandFlag{Name: "ratio", Type: "float"})
	require.ErrorIs(t, err, errUtils.ErrCustomCommandFlagType)

	assert.Contains(t, strings.Join(cerrors.GetAllHints(err), "\n"), "string, bool, int")
}

func TestIntFlagDefault(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    int
		wantErr bool
	}{
		{name: "unset", value: nil, want: 0},
		{name: "int", value: 3, want: 3},
		{name: "int64", value: int64(4), want: 4},
		{name: "uint64", value: uint64(5), want: 5},
		{name: "whole float", value: float64(6), want: 6},
		{name: "numeric string", value: " 7 ", want: 7},
		{name: "empty string", value: "", want: 0},
		{name: "negative", value: -2, want: -2},
		{name: "fractional float", value: 1.5, wantErr: true},
		{name: "uint64 beyond int range", value: uint64(math.MaxUint64), wantErr: true},
		{name: "large whole float", value: float64(1 << 30), want: 1 << 30},
		{name: "smallest int as float", value: float64(math.MinInt), want: math.MinInt},
		{name: "float far above int range", value: 1e300, wantErr: true},
		{name: "float far below int range", value: -1e300, wantErr: true},
		{name: "float at 2^63", value: 9.223372036854775808e18, wantErr: true},
		{name: "float at -2^63 minus one step", value: math.Nextafter(float64(math.MinInt), math.Inf(-1)), wantErr: true},
		{name: "positive infinity", value: math.Inf(1), wantErr: true},
		{name: "negative infinity", value: math.Inf(-1), wantErr: true},
		{name: "not a number", value: math.NaN(), wantErr: true},
		{name: "text", value: "x", wantErr: true},
		{name: "bool", value: false, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IntFlagDefault(&schema.CommandFlag{Name: "count", Default: tt.value})
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrCustomCommandFlagDefault)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseIntFlagValue(t *testing.T) {
	got, err := ParseIntFlagValue("count", "42")
	require.NoError(t, err)
	assert.Equal(t, 42, got)

	_, err = ParseIntFlagValue("count", "forty-two")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
}

func TestValidateFlagNamesTheFlagWithAWrongTypedDefault(t *testing.T) {
	err := ValidateFlag("deploy", &schema.CommandFlag{Name: "replicas", Default: 2})
	require.ErrorIs(t, err, errUtils.ErrCustomCommandFlagDefault)
	assert.Contains(t, err.Error(), "replicas")
	assert.Contains(t, cerrors.GetAllHints(err)[0], "default:")
}
