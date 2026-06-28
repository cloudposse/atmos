package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

// sampleTestJSON is a `terraform test -json` stream: a passing run, a failing run
// whose diagnostic (with a source range) arrives BEFORE its complete event, a
// skipped run, and the final summary.
const sampleTestJSON = `{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"ok","test_run":{"path":"tests/app.tftest.hcl","run":"ok","progress":"complete","status":"pass","elapsed":120}}
{"@level":"error","type":"diagnostic","@testfile":"tests/app.tftest.hcl","@testrun":"broken","diagnostic":{"severity":"error","summary":"Test assertion failed","detail":"bucket not created","range":{"filename":"tests/app.tftest.hcl","start":{"line":30,"column":5}}}}
{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"broken","test_run":{"path":"tests/app.tftest.hcl","run":"broken","progress":"running"}}
{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"broken","test_run":{"path":"tests/app.tftest.hcl","run":"broken","progress":"complete","status":"fail","elapsed":50}}
{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"skipped","test_run":{"run":"skipped","progress":"complete","status":"skip"}}
{"@level":"info","type":"test_summary","test_summary":{"status":"fail","passed":1,"failed":1,"errored":0,"skipped":1}}
`

func testJSONData(t *testing.T, result *plugin.OutputResult) *plugin.TerraformTestOutputData {
	t.Helper()
	require.NotNil(t, result)
	data, ok := result.Data.(*plugin.TerraformTestOutputData)
	require.True(t, ok)
	return data
}

func TestParseTestJSON(t *testing.T) {
	result := ParseTestJSON([]byte(sampleTestJSON))
	data := testJSONData(t, result)

	assert.True(t, result.HasErrors)
	assert.Equal(t, 3, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 1, data.Fail)
	assert.Equal(t, 1, data.Skip)

	require.Len(t, data.Runs, 3)
	// Passing run with elapsed → duration in seconds.
	assert.Equal(t, plugin.TerraformTestRun{Name: "ok", File: "tests/app.tftest.hcl", Status: "pass", Duration: 0.12}, data.Runs[0])
	// Failing run: the earlier diagnostic is attached (message + file:line).
	broken := data.Runs[1]
	assert.Equal(t, "broken", broken.Name)
	assert.Equal(t, "fail", broken.Status)
	assert.Equal(t, "tests/app.tftest.hcl", broken.File)
	assert.Equal(t, 30, broken.Line)
	assert.Equal(t, "Test assertion failed: bucket not created", broken.Error)
	assert.Equal(t, "skip", data.Runs[2].Status)

	assert.Contains(t, result.Errors, "Test assertion failed: bucket not created")
}

func TestParseTestJSON_AllPass(t *testing.T) {
	stream := `{"@level":"info","type":"test_run","@testrun":"a","test_run":{"run":"a","progress":"complete","status":"pass"}}
{"@level":"info","type":"test_summary","test_summary":{"status":"pass","passed":1,"failed":0,"errored":0,"skipped":0}}
`
	result := ParseTestJSON([]byte(stream))
	data := testJSONData(t, result)
	assert.False(t, result.HasErrors)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 0, data.Fail)
}

func TestParseOutput_RoutesTestJSON(t *testing.T) {
	// Leading `{` → JSON path; the text path would not populate File/Line.
	data := testJSONData(t, ParseOutput(sampleTestJSON, "test"))
	assert.Equal(t, 30, data.Runs[1].Line)
}

func TestParseOutput_RoutesTestText(t *testing.T) {
	// Human output still routes to the regex parser.
	data := testJSONData(t, ParseOutput("  run \"a\"... pass\n\nSuccess! 1 passed, 0 failed.\n", "test"))
	assert.Equal(t, 1, data.Total)
	assert.Equal(t, 0, data.Runs[0].Line, "text path has no line info")
}

func TestRenderTestText(t *testing.T) {
	text := RenderTestText([]byte(sampleTestJSON))
	assert.Contains(t, text, `✓ run "ok"... pass`)
	assert.Contains(t, text, `✗ run "broken"... fail`)
	assert.Contains(t, text, "Test assertion failed: bucket not created")
	assert.Contains(t, text, "Failure! 1 passed, 1 failed, 1 skipped.")
}

func TestRenderTestText_Empty(t *testing.T) {
	assert.Empty(t, RenderTestText([]byte("")))
	assert.Empty(t, RenderTestText([]byte("not json")))
}

func TestToJUnit(t *testing.T) {
	data := testJSONData(t, ParseTestJSON([]byte(sampleTestJSON)))
	report := toJUnit(data, "app")

	require.Len(t, report.Suites, 1)
	suite := report.Suites[0]
	assert.Equal(t, "tests/app.tftest.hcl", suite.Name)
	assert.Equal(t, 3, suite.Tests)
	assert.Equal(t, 1, suite.Failures)
	assert.Equal(t, 1, suite.Skipped)

	require.Len(t, suite.Cases, 3)
	failing := suite.Cases[1]
	assert.Equal(t, "broken", failing.Name)
	assert.Equal(t, "app", failing.Classname)
	assert.Equal(t, 30, failing.Line)
	require.NotNil(t, failing.Failure)
	assert.Equal(t, "Test assertion failed: bucket not created", failing.Failure.Message)

	// Report-level rollups.
	assert.Equal(t, 3, report.Tests)
	assert.Equal(t, 1, report.Failures)
	assert.False(t, report.Passed())
}
