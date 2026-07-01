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
	// No per-run lines captured; only the summary line is present.
	result := ParseTestOutput("Success! 2 passed, 0 failed.\n")
	data := testData(t, result)

	assert.Equal(t, 2, data.Total)
	assert.Equal(t, 2, data.Pass)
	assert.Equal(t, 0, data.Fail)
	assert.Empty(t, data.Runs)
}

func TestParseTestOutput_FailureSummaryFallback(t *testing.T) {
	result := ParseTestOutput("Failure! 1 passed, 2 failed.\n")
	data := testData(t, result)

	assert.True(t, result.HasErrors)
	assert.Equal(t, 3, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 2, data.Fail)
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
