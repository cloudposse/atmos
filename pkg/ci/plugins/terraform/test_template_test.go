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

// TestTestTemplate_SummaryFallback_AllPass covers the shape ParseTestOutput
// produces when per-run lines weren't captured and it falls back to a single
// synthesized row -- the table must still render, not just badges.
func TestTestTemplate_SummaryFallback_AllPass(t *testing.T) {
	ctx := &TerraformTemplateContext{
		TemplateContext: &plugin.TemplateContext{
			Component: "app",
			Stack:     "local",
			Command:   "test",
			Result:    &plugin.OutputResult{HasErrors: false},
		},
		TestResult: &plugin.TerraformTestOutputData{
			Total: 2,
			Pass:  2,
			Runs: []plugin.TerraformTestRun{
				{Name: "test summary (per-run detail unavailable): 2 passed, 0 failed", Status: "pass"},
			},
		},
	}

	rendered := renderTestTemplate(t, ctx)

	for _, want := range []string{
		"Tests Passed for `app` in `local`",
		"TESTS-2",
		"PASSED-2",
		"| Result | File | Run | Duration | Details |",
		"| :white_check_mark: pass |  | `test summary (per-run detail unavailable): 2 passed, 0 failed` |  | |",
	} {
		assert.Contains(t, rendered, want)
	}
	assert.NotContains(t, rendered, "Tests Failed")
}

// TestTestTemplate_SummaryFallback_LosesRunDetail documents the residual, irreducible
// case: when NOTHING but the trailing summary line survived -- not even terraform's
// "Error:" diagnostic block -- there is no assertion detail left in the captured text
// to recover, so the rendered CI summary can only show aggregate counts and the repro
// command. Contrast with TestTestTemplate_SummaryFallback_RecoversErrorDetail, where
// the "Error:" block did survive and per-run detail is now recovered.
func TestTestTemplate_SummaryFallback_LosesRunDetail(t *testing.T) {
	// Only the trailing summary line survives -- simulates the per-run lines AND the
	// "Error:" diagnostic block being dropped (e.g. output buffering), even though the
	// run that failed had rich detail in reality (name
	// "provisions_resources_against_emulator", file tests/app.tftest.hcl, line 30,
	// assertion "The S3 bucket was not created...").
	const bufferedOutput = "Failure! 1 passed, 1 failed.\n"

	result := ParseTestOutput(bufferedOutput)
	data := testData(t, result)

	ctx := &TerraformTemplateContext{
		TemplateContext: &plugin.TemplateContext{
			Component: "app",
			Stack:     "local",
			Command:   "test",
			Result:    result,
		},
		TestResult: data,
	}
	rendered := renderTestTemplate(t, ctx)

	// What the summary *does* show: aggregate counts and the repro command.
	for _, want := range []string{
		"FAILED-1",
		"PASSED-1",
		"atmos terraform test app -s local",
	} {
		assert.Contains(t, rendered, want)
	}

	// What's missing: the specific run's name, file, line, and assertion message that
	// a fully-detailed (JSON-parsed) run would have surfaced -- this is the reported gap.
	for _, missing := range []string{
		"provisions_resources_against_emulator",
		"tests/app.tftest.hcl",
		":30",
		"The S3 bucket was not created",
	} {
		assert.NotContains(t, rendered, missing)
	}

	// Only the single synthesized aggregate row exists -- no per-run breakdown.
	require.Len(t, data.Runs, 1)
	assert.Contains(t, data.Runs[0].Name, "per-run detail unavailable")
}

// TestTestTemplate_SummaryFallback_RecoversErrorDetail is the fix for the gap
// TestTestTemplate_SummaryFallback_LosesRunDetail reproduces: when the per-run
// lines are dropped but terraform's "Error:" diagnostic block survived, the CI
// summary now renders the recovered file, line, and assertion message instead of
// only the generic "per-run detail unavailable" placeholder.
func TestTestTemplate_SummaryFallback_RecoversErrorDetail(t *testing.T) {
	const output = `Error: Test assertion failed

  on tests/app.tftest.hcl line 30:
  30:     condition = output.bucket_id == "atmos-demo-test"

The S3 bucket was not created against the emulator
╵

Failure! 1 passed, 1 failed.
`
	result := ParseTestOutput(output)
	data := testData(t, result)

	ctx := &TerraformTemplateContext{
		TemplateContext: &plugin.TemplateContext{
			Component: "app",
			Stack:     "local",
			Command:   "test",
			Result:    result,
		},
		TestResult: data,
	}
	rendered := renderTestTemplate(t, ctx)

	for _, want := range []string{
		"FAILED-1",
		"PASSED-1",
		"`tests/app.tftest.hcl:30`",
		"Test assertion failed",
		"The S3 bucket was not created against the emulator",
		"atmos terraform test app -s local",
	} {
		assert.Contains(t, rendered, want)
	}
}

// TestTestTemplate_SummaryFallback_WithFailure covers the failing counterpart
// of the summary-only fallback.
func TestTestTemplate_SummaryFallback_WithFailure(t *testing.T) {
	ctx := &TerraformTemplateContext{
		TemplateContext: &plugin.TemplateContext{
			Component: "app",
			Stack:     "local",
			Command:   "test",
			Result:    &plugin.OutputResult{HasErrors: true},
		},
		TestResult: &plugin.TerraformTestOutputData{
			Total: 3,
			Pass:  1,
			Fail:  2,
			Runs: []plugin.TerraformTestRun{
				{Name: "test summary (per-run detail unavailable): 1 passed, 2 failed", Status: "fail"},
			},
		},
	}

	rendered := renderTestTemplate(t, ctx)

	for _, want := range []string{
		"Tests Failed for `app` in `local`",
		"FAILED-2",
		"| Result | File | Run | Duration | Details |",
		"| :x: fail |  | `test summary (per-run detail unavailable): 1 passed, 2 failed` |  |  |",
	} {
		assert.Contains(t, rendered, want)
	}
}
