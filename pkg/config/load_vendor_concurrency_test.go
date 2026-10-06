package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/concurrency"
)

var _ = schema.Vendor{MaxConcurrency: 1}

func TestLoadConfigVendorConcurrencyEdition(t *testing.T) {
	tests := []struct {
		name, yaml, profile, env, flag string
		noConfig                       bool
		want                           int
	}{
		{name: "latest", want: 4},
		{name: "before change", yaml: "edition: '2026-09-14'\n", want: 1},
		{name: "change date", yaml: "edition: '2026-09-15'\n", want: 4},
		{name: "after change", yaml: "edition: '2026-09-16'\n", want: 4},
		{name: "previous month", yaml: "edition: '2026-08'\n", want: 1},
		{name: "change month includes release", yaml: "edition: '2026-09'\n", want: 4},
		{name: "change year includes release", yaml: "edition: '2026'\n", want: 4},
		{name: "explicit config overrides old edition", yaml: "edition: '2026-08'\nvendor:\n  max_concurrency: 6\n", want: 6},
		{name: "profile sets old edition", profile: "edition: '2026-08'\n", want: 1},
		{name: "profile overrides workers", yaml: "edition: '2026-08'\nvendor:\n  max_concurrency: 2\n", profile: "vendor:\n  max_concurrency: 7\n", want: 7},
		{name: "environment overrides profile", yaml: "edition: '2026-08'\n", profile: "vendor:\n  max_concurrency: 7\n", env: "8", want: 8},
		{name: "flag overrides environment", yaml: "edition: '2026-08'\n", profile: "vendor:\n  max_concurrency: 7\n", env: "8", flag: "9", want: 9},
		{name: "no config uses latest", noConfig: true, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeEditionTestConfig(t, "base_path: ./\nprofiles:\n  base_path: profiles\n"+tt.yaml)
			viper.Reset()
			t.Cleanup(viper.Reset)
			t.Setenv("ATMOS_PROFILE", "")
			t.Setenv(concurrency.Env, "")
			require.NoError(t, os.Unsetenv(concurrency.Env))
			if tt.env != "" {
				t.Setenv(concurrency.Env, tt.env)
			}
			if tt.noConfig {
				require.NoError(t, os.Remove(AtmosConfigFileName))
			}
			info := &schema.ConfigAndStacksInfo{}
			if tt.profile != "" {
				dir := filepath.Join("profiles", "vendor")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, AtmosConfigFileName), []byte(tt.profile), 0o644))
				info.ProfilesFromArg = []string{"vendor"}
			}
			config, err := LoadConfig(info)
			require.NoError(t, err)
			flags := pflag.NewFlagSet("vendor", pflag.ContinueOnError)
			flags.Int(concurrency.Flag, concurrency.Default, "Worker limit")
			if tt.flag != "" {
				require.NoError(t, flags.Set(concurrency.Flag, tt.flag))
			}
			got, err := concurrency.Resolve(flags, &config)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			if tt.env == "" && tt.flag == "" {
				assert.Equal(t, tt.want, config.Vendor.MaxConcurrency)
			}
		})
	}
}

func TestLoadConfigVendorConcurrencyNoConfigEditionEnv(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\n")
	require.NoError(t, os.Remove(AtmosConfigFileName))
	t.Setenv("ATMOS_EDITION", "2026-09-14")
	config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Equal(t, 1, config.Vendor.MaxConcurrency, "fallback defaults must not shadow edition rollback")
}

func TestLoadConfigVendorConcurrencyInvalid(t *testing.T) {
	for _, value := range []string{"0", "-1", "four"} {
		t.Run(value, func(t *testing.T) {
			writeEditionTestConfig(t, "base_path: ./\nvendor:\n  max_concurrency: "+value+"\n")
			t.Setenv(concurrency.Env, "")
			require.NoError(t, os.Unsetenv(concurrency.Env))
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)
			_, err = concurrency.Resolve(nil, &config)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
		})
	}
}
