package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
)

var _ Logger = (*log.AtmosLogger)(nil)

func evalLog(source, prefix string, sink Logger) (starlark.Value, error) {
	members := Members("deploy", func(*starlark.Thread) string { return prefix }, func() Logger { return sink })
	return starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "log.star", source, starlark.StringDict{
		"log": &starlarkstruct.Module{Name: "log", Members: members},
	})
}

func TestMembersRouteStructuredFields(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"trace", "debug", "info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			sink := log.NewAtmosLogger(charm.NewWithOptions(&buf, charm.Options{Level: log.TraceLevel}))
			result, err := evalLog(`log.`+level+`("resolved", component="api", n=2, ratio=1.5, ok=True, none=None, items=[1,"a"], big=1<<80)`, "[outer] [inner[0]] ", sink)
			require.NoError(t, err)
			assert.Equal(t, starlark.None, result)
			text := buf.String()
			assert.Equal(t, 1, strings.Count(text, "\n"))
			assert.Contains(t, text, "resolved")
			assert.Contains(t, text, "step=deploy task=outer/inner[0] component=api n=2 ratio=1.5 ok=true none=<nil>")
			assert.Contains(t, text, `items="[1, \"a\"]"`)
			assert.Contains(t, text, "big=1208925819614629174706176")
		})
	}
}

func TestCallerFieldsOverrideContextAndLevelsFilter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	sink := log.NewAtmosLogger(charm.NewWithOptions(&buf, charm.Options{Level: log.InfoLevel}))
	_, err := evalLog(`log.debug("hidden")`, "", sink)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
	_, err = evalLog(`log.info("visible",step="custom",task="override")`, "[automatic] ", sink)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "step=custom task=override")
	assert.NotContains(t, buf.String(), "automatic")
	assert.Equal(t, 1, strings.Count(buf.String(), "step="))
}

func TestInvalidLogCallsWriteNothing(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`log.info()`, `log.info(1)`, `log.info("a","b")`, `log.info(message="a")`, `log.info("a",**{"bad field":1})`} {
		var buf bytes.Buffer
		sink := log.NewAtmosLogger(charm.NewWithOptions(&buf, charm.Options{Level: log.TraceLevel}))
		_, err := evalLog(source, "", sink)
		require.Error(t, err, source)
		assert.Empty(t, buf.String())
	}
}

func TestDefaultLoggerAndSecretMasking(t *testing.T) {
	var buf bytes.Buffer
	previous := log.Default()
	log.SetDefault(log.NewAtmosLogger(charm.NewWithOptions(&buf, charm.Options{Level: log.InfoLevel})))
	t.Cleanup(func() { log.SetDefault(previous) })
	iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
	t.Cleanup(iolib.Reset)
	const secret = "stdlib-log-secret-849b70"
	iolib.RegisterSecret(secret)
	require.NotEqual(t, secret, iolib.MaskString(secret))
	_, err := evalLog(`log.info("`+secret+`",token="`+secret+`",nested=["`+secret+`"])`, "", nil)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), secret)
	assert.Contains(t, buf.String(), "<MASKED>")
	assert.Contains(t, buf.String(), "step=deploy")
	assert.NotContains(t, buf.String(), "task=")
}

func TestNativeScalarSecretsAreMaskedBeforeLogging(t *testing.T) {
	// The masker is process-wide, so this test must not run in parallel.
	iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: false})
	t.Cleanup(iolib.Reset)
	secrets := []string{"739105", "8426.375", "true", "deploy", "scope-secret"}
	for _, secret := range secrets {
		iolib.RegisterSecret(secret)
	}
	for _, format := range []charm.Formatter{charm.TextFormatter, charm.JSONFormatter} {
		var buf bytes.Buffer
		sink := log.NewAtmosLogger(charm.NewWithOptions(&buf, charm.Options{Level: log.InfoLevel, Formatter: format}))
		_, err := evalLog(`log.info("scalars", pin=739105, ratio=8426.375, enabled=True, count=7)`, "[scope-secret] ", sink)
		require.NoError(t, err)
		for _, secret := range secrets {
			assert.NotContains(t, buf.String(), secret)
		}
		assert.Equal(t, len(secrets), strings.Count(buf.String(), "<MASKED>"))
		if format == charm.JSONFormatter {
			var fields map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &fields))
			assert.Equal(t, float64(7), fields["count"], "unmasked numbers retain their JSON type")
		} else {
			assert.Contains(t, buf.String(), "count=7")
		}
	}
	assert.Equal(t, int64(7), logValue(starlark.MakeInt(7)))
	assert.Equal(t, float64(1.5), logValue(starlark.Float(1.5)))
	assert.Equal(t, false, logValue(starlark.False))

	iolib.ApplyMaskingConfig(&iolib.Config{DisableMasking: true})
	assert.Equal(t, int64(739105), logValue(starlark.MakeInt(739105)))
	assert.Equal(t, float64(8426.375), logValue(starlark.Float(8426.375)))
	assert.Equal(t, true, logValue(starlark.True))
}
