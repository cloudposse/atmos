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
			Files: []plugin.TerraformTestFile{
				{Path: "tests/app.tftest.hcl", Status: "pass", Pass: 3},
			},
			Runs: []plugin.TerraformTestRun{
				{Name: "bucket_name_is_namespaced", File: "tests/app.tftest.hcl", Status: "pass", Duration: 0.12},
				{Name: "provisions_resources_against_emulator", File: "tests/app.tftest.hcl", Status: "pass", Duration: 1.5},
				{Name: "versioning_can_be_disabled", File: "tests/app.tftest.hcl", Status: "pass", Duration: 0.03},
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
		"`tests/app.tftest.hcl`",
		"0.12s",
		"<details><summary>Detailed test results</summary>",
		"| `tests/app.tftest.hcl` | :white_check_mark: pass | 3 | 0 | 0 | 0 |",
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
			Total: 3,
			Pass:  1,
			Fail:  1,
			Error: 1,
			Files: []plugin.TerraformTestFile{
				{Path: "tests/app.tftest.hcl", Status: "fail", Pass: 1, Fail: 1},
				{Path: "tests/extra.tftest.hcl", Status: "error", Error: 1},
			},
			Runs: []plugin.TerraformTestRun{
				{Name: "ok_case", File: "tests/app.tftest.hcl", Status: "pass", Duration: 0.1},
				{Name: "broken_case", File: "tests/app.tftest.hcl", Status: "fail", Error: "Test assertion failed", Line: 22, Duration: 0.2},
				{Name: "setup", File: "tests/extra.tftest.hcl", Status: "error", Error: "Provider error", Line: 8},
			},
			CleanupFailures: []plugin.TerraformTestCleanupFailure{
				{File: "tests/extra.tftest.hcl", Run: "setup", Resources: []string{"aws_s3_bucket.left"}},
			},
		},
	}

	rendered := renderTestTemplate(t, ctx)

	for _, want := range []string{
		"Tests Failed for `app` in `local`",
		"FAILED-1",
		"ERRORED-1",
		"broken_case",
		"`tests/app.tftest.hcl:22` Test assertion failed",
		"`tests/extra.tftest.hcl:8` Provider error",
		"| `tests/extra.tftest.hcl` | :boom: error | 0 | 0 | 1 | 0 |",
		"### Cleanup failures",
		"`aws_s3_bucket.left`",
		"Test assertion failed",
	} {
		assert.Contains(t, rendered, want)
	}
}
