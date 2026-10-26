package utils

import (
	"bytes"
	"os"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessTagEnvDefaultWithApostrophe(t *testing.T) {
	t.Setenv("APP_NAME", "")
	require.NoError(t, os.Unsetenv("APP_NAME"))
	data, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{},
		`app_name: !env 'APP_NAME "Bob''s app"'`, "env-example.yaml")
	require.NoError(t, err)
	encoded, ok := data["app_name"].(string)
	require.True(t, ok)
	source, ok := yamlsource.Decode(encoded)
	require.True(t, ok)
	value, err := ProcessTagEnv(source.Expression(), nil)
	require.NoError(t, err)
	assert.Equal(t, "Bob's app", value)
}

func TestProcessTagEnvWrapsParserError(t *testing.T) {
	_, err := ProcessTagEnv(`!env "unterminated`, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAtmosYAMLFunction)

	var parseErr *parser.Error
	require.ErrorAs(t, err, &parseErr)
	assert.Equal(t, "unterminated quoted value", parseErr.Message)
}

// captureEnvLogs routes the default logger to a buffer at warn level for the
// duration of the test and restores the previous level and output afterwards.
func captureEnvLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	prevLevel := log.GetLevel()
	log.SetLevel(charm.WarnLevel)
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(prevLevel)
	})
	return &buf
}

func TestProcessTagEnv_UnsetWithoutDefaultWarnsAndReturnsEmpty(t *testing.T) {
	const name = "ATMOS_TEST_ENV_FUNC_UNSET_VAR"
	require.NoError(t, os.Unsetenv(name))
	buf := captureEnvLogs(t)

	got, err := ProcessTagEnv("!env "+name, nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Contains(t, buf.String(), "environment variable is not set and has no default; using empty string")
	assert.Contains(t, buf.String(), name)
}

// Negative paths: a set variable, an explicit default, and a value from the
// component env section must not warn.
func TestProcessTagEnv_NoWarningWhenResolved(t *testing.T) {
	const name = "ATMOS_TEST_ENV_FUNC_RESOLVED_VAR"

	tests := []struct {
		name  string
		input string
		env   string
		ctx   EnvVarContext
		want  string
	}{
		{name: "set in OS environment", input: "!env " + name, env: "from-os", want: "from-os"},
		{name: "unset with default", input: "!env " + name + " fallback", want: "fallback"},
		{name: "from component env section", input: "!env " + name, ctx: testEnvContext{name: "from-stack"}, want: "from-stack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.Unsetenv(name))
			if tt.env != "" {
				t.Setenv(name, tt.env)
			}
			buf := captureEnvLogs(t)

			got, err := ProcessTagEnv(tt.input, tt.ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, buf.String(), "environment variable is not set")
		})
	}
}

// testEnvContext is a minimal EnvVarContext serving a single env section entry.
type testEnvContext struct {
	name string
}

func (c testEnvContext) GetComponentEnvSection() map[string]any {
	return map[string]any{"ATMOS_TEST_ENV_FUNC_RESOLVED_VAR": c.name}
}
