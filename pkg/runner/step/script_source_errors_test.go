package step

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMaterializeScriptSourceRenderFailures(t *testing.T) {
	root := t.TempDir()
	errRender := errors.New("renderer exploded")

	t.Run("a renderer failure is reported against the source field", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeScript, Source: "[[ .name ]].star"}
		err := MaterializeScriptSource(step, root, func(string) (string, error) { return "", errRender })
		require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
		require.ErrorIs(t, err, errRender)
		assert.Contains(t, contextOf(err), "source")
		assert.Equal(t, "[[ .name ]].star", step.Source, "a failed render leaves the step untouched")
		assert.Empty(t, step.Script)
	})

	t.Run("a source that renders to a remote location is rejected", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "m", Type: schema.TaskTypeScript, Source: "{{ .url }}"}
		err := MaterializeScriptSource(step, root, func(string) (string, error) { return "https://example.com/check.star", nil })
		require.ErrorIs(t, err, errUtils.ErrScriptSourceRemote)
		assert.Contains(t, contextOf(err), "https://example.com/check.star")
		assert.Empty(t, step.Script)
	})
}

func TestScriptHandlerExecuteSourceTemplateFailures(t *testing.T) {
	initShellTestIO(t)
	tests := []struct {
		name   string
		source string
		want   error
	}{
		{name: "unparsable template", source: "scripts/{{ .broken", want: errUtils.ErrTemplateEvaluation},
		// The raw source holds no URL separator, so only the rendered value reveals the remote location.
		{name: "template rendering to a URL", source: `{{ "https:" }}//example.com/check.star`, want: errUtils.ErrScriptSourceRemote},
		{name: "template rendering to a go-getter source", source: `{{ "git:" }}:https://example.com/repo.git//check.star`, want: errUtils.ErrScriptSourceRemote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &schema.WorkflowStep{
				Name: "tmpl", Type: schema.TaskTypeScript, Source: tt.source,
				WorkingDirectory: t.TempDir(), Output: "none",
			}
			_, err := (&ScriptHandler{}).Execute(context.Background(), step, NewVariables())
			require.ErrorIs(t, err, tt.want)
			assert.Contains(t, contextOf(err), "tmpl")
		})
	}
}
