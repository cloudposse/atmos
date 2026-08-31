package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

const passingTestOutput = `tests/app.tftest.hcl... in progress
  run "bucket_name_is_namespaced"... pass
  run "provisions_resources_against_emulator"... pass
  run "versioning_can_be_disabled"... pass
tests/app.tftest.hcl... tearing down
tests/app.tftest.hcl... pass

Success! 3 passed, 0 failed.
`

const failingTestOutput = `tests/app.tftest.hcl... in progress
  run "bucket_name_is_namespaced"... pass
  run "provisions_resources_against_emulator"... fail

Error: Test assertion failed

  on tests/app.tftest.hcl line 30:
  30:     condition = output.bucket_id == "atmos-demo-test"

The S3 bucket was not created against the emulator

tests/app.tftest.hcl... fail

Failure! 1 passed, 1 failed.
`

const skippedTestOutput = `tests/app.tftest.hcl... in progress
  run "first"... pass
  run "second"... skip
tests/app.tftest.hcl... pass

Success! 1 passed, 0 failed.
`

func testData(t *testing.T, result *plugin.OutputResult) *plugin.TerraformTestOutputData {
	t.Helper()
	require.NotNil(t, result)
	data, ok := result.Data.(*plugin.TerraformTestOutputData)
	require.True(t, ok, "result.Data should be *TerraformTestOutputData")
	return data
}

func TestParseTestOutput_AllPass(t *testing.T) {
	result := ParseTestOutput(passingTestOutput)
	data := testData(t, result)

	assert.False(t, result.HasErrors)
	assert.Equal(t, 3, data.Total)
	assert.Equal(t, 3, data.Pass)
	assert.Equal(t, 0, data.Fail)
	assert.Equal(t, 0, data.Skip)

	// Assert element contents (not just length): first and last run by value.
	require.Len(t, data.Runs, 3)
	assert.Equal(t, plugin.TerraformTestRun{Name: "bucket_name_is_namespaced", Status: "pass"}, data.Runs[0])
	assert.Equal(t, plugin.TerraformTestRun{Name: "versioning_can_be_disabled", Status: "pass"}, data.Runs[2])
}

func TestParseTestOutput_WithFailure(t *testing.T) {
	result := ParseTestOutput(failingTestOutput)
	data := testData(t, result)

	assert.True(t, result.HasErrors, "a failing run must set HasErrors")
	assert.NotEmpty(t, result.Errors, "terraform Error: blocks should be surfaced")
	assert.Equal(t, 2, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 1, data.Fail)

	require.Len(t, data.Runs, 2)
	assert.Equal(t, "pass", data.Runs[0].Status)
	assert.Equal(t, plugin.TerraformTestRun{Name: "provisions_resources_against_emulator", Status: "fail"}, data.Runs[1])
}

func TestParseTestOutput_WithSkip(t *testing.T) {
	result := ParseTestOutput(skippedTestOutput)
	data := testData(t, result)

	assert.False(t, result.HasErrors)
	assert.Equal(t, 2, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 1, data.Skip)
	require.Len(t, data.Runs, 2)
	assert.Equal(t, "skip", data.Runs[1].Status)
}

func TestParseTestOutput_SummaryFallback(t *testing.T) {
	// No per-run lines captured; only the summary line is present. A synthesized
	// row still populates Runs so the CI summary's results table renders.
	result := ParseTestOutput("Success! 2 passed, 0 failed.\n")
	data := testData(t, result)

	assert.Equal(t, 2, data.Total)
	assert.Equal(t, 2, data.Pass)
	assert.Equal(t, 0, data.Fail)
	require.Len(t, data.Runs, 1)
	assert.Equal(t, testStatusPass, data.Runs[0].Status)
}

func TestParseTestOutput_FailureSummaryFallback(t *testing.T) {
	result := ParseTestOutput("Failure! 1 passed, 2 failed.\n")
	data := testData(t, result)

	assert.True(t, result.HasErrors)
	assert.Equal(t, 3, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 2, data.Fail)
	require.Len(t, data.Runs, 1)
	assert.Equal(t, testStatusFail, data.Runs[0].Status)
}

func TestParseTestOutput_SummaryFallback_RecoversErrorDetail(t *testing.T) {
	// Per-run "run ... pass/fail" lines were dropped, but the single "Error:"
	// diagnostic block terraform prints for the failing assertion survived. The
	// synthesized fallback row should recover the file/line from it -- but NOT
	// copy the raw multi-line message into the row itself, since that text is
	// already rendered safely in the fenced result.Errors block and embedding it
	// in a table cell would break the markdown table (multi-line content, and
	// HCL conditions routinely contain "|").
	const output = `Error: Test assertion failed

  on tests/app.tftest.hcl line 30:
  30:     condition = output.bucket_id == "atmos-demo-test"

The S3 bucket was not created against the emulator
╵

Failure! 1 passed, 1 failed.
`
	result := ParseTestOutput(output)
	data := testData(t, result)

	assert.True(t, result.HasErrors)
	assert.Equal(t, 2, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 1, data.Fail)

	require.Len(t, data.Runs, 1)
	run := data.Runs[0]
	assert.Equal(t, testStatusFail, run.Status)
	assert.Equal(t, "tests/app.tftest.hcl", run.File)
	assert.Equal(t, 30, run.Line)
	assert.Empty(t, run.Error, "the raw diagnostic block must not be duplicated into the row")

	// The full message is still available -- just via the separate, safely
	// fenced result.Errors block, not the row.
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "The S3 bucket was not created against the emulator")
}

func TestParseTestOutput_SummaryFallback_MultipleErrorBlocks_NoLocationAttributed(t *testing.T) {
	// Two distinct assertions failed in two different files. Attributing the
	// aggregate row's File/Line to just the first block would misrepresent which
	// failure it actually belongs to, so neither should be set when more than
	// one error block is present.
	const output = `Error: Test assertion failed

  on tests/app.tftest.hcl line 12:
  12:     condition = output.first == "expected"

first assertion message
╵

Error: Test assertion failed

  on tests/extra.tftest.hcl line 44:
  44:     condition = output.second == "expected"

second assertion message
╵

Failure! 0 passed, 2 failed.
`
	result := ParseTestOutput(output)
	data := testData(t, result)

	assert.Equal(t, 2, data.Total)
	assert.Equal(t, 0, data.Pass)
	assert.Equal(t, 2, data.Fail)

	require.Len(t, data.Runs, 1)
	run := data.Runs[0]
	assert.Equal(t, testStatusFail, run.Status)
	assert.Empty(t, run.File)
	assert.Zero(t, run.Line)
	assert.Empty(t, run.Error)

	// Both failures' full detail are still available via result.Errors.
	require.Len(t, result.Errors, 2)
	assert.Contains(t, result.Errors[0], "first assertion message")
	assert.Contains(t, result.Errors[1], "second assertion message")
}

func TestParseTestOutput_SummaryFallback_ZeroTotal(t *testing.T) {
	// No per-run lines and no summary match; nothing ran, so no row should be
	// synthesized -- a phantom row would misrepresent an empty result as a test.
	result := ParseTestOutput("terraform init...\n")
	data := testData(t, result)

	assert.Equal(t, 0, data.Total)
	assert.Empty(t, data.Runs)
}

func TestParseTestOutput_Empty(t *testing.T) {
	result := ParseTestOutput("")
	data := testData(t, result)

	assert.False(t, result.HasErrors)
	assert.Equal(t, 0, data.Total)
	assert.Empty(t, data.Runs)
}

// TestParseOutput_RoutesTest verifies the command dispatcher routes "test" to the
// test parser (not the default minimal result).
func TestParseOutput_RoutesTest(t *testing.T) {
	result := ParseOutput(passingTestOutput, "test")
	data := testData(t, result)
	assert.Equal(t, 3, data.Total)
}
