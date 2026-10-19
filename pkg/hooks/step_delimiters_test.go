package hooks

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/edition"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// Compile-time guards for the schema fields these tests set.
var (
	_ = schema.AtmosConfiguration{Edition: ""}
	_ = schema.ConfigAndStacksInfo{ComponentSection: nil}
)

func delimiterConfig(pin string, delims ...string) *schema.AtmosConfiguration {
	cfg := &schema.AtmosConfiguration{Edition: pin}
	cfg.Templates.Settings.Delimiters = delims
	return cfg
}

func TestExecContextStepDelimiters(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *schema.AtmosConfiguration
		want    delimiters.Delimiters
		wantErr error
	}{
		{name: "nil config", cfg: nil, want: delimiters.Default()},
		{name: "unpinned custom", cfg: delimiterConfig("", "[[", "]]"), want: delimiters.Delimiters{Left: "[[", Right: "]]"}},
		{name: "pinned before the transition", cfg: delimiterConfig("2026-10-18", "[[", "]]"), want: delimiters.Default()},
		{name: "pinned on the transition date", cfg: delimiterConfig("2026-10-19", "[[", "]]"), want: delimiters.Delimiters{Left: "[[", Right: "]]"}},
		{name: "invalid pin", cfg: delimiterConfig("soon", "[[", "]]"), wantErr: edition.ErrInvalidEdition},
		{name: "invalid delimiter list", cfg: delimiterConfig("", "[["), wantErr: errUtils.ErrInvalidTemplateSettings},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "log"})
			ctx.AtmosConfig = tt.cfg
			got, err := ctx.stepDelimiters()
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				_, again := ctx.stepDelimiters()
				require.ErrorIs(t, again, tt.wantErr, "the failure is cached for the invocation")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.want, stepVariables(ctx).TemplateDelimiters())
		})
	}
}

func TestProcessHookExecutionValueHonorsStepDelimiters(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"stage": "dev"}}
	const input = "[[ .stage ]]|{{ .stage }}"
	tests := []struct {
		name string
		cfg  *schema.AtmosConfiguration
		want string
	}{
		{name: "unpinned custom delimiters", cfg: delimiterConfig("", "[[", "]]"), want: "dev|{{ .stage }}"},
		{name: "pinned on the transition date", cfg: delimiterConfig("2026-10-19", "[[", "]]"), want: "dev|{{ .stage }}"},
		{name: "pinned before the transition keeps the default pair", cfg: delimiterConfig("2026-10-18", "[[", "]]"), want: "[[ .stage ]]|dev"},
		{name: "no delimiters configured", cfg: delimiterConfig(""), want: "[[ .stage ]]|dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := processHookExecutionValue(tt.cfg, input, info)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			// Keys and nested values render with the same pair.
			nested, err := processHookExecutionValue(tt.cfg, map[string]any{input: []any{input}}, info)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{tt.want: []any{tt.want}}, nested)
		})
	}
}

func TestProcessHookExecutionValueRejectsInvalidDelimiterConfig(t *testing.T) {
	_, err := processHookExecutionValue(delimiterConfig("soon", "[[", "]]"), "x", nil)
	require.ErrorIs(t, err, edition.ErrInvalidEdition)

	_, err = processHookExecutionValue(delimiterConfig("", "[["), "x", nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidTemplateSettings)

	// An invalid pair is only an error where it takes effect: an old pin never reads it.
	got, err := processHookExecutionValue(delimiterConfig("2026-10-18", "[["), "{{ .a }}", &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"a": "ok"}})
	require.NoError(t, err)
	assert.Equal(t, "ok", got)
}

func TestStepsEngineRendersStepsWithConfiguredDelimiters(t *testing.T) {
	const key = "HOOK_DELIMITER_VALUE"
	tests := []struct {
		name string
		cfg  *schema.AtmosConfiguration
		want string
	}{
		{name: "unpinned custom delimiters render [[ ]] only", cfg: delimiterConfig("", "[[", "]]"), want: "bar {{ .env." + key + " }}"},
		{name: "pinned on the transition date", cfg: delimiterConfig("2026-10-19", "[[", "]]"), want: "bar {{ .env." + key + " }}"},
		{name: "pinned before the transition renders {{ }} only", cfg: delimiterConfig("2026-10-18", "[[", "]]"), want: "[[ .env." + key + " ]] bar"},
		{name: "default delimiters", cfg: delimiterConfig(""), want: "[[ .env." + key + " ]] bar"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := []hookEnvState{}
			// The type is rendered as part of the payload, so it must not hold template text.
			handlerType := fmt.Sprintf("hook-delimiter-capture-%d", i)
			runnerstep.Register(&hookEnvCaptureHandler{
				BaseHandler: runnerstep.NewBaseHandler(handlerType, runnerstep.CategoryCommand, false),
				key:         key,
				captured:    &captured,
			})
			hook := &Hook{Kind: stepsKindName, With: []any{
				map[string]any{"type": "env", "vars": map[string]string{key: "bar"}},
				map[string]any{"type": handlerType, "content": "[[ .env." + key + " ]] {{ .env." + key + " }}"},
			}}
			ctx := stepsExecContext(hook)
			ctx.AtmosConfig = tt.cfg

			_, err := stepsEngine{}.Run(ctx)
			require.NoError(t, err)
			require.Len(t, captured, 1)
			assert.Equal(t, tt.want, captured[0].template)
		})
	}
}

func TestStepEngineRendersPayloadWithConfiguredDelimiters(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{Stack: "test-stack", ComponentSection: map[string]any{"stage": "dev"}}
	for _, tt := range []struct {
		name string
		cfg  *schema.AtmosConfiguration
		want string
	}{
		{name: "custom", cfg: delimiterConfig("", "[[", "]]"), want: "dev {{ .stage }}"},
		{name: "old pin", cfg: delimiterConfig("2026-10-18", "[[", "]]"), want: "[[ .stage ]] dev"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hook := &Hook{Kind: stepKindName, Type: "log", With: map[string]any{"content": "[[ .stage ]] {{ .stage }}"}}
			ctx := stepExecContext(hook)
			ctx.AtmosConfig = tt.cfg
			ctx.Info = info

			ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
			require.NoError(t, err)
			assert.Equal(t, tt.want, ws.Content)
		})
	}
}

func TestStepEnginesFailOnInvalidDelimiterConfig(t *testing.T) {
	hook := &Hook{Kind: stepKindName, Type: "log", With: map[string]any{"content": "x"}}
	ctx := stepExecContext(hook)
	ctx.AtmosConfig = delimiterConfig("", "[[")
	_, err := stepEngine{}.Run(ctx)
	require.ErrorIs(t, err, errUtils.ErrInvalidTemplateSettings)

	stepsHook := &Hook{Kind: stepsKindName, With: []any{map[string]any{"type": "log", "content": "x"}}}
	stepsCtx := stepsExecContext(stepsHook)
	stepsCtx.AtmosConfig = delimiterConfig("soon", "[[", "]]")
	_, err = stepsEngine{}.Run(stepsCtx)
	require.ErrorIs(t, err, edition.ErrInvalidEdition)
}
