package concurrency

import (
	"os"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.Vendor{MaxConcurrency: 1}

func TestResolvePrecedence(t *testing.T) {
	tests := []struct {
		name    string
		config  *schema.AtmosConfiguration
		env     *string
		flag    string
		want    int
		invalid bool
	}{
		{name: "nil config uses latest default", want: 4},
		{name: "edition adjusted config remains serial", config: configWithWorkers(1), want: 1},
		{name: "explicit config", config: configWithWorkers(7), want: 7},
		{name: "environment overrides config", config: configWithWorkers(1), env: stringPointer("8"), want: 8},
		{name: "flag overrides environment and config", config: configWithWorkers(1), env: stringPointer("8"), flag: "12", want: 12},
		{name: "flag overrides invalid environment", config: configWithWorkers(1), env: stringPointer("invalid"), flag: "2", want: 2},
		{name: "zero config rejected", config: configWithWorkers(0), invalid: true},
		{name: "negative config rejected", config: configWithWorkers(-1), invalid: true},
		{name: "zero environment rejected", env: stringPointer("0"), invalid: true},
		{name: "negative environment rejected", env: stringPointer("-1"), invalid: true},
		{name: "empty environment rejected", env: stringPointer(""), invalid: true},
		{name: "malformed environment rejected", env: stringPointer("four"), invalid: true},
		{name: "fractional environment rejected", env: stringPointer("2.5"), invalid: true},
		{name: "overflow environment rejected", env: stringPointer("999999999999999999999999999"), invalid: true},
		{name: "zero flag rejected", env: stringPointer("4"), flag: "0", invalid: true},
		{name: "negative flag rejected", flag: "-2", invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(Env, "")
			require.NoError(t, os.Unsetenv(Env))
			if tt.env != nil {
				t.Setenv(Env, *tt.env)
			}
			flags := pflag.NewFlagSet("vendor", pflag.ContinueOnError)
			flags.Int(Flag, Default, "Worker limit")
			if tt.flag != "" {
				require.NoError(t, flags.Set(Flag, tt.flag))
			}
			got, err := Resolve(flags, tt.config)
			if tt.invalid {
				require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
				assert.Zero(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveWithoutFlags(t *testing.T) {
	t.Setenv(Env, "5")
	got, err := Resolve(nil, configWithWorkers(1))
	require.NoError(t, err)
	assert.Equal(t, 5, got)
}

func TestResolveRejectsIncorrectFlagType(t *testing.T) {
	flags := pflag.NewFlagSet("vendor", pflag.ContinueOnError)
	flags.String(Flag, "", "Worker limit")
	require.NoError(t, flags.Set(Flag, "8"))
	got, err := Resolve(flags, nil)
	require.Error(t, err)
	assert.Zero(t, got)
}

func TestEffective(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		explicit int
		want     int
		invalid  bool
	}{
		{name: "absent options use latest default", want: 4},
		{name: "zero value library config uses latest default", config: configWithWorkers(0), want: 4},
		{name: "loaded edition default preserved", config: configWithWorkers(1), want: 1},
		{name: "explicit options override loaded config", config: configWithWorkers(1), explicit: 8, want: 8},
		{name: "explicit options override invalid config", config: configWithWorkers(-1), explicit: 2, want: 2},
		{name: "negative explicit options rejected", explicit: -1, invalid: true},
		{name: "negative configuration rejected", config: configWithWorkers(-1), invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Effective(tt.config, tt.explicit)
			if tt.invalid {
				require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
				assert.Zero(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func configWithWorkers(workers int) *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{Vendor: schema.Vendor{MaxConcurrency: workers}}
}

func stringPointer(value string) *string { return &value }
