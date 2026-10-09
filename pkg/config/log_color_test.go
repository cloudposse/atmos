package config

import (
	"bytes"
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.Logs{Color: new(true)}

func TestLoggingColorConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name string
		yaml string
		env  string
		args []string
		want bool
	}{
		{"default", "{}", "", nil, true},
		{"YAML boolean false", "logs: {color: false}", "", nil, false},
		{"YAML boolean true", "logs: {color: true}", "", nil, true},
		{"YAML string", "logs: {color: 'false'}", "", nil, false},
		{"environment beats config", "logs: {color: false}", "true", nil, true},
		{"flag beats environment", "logs: {color: true}", "true", []string{"--logs-color=false"}, false},
		{"bare flag beats environment", "logs: {color: false}", "false", []string{"--logs-color", "version"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs, oldLogger := os.Args, log.Default()
			t.Cleanup(func() { os.Args = oldArgs; log.SetDefault(oldLogger) })
			os.Args = append([]string{"atmos"}, tt.args...)
			log.SetDefault(log.New())
			t.Setenv("ATMOS_LOGS_COLOR", tt.env)
			v := viper.New()
			v.SetConfigType("yaml")
			require.NoError(t, v.ReadConfig(bytes.NewBufferString(tt.yaml)))
			var cfg schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&cfg, atmosDecodeHook()))
			require.NoError(t, setLoggingColor(&cfg, ""))
			assert.Equal(t, tt.want, *cfg.Logs.Color)
		})
	}
}

func TestLoggingColorInvalidValue(t *testing.T) {
	t.Setenv("ATMOS_LOGS_COLOR", "")
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"atmos"}
	cfg := schema.AtmosConfiguration{}
	err := setLoggingColor(&cfg, "auto")
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	assert.ErrorContains(t, err, "logs.color must be true or false")
}
