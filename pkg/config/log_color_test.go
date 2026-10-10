package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	ckerrors "github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.Logs{Color: new(true)}

// setArgs replaces os.Args for the duration of the test.
func setArgs(t *testing.T, args ...string) {
	t.Helper()
	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = append([]string{"atmos"}, args...)
}

// restoreLogger restores the default logger after a test that changes its color.
func restoreLogger(t *testing.T) {
	t.Helper()
	oldLogger := log.Default()
	t.Cleanup(func() { log.SetDefault(oldLogger) })
	log.SetDefault(log.New())
}

// newColorViper mirrors the Viper setup LoadConfig uses for logs.color.
func newColorViper(t *testing.T, yaml string) *viper.Viper {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetTypeByDefaultValue(true)
	v.SetDefault("logs.color", true)
	require.NoError(t, v.ReadConfig(bytes.NewBufferString(yaml)))
	return v
}

func TestLoggingColorConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name string
		yaml string
		env  string
		flag string
		args []string
		want bool
	}{
		{"default", "{}", "", "", nil, true},
		{"YAML boolean false", "logs: {color: false}", "", "", nil, false},
		{"YAML boolean true", "logs: {color: true}", "", "", nil, true},
		{"YAML string", "logs: {color: 'false'}", "", "", nil, false},
		{"YAML string TRUE", "logs: {color: 'TRUE'}", "", "", nil, true},
		{"YAML integer 0", "logs: {color: 0}", "", "", nil, false},
		{"YAML integer 1", "logs: {color: 1}", "", "", nil, true},
		{"YAML null keeps default", "logs: {color: }", "", "", nil, true},
		{"environment true", "{}", "true", "", nil, true},
		{"environment false", "{}", "false", "", nil, false},
		{"environment 1", "logs: {color: false}", "1", "", nil, true},
		{"environment 0", "{}", "0", "", nil, false},
		{"environment TRUE", "logs: {color: false}", "TRUE", "", nil, true},
		{"environment beats config", "logs: {color: false}", "true", "", nil, true},
		{"CLI argument false", "{}", "", "", []string{"--logs-color=false"}, false},
		{"flag value true", "logs: {color: false}", "", "true", nil, true},
		{"flag value 0", "{}", "", "0", nil, false},
		{"flag value TRUE", "logs: {color: false}", "", "TRUE", nil, true},
		{"flag beats environment", "logs: {color: true}", "true", "", []string{"--logs-color=false"}, false},
		{"flag value beats environment", "logs: {color: true}", "true", "false", nil, false},
		{"bare flag beats environment", "logs: {color: false}", "false", "", []string{"--logs-color", "version"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setArgs(t, tt.args...)
			restoreLogger(t)
			t.Setenv("ATMOS_LOGS_COLOR", tt.env)
			v := newColorViper(t, tt.yaml)
			require.NoError(t, validateLogsColorConfig(v))
			var cfg schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&cfg, atmosDecodeHook()))
			require.NoError(t, setLoggingColor(&cfg, tt.flag))
			require.NotNil(t, cfg.Logs.Color)
			assert.Equal(t, tt.want, *cfg.Logs.Color)
		})
	}
}

func TestLoggingColorInvalidValue(t *testing.T) {
	for _, tt := range []struct {
		name       string
		env        string
		flag       string
		args       []string
		wantSource string
		wantValue  string
		wantHint   string
	}{
		{"flag value", "", "auto", nil, "--logs-color", "auto", "--logs-color=true"},
		{"flag value beats valid environment", "true", "off", nil, "--logs-color", "off", "--logs-color=true"},
		{"environment off", "off", "", nil, "ATMOS_LOGS_COLOR", "off", "ATMOS_LOGS_COLOR=true"},
		{"environment yes", "yes", "", nil, "ATMOS_LOGS_COLOR", "yes", "ATMOS_LOGS_COLOR=false"},
		{"environment unrelated CLI flag", "nope", "", []string{"version"}, "ATMOS_LOGS_COLOR", "nope", "unset it"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setArgs(t, tt.args...)
			restoreLogger(t)
			t.Setenv("ATMOS_LOGS_COLOR", tt.env)
			err := setLoggingColor(&schema.AtmosConfiguration{}, tt.flag)
			require.Error(t, err)
			require.ErrorIs(t, err, errUtils.ErrInvalidLogsColor)
			assertColorErrorDetail(t, err, tt.wantSource, tt.wantValue, tt.wantHint)
		})
	}
}

// TestLoggingColorInvalidEnvironmentOverriddenByValidFlag checks that a valid CLI flag still wins over a bad
// environment variable, so the precedence is unchanged by the validation.
func TestLoggingColorInvalidEnvironmentOverriddenByValidFlag(t *testing.T) {
	setArgs(t, "--logs-color=false")
	restoreLogger(t)
	t.Setenv("ATMOS_LOGS_COLOR", "off")
	cfg := schema.AtmosConfiguration{}
	require.NoError(t, setLoggingColor(&cfg, ""))
	require.NotNil(t, cfg.Logs.Color)
	assert.False(t, *cfg.Logs.Color)
}

func TestValidateLogsColorConfig(t *testing.T) {
	t.Run("no logs map keeps default", func(t *testing.T) {
		require.NoError(t, validateLogsColorConfig(viper.New()))
	})
	// Prerequisite: Viper coerces the invalid value to false, which is why raw validation is needed.
	t.Run("prerequisite viper coerces invalid value to false", func(t *testing.T) {
		v := newColorViper(t, "logs: {color: nope}")
		assert.Equal(t, false, v.Get("logs.color"))
	})

	for _, tt := range []struct {
		name      string
		yaml      string
		invalid   bool
		wantValue string // The value the error is expected to quote.
	}{
		{"absent", "{}", false, ""},
		{"logs without color", "logs: {level: Info}", false, ""},
		{"null", "logs: {color: }", false, ""},
		{"true", "logs: {color: true}", false, ""},
		{"false", "logs: {color: false}", false, ""},
		{"string false", "logs: {color: 'false'}", false, ""},
		{"string TRUE", "logs: {color: 'TRUE'}", false, ""},
		{"integer 1", "logs: {color: 1}", false, ""},
		{"integer 0", "logs: {color: 0}", false, ""},
		{"nope", "logs: {color: nope}", true, "nope"},
		{"off", "logs: {color: off}", true, "off"},
		{"yes", "logs: {color: yes}", true, "yes"},
		{"integer 2", "logs: {color: 2}", true, "2"},
		{"empty string", "logs: {color: ''}", true, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLogsColorConfig(newColorViper(t, tt.yaml))
			if !tt.invalid {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrInvalidLogsColor)
			assertColorErrorDetail(t, err, "logs.color", tt.wantValue, "`logs.color`")
		})
	}

	t.Run("list value", func(t *testing.T) {
		err := validateLogsColorConfig(newColorViper(t, "logs: {color: [true]}"))
		require.ErrorIs(t, err, errUtils.ErrInvalidLogsColor)
		assertColorErrorDetail(t, err, "logs.color", "[true]", "`logs.color`")
	})
}

// TestLoggingConfigRejectsInvalidColor checks that the enclosing logging setup preserves
// the actionable color error and stops before applying unrelated logging overrides.
func TestLoggingConfigRejectsInvalidColor(t *testing.T) {
	setArgs(t)
	restoreLogger(t)
	t.Setenv("ATMOS_LOGS_COLOR", "auto")
	cfg := schema.AtmosConfiguration{Logs: schema.Logs{Level: "Warning"}}
	err := setLoggingConfig(&cfg, &schema.ConfigAndStacksInfo{LogsLevel: "Debug"})
	require.ErrorIs(t, err, errUtils.ErrInvalidLogsColor)
	assertColorErrorDetail(t, err, "ATMOS_LOGS_COLOR", "auto", "unset it")
	assert.Equal(t, "Warning", cfg.Logs.Level)
}

// TestInitCliConfigRejectsInvalidLogsColor verifies the full load path reports a bad logs.color
// instead of silently turning it into false, and still accepts YAML booleans.
func TestInitCliConfigRejectsInvalidLogsColor(t *testing.T) {
	for _, tt := range []struct {
		name      string
		yaml      string
		want      bool
		wantValue string // Non-empty means an error naming this value is expected.
	}{
		{"absent defaults to true", "", true, ""},
		{"true", "  color: true\n", true, ""},
		{"false", "  color: false\n", false, ""},
		{"nope", "  color: nope\n", false, "nope"},
		{"off", "  color: off\n", false, "off"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			content := "base_path: ./\nlogs:\n  file: /dev/stderr\n  level: Info\n" + tt.yaml
			require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(content), 0o600))
			t.Chdir(dir)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
			t.Setenv("ATMOS_LOGS_COLOR", "")
			setArgs(t)
			restoreLogger(t)

			atmosConfig, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
			if tt.wantValue != "" {
				require.ErrorIs(t, err, errUtils.ErrInvalidLogsColor)
				assertColorErrorDetail(t, err, "logs.color", tt.wantValue, "`logs.color`")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, atmosConfig.Logs.Color)
			assert.Equal(t, tt.want, *atmosConfig.Logs.Color)
		})
	}
}

// assertColorErrorDetail checks that the error explanation names the source and value, and a hint mentions wantHint.
func assertColorErrorDetail(t *testing.T, err error, source, value, wantHint string) {
	t.Helper()
	details := ckerrors.GetAllDetails(err)
	require.Len(t, details, 1)
	assert.Contains(t, details[0], source+" must be true or false")
	assert.Contains(t, details[0], `got "`+value+`"`)
	hints := ckerrors.GetAllHints(err)
	require.Len(t, hints, 1)
	assert.Contains(t, hints[0], wantHint)
}
