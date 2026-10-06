package hooks

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.GitHookEntry{Steps: schema.Tasks{}}

func TestRunEmbeddedSteps(t *testing.T) {
	t.Parallel()
	var cfg schema.GitConfig
	require.NoError(t, yaml.Unmarshal([]byte(`hooks:
  commit-msg:
    steps:
      - name: first
        type: script
        interpreter: starlark
        script: |
          print(ctx.args)
          output = steps.join(options=["hello", "world"], separator=" ").value
        outputs:
          greeting: '{{ .value }}'
      - name: second
        type: script
        interpreter: starlark
        env:
          MESSAGE: '{{ .steps.first.outputs.greeting }}'
        script: print(env["MESSAGE"])
`), &cfg))
	var stdout, stderr bytes.Buffer
	err := Run(&cfg, "commit-msg", []string{"a file", "--flag", "{{ literal }}"}, WithAtmosConfig(&schema.AtmosConfiguration{}), WithOutputWriters(&stdout, &stderr))
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), `["a file", "--flag", "{{ literal }}"]`)
	assert.Contains(t, stdout.String(), "hello world")
}

func TestRunStepValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		entry schema.GitHookEntry
	}{
		{"empty", schema.GitHookEntry{}},
		{"both", schema.GitHookEntry{Command: "unused", Steps: schema.Tasks{{Type: "join"}}}},
		{"unknown", schema.GitHookEntry{Steps: schema.Tasks{{Type: "not-a-step"}}}},
		{"duplicate", schema.GitHookEntry{Steps: schema.Tasks{{Name: "same", Type: "join"}, {Name: "same", Type: "join"}}}},
		{"scheduler", schema.GitHookEntry{Steps: schema.Tasks{{Type: "join", Needs: []string{"other"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			err := Run(&schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": tc.entry}}, "pre-commit", nil, WithOutputWriters(&stdout, &stdout))
			require.Error(t, err)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestRunStepFailureAndCancellation(t *testing.T) {
	t.Parallel()
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{
		{Name: "failure", Type: "script", Interpreter: "starlark", Script: `errors.build("invalid repository").with_hint("fix it").fail()`},
		{Name: "later", Type: "script", Interpreter: "starlark", Script: `print("must not run")`},
	}}}}
	var stdout bytes.Buffer
	err := Run(cfg, "pre-commit", nil, WithOutputWriters(&stdout, &stdout))
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Contains(t, err.Error(), "failure")
	assert.NotContains(t, stdout.String(), "must not run")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = Run(cfg, "pre-commit", nil, WithContext(ctx), WithOutputWriters(&stdout, &stdout))
	require.ErrorIs(t, err, context.Canceled)
}

func TestRunStepPreflightAndTimeout(t *testing.T) {
	t.Parallel()
	var stdout bytes.Buffer
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{
		{Type: "script", Interpreter: "starlark", Script: `print("must not run")`},
		{Type: "unknown"},
	}}}}
	require.Error(t, Run(cfg, "pre-commit", nil, WithOutputWriters(&stdout, &stdout)))
	assert.Empty(t, stdout.String(), "preflight must validate the whole sequence")
	cfg.Hooks["pre-commit"] = schema.GitHookEntry{Steps: schema.Tasks{{Name: "bounded", Type: "script", Interpreter: "starlark", Timeout: time.Millisecond, Script: "for i in range(1000000000):\n    pass"}}}
	err := Run(cfg, "pre-commit", nil, WithOutputWriters(&stdout, &stdout))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "bounded")
}
