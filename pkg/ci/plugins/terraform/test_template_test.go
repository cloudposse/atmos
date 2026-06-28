package terraform

import (
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

func renderTestTemplate(t *testing.T, ctx *TerraformTemplateContext) string {
	t.Helper()
	content, err := defaultTemplates.ReadFile("templates/test.md")
	require.NoError(t, err)
	tmpl, err := template.New("test").Parse(string(content))
	require.NoError(t, err)
	var buf strings.Builder
	require.NoError(t, tmpl.Execute(&buf, ctx))
	return buf.String()
}

func TestTestTemplate_AllPass(t *testing.T) {
	ctx := &TerraformTemplateContext{
		TemplateContext: &plugin.TemplateContext{
			Component: "app",
			Stack:     "local",
			Command:   "test",
			Result:    &plugin.OutputResult{HasErrors: false},
		},
		TestResult: &plugin.TerraformTestOutputData{
			Total: 3,
			Pass:  3,
			Runs: []plugin.TerraformTestRun{
				{Name: "bucket_name_is_namespaced", Status: "pass"},
				{Name: "provisions_resources_against_emulator", Status: "pass"},
				{Name: "versioning_can_be_disabled", Status: "pass"},
			},
		},
	}

	rendered := renderTestTemplate(t, ctx)

	for _, want := range []string{
		"Tests Passed for `app` in `local`",
		"TESTS-3",
		"PASSED-3",
		"bucket_name_is_namespaced",
		"versioning_can_be_disabled",
		"atmos terraform test app -s local",
	} {
		assert.Contains(t, rendered, want)
	}
	assert.NotContains(t, rendered, "Tests Failed")
}

func TestTestTemplate_WithFailure(t *testing.T) {
	ctx := &TerraformTemplateContext{
		TemplateContext: &plugin.TemplateContext{
			Component: "app",
			Stack:     "local",
			Command:   "test",
			Result: &plugin.OutputResult{
				HasErrors: true,
				Errors:    []string{"Error: Test assertion failed"},
			},
		},
		TestResult: &plugin.TerraformTestOutputData{
			Total: 2,
			Pass:  1,
			Fail:  1,
			Runs: []plugin.TerraformTestRun{
				{Name: "ok_case", Status: "pass"},
				{Name: "broken_case", Status: "fail"},
			},
		},
	}

	rendered := renderTestTemplate(t, ctx)

	for _, want := range []string{
		"Tests Failed for `app` in `local`",
		"FAILED-1",
		"broken_case",
		"Test assertion failed",
	} {
		assert.Contains(t, rendered, want)
	}
}
