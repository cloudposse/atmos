package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProErrorConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, enabled, report string
		wantEnabled                 bool
		wantReport                  *bool
	}{
		{name: "omitted follows Pro", yaml: "settings:\n  pro:\n    enabled: true\n", wantEnabled: true},
		{name: "explicit opt out", yaml: "settings:\n  pro:\n    enabled: true\n    errors:\n      enabled: false\n", wantEnabled: true, wantReport: boolValue(false)},
		{name: "environment opt out", yaml: "settings:\n  pro:\n    errors:\n      enabled: true\n", enabled: "true", report: "false", wantEnabled: true, wantReport: boolValue(false)},
		{name: "environment only", yaml: "{}", enabled: "true", report: "true", wantEnabled: true, wantReport: boolValue(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATMOS_PRO_ENABLED", tc.enabled)
			t.Setenv("ATMOS_PRO_ERRORS_ENABLED", tc.report)
			v := viper.New()
			v.SetConfigType("yaml")
			setEnv(v)
			require.NoError(t, v.ReadConfig(strings.NewReader(tc.yaml)))
			var config schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&config))
			assert.Equal(t, tc.wantEnabled, config.Settings.Pro.Enabled)
			assert.Equal(t, tc.wantReport, config.Settings.Pro.Errors.Enabled)
		})
	}
}

func boolValue(value bool) *bool { return &value }
