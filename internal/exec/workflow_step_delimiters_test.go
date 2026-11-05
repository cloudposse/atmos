package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// Compile-time guards for the schema fields these tests set.
var (
	_ = schema.AtmosConfiguration{Edition: ""}
	_ = schema.WorkflowStep{Content: "", Command: "", Env: nil}
)

func TestProcessStepTmplHonorsDelimiters(t *testing.T) {
	data := map[string]any{"name": "ok"}
	tests := []struct {
		name   string
		delims delimiters.Delimiters
		input  string
		want   string
	}{
		{name: "custom pair renders and the default pair is literal", delims: delimiters.Delimiters{Left: "[[", Right: "]]"}, input: "[[ .name ]] {{ .name }}", want: "ok {{ .name }}"},
		{name: "default pair renders and the custom pair is literal", delims: delimiters.Default(), input: "[[ .name ]] {{ .name }}", want: "[[ .name ]] ok"},
		{name: "zero value is the default pair", delims: delimiters.Delimiters{}, input: "{{ .name }}", want: "ok"},
		{name: "template functions work under custom delimiters", delims: delimiters.Delimiters{Left: "<%", Right: "%>"}, input: `<% .name | upper %>`, want: "OK"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProcessStepTmpl(&schema.AtmosConfiguration{}, "step", tt.input, data, tt.delims)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveTemplateDelimitersSharesTheValidation(t *testing.T) {
	left, right, err := resolveTemplateDelimiters(nil)
	require.NoError(t, err)
	assert.Equal(t, "{{", left)
	assert.Equal(t, "}}", right)

	left, right, err = resolveTemplateDelimiters([]string{"[[", "]]"})
	require.NoError(t, err)
	assert.Equal(t, "[[", left)
	assert.Equal(t, "]]", right)

	_, _, err = resolveTemplateDelimiters([]string{"[["})
	require.ErrorIs(t, err, errUtils.ErrInvalidTemplateSettings)
}

// TestExecuteWorkflowStepTemplatesHonorConfiguredDelimiters runs a real workflow whose later step
// references the first step's output with each delimiter pair, under the edition pins that select
// the old and the new behavior.
func TestExecuteWorkflowStepTemplatesHonorConfiguredDelimiters(t *testing.T) {
	if !t.Run("subprocess prerequisite", requireWorkflowOutputHelper) {
		return
	}

	const content = "[[ .steps.produce.value ]] {{ .steps.produce.value }}"
	tests := []struct {
		name    string
		pin     string
		want    string
		wantEnv string
	}{
		{name: "unpinned", pin: "", want: "produced {{ .steps.produce.value }}", wantEnv: "produced-{{ .steps.produce.value }}"},
		{name: "pinned on the transition date", pin: "2026-10-19", want: "produced {{ .steps.produce.value }}", wantEnv: "produced-{{ .steps.produce.value }}"},
		{name: "pinned before the transition", pin: "2026-10-18", want: "[[ .steps.produce.value ]] produced", wantEnv: "[[ .steps.produce.value ]]-produced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ResetStepExecutorState()
			t.Cleanup(ResetStepExecutorState)

			exePath, err := os.Executable()
			require.NoError(t, err)
			workflowDef := &schema.WorkflowDefinition{
				Steps: []schema.WorkflowStep{
					{
						Name:    "produce",
						Type:    "shell",
						Command: fmt.Sprintf("%q", exePath),
						Env:     map[string]string{"_ATMOS_TEST_STDOUT": "produced"},
					},
					{Name: "render", Type: "markdown", Content: content},
					{
						// The helper binary prints its _ATMOS_TEST_STDOUT env value, so the output is the rendered env.
						Name:    "echo-env",
						Type:    "shell",
						Command: fmt.Sprintf("%q", exePath),
						Env:     map[string]string{"_ATMOS_TEST_STDOUT": "[[ .steps.produce.value ]]-{{ .steps.produce.value }}"},
					},
				},
			}
			atmosConfig := schema.AtmosConfiguration{BasePath: t.TempDir(), Edition: tt.pin}
			atmosConfig.Templates.Settings.Delimiters = []string{"[[", "]]"}

			err = ExecuteWorkflow(atmosConfig, "delimiters", filepath.Join(atmosConfig.BasePath, "workflow.yaml"), workflowDef, false, "", "", "")
			require.NoError(t, err)

			rendered, ok := stepExecutorState.GetResult("render")
			require.True(t, ok)
			assert.Equal(t, tt.want, rendered.Value)

			echoed, ok := stepExecutorState.GetResult("echo-env")
			require.True(t, ok)
			assert.Equal(t, tt.wantEnv, echoed.Value)
		})
	}
}

func TestExecuteWorkflowRejectsInvalidStepDelimiterConfig(t *testing.T) {
	ResetStepExecutorState()
	t.Cleanup(ResetStepExecutorState)

	workflowDef := &schema.WorkflowDefinition{Steps: []schema.WorkflowStep{{Name: "render", Type: "markdown", Content: "x"}}}
	atmosConfig := schema.AtmosConfiguration{BasePath: t.TempDir()}
	atmosConfig.Templates.Settings.Delimiters = []string{"[["}

	err := ExecuteWorkflow(atmosConfig, "invalid-delimiters", filepath.Join(atmosConfig.BasePath, "workflow.yaml"), workflowDef, false, "", "", "")
	require.ErrorIs(t, err, errUtils.ErrInvalidTemplateSettings)

	atmosConfig.Templates.Settings.Delimiters = nil
	atmosConfig.Edition = "later"
	err = ExecuteWorkflow(atmosConfig, "invalid-pin", filepath.Join(atmosConfig.BasePath, "workflow.yaml"), workflowDef, false, "", "", "")
	require.ErrorIs(t, err, edition.ErrInvalidEdition)
}
