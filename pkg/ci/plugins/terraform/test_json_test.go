package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

// sampleTestJSON is a `terraform test -json` stream: two test files, a passing
// run, a failing run whose diagnostic arrives BEFORE its complete event, a
// skipped run, an errored run, cleanup leftovers, and the final summary.
const sampleTestJSON = `{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"ok","test_run":{"path":"tests/app.tftest.hcl","run":"ok","progress":"complete","status":"pass","elapsed":120}}
{"@level":"error","type":"diagnostic","@testfile":"tests/app.tftest.hcl","@testrun":"broken","diagnostic":{"severity":"error","summary":"Test assertion failed","detail":"bucket not created","range":{"filename":"tests/app.tftest.hcl","start":{"line":30,"column":5}}}}
{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"broken","test_run":{"path":"tests/app.tftest.hcl","run":"broken","progress":"running"}}
{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"broken","test_run":{"path":"tests/app.tftest.hcl","run":"broken","progress":"complete","status":"fail","elapsed":50}}
{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"skipped","test_run":{"path":"tests/app.tftest.hcl","run":"skipped","progress":"complete","status":"skip"}}
{"@level":"info","type":"test_file","@testfile":"tests/app.tftest.hcl","test_file":{"path":"tests/app.tftest.hcl","progress":"complete","status":"fail"}}
{"@level":"error","type":"diagnostic","@testfile":"tests/extra.tftest.hcl","@testrun":"setup","diagnostic":{"severity":"error","summary":"Provider error","detail":"could not create role","range":{"filename":"tests/extra.tftest.hcl","start":{"line":12,"column":3}}}}
{"@level":"info","type":"test_run","@testfile":"tests/extra.tftest.hcl","@testrun":"setup","test_run":{"path":"tests/extra.tftest.hcl","run":"setup","progress":"complete","status":"error","elapsed":75}}
{"@level":"info","type":"test_cleanup","@testfile":"tests/extra.tftest.hcl","@testrun":"setup","test_cleanup":{"failed_resources":[{"instance":"aws_s3_bucket.left"},{"instance":"aws_iam_role.left"}]}}
{"@level":"info","type":"test_file","@testfile":"tests/extra.tftest.hcl","test_file":{"path":"tests/extra.tftest.hcl","progress":"complete","status":"error"}}
{"@level":"info","type":"test_summary","test_summary":{"status":"error","passed":1,"failed":1,"errored":1,"skipped":1}}
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
	assert.Equal(t, 4, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 1, data.Fail)
	assert.Equal(t, 1, data.Error)
	assert.Equal(t, 1, data.Skip)

	require.Len(t, data.Runs, 4)
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
	errored := data.Runs[3]
	assert.Equal(t, "setup", errored.Name)
	assert.Equal(t, "error", errored.Status)
	assert.Equal(t, "tests/extra.tftest.hcl", errored.File)
	assert.Equal(t, 12, errored.Line)
	assert.Equal(t, "Provider error: could not create role", errored.Error)

	require.Len(t, data.Files, 2)
	assert.Equal(t, plugin.TerraformTestFile{Path: "tests/app.tftest.hcl", Status: "fail", Pass: 1, Fail: 1, Skip: 1}, data.Files[0])
	assert.Equal(t, plugin.TerraformTestFile{Path: "tests/extra.tftest.hcl", Status: "error", Error: 1}, data.Files[1])

	require.Len(t, data.CleanupFailures, 1)
	assert.Equal(t, plugin.TerraformTestCleanupFailure{
		File:      "tests/extra.tftest.hcl",
		Run:       "setup",
		Resources: []string{"aws_s3_bucket.left", "aws_iam_role.left"},
	}, data.CleanupFailures[0])

	assert.Contains(t, result.Errors, "Test assertion failed: bucket not created")
	assert.Contains(t, result.Errors, "Provider error: could not create role")
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
	assert.Equal(t, 0, data.Error)
}

func TestParseTestJSON_SummaryExceedsRuns(t *testing.T) {
	// Only the authoritative test_summary event arrives; no test_run "complete"
	// events were captured into data.Runs (e.g. one was dropped upstream). Runs
	// must be backfilled so Total/JUnit/the results table never under-report a
	// passing run as tests="0".
	stream := `{"@level":"info","type":"test_summary","test_summary":{"status":"pass","passed":1,"failed":0,"errored":0,"skipped":0}}
`
	result := ParseTestJSON([]byte(stream))
	data := testJSONData(t, result)

	assert.False(t, result.HasErrors)
	assert.Equal(t, 1, data.Total)
	assert.Equal(t, 1, data.Pass)
	require.Len(t, data.Runs, 1)
	assert.Equal(t, testStatusPass, data.Runs[0].Status)
}

func TestBackfillMissingTestJSONRuns(t *testing.T) {
	tests := []struct {
		name string
		data plugin.TerraformTestOutputData
		want []string // expected Status of each run in data.Runs after backfill
	}{
		{
			name: "no mismatch leaves runs untouched",
			data: plugin.TerraformTestOutputData{
				Pass: 1,
				Runs: []plugin.TerraformTestRun{{Name: "a", Status: testStatusPass}},
			},
			want: []string{testStatusPass},
		},
		{
			name: "summary exceeds runs — fully missing",
			data: plugin.TerraformTestOutputData{Pass: 1},
			want: []string{testStatusPass},
		},
		{
			name: "summary exceeds runs — partial, mixed statuses",
			data: plugin.TerraformTestOutputData{
				Pass: 2,
				Fail: 1,
				Runs: []plugin.TerraformTestRun{{Name: "a", Status: testStatusPass}},
			},
			want: []string{testStatusPass, testStatusPass, testStatusFail},
		},
		{
			name: "runs exceed summary — never deletes real data",
			data: plugin.TerraformTestOutputData{
				Pass: 1,
				Runs: []plugin.TerraformTestRun{
					{Name: "a", Status: testStatusPass},
					{Name: "b", Status: testStatusPass},
				},
			},
			want: []string{testStatusPass, testStatusPass},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.data
			backfillMissingTestJSONRuns(&data)
			require.Len(t, data.Runs, len(tt.want))
			for i, status := range tt.want {
				assert.Equal(t, status, data.Runs[i].Status)
			}
		})
	}
}

func TestToJUnit_BackfillsMissingRuns(t *testing.T) {
	stream := `{"@level":"info","type":"test_summary","test_summary":{"status":"pass","passed":1,"failed":0,"errored":0,"skipped":0}}
`
	data := testJSONData(t, ParseTestJSON([]byte(stream)))
	report := toJUnit(data, "app")

	assert.Equal(t, 1, report.Tests)
	assert.True(t, report.Passed())
}

// sampleOpenTofuPassJSON is a verbatim `tofu test -json` stream (OpenTofu
// 1.12.5): OpenTofu emits exactly one test_run/test_file event per run/file,
// carrying only `status` -- there is no `progress` field at all.
const sampleOpenTofuPassJSON = `{"@level":"info","@message":"OpenTofu 1.12.5","@module":"tofu.ui","@timestamp":"2026-09-08T09:25:28.566974-05:00","tofu":"1.12.5","type":"version","ui":"1.2"}
{"@level":"info","@message":"Found 1 file and 2 run blocks","@module":"tofu.ui","@timestamp":"2026-09-08T09:25:28.567828-05:00","test_abstract":{"tests/min.tftest.hcl":["plan_case","apply_case"]},"type":"test_abstract"}
{"@level":"info","@message":"tests/min.tftest.hcl... pass","@module":"tofu.ui","@testfile":"tests/min.tftest.hcl","@timestamp":"2026-09-08T09:25:28.581280-05:00","test_file":{"path":"tests/min.tftest.hcl","status":"pass"},"type":"test_file"}
{"@level":"info","@message":"  \"plan_case\"... pass","@module":"tofu.ui","@testfile":"tests/min.tftest.hcl","@testrun":"plan_case","@timestamp":"2026-09-08T09:25:28.581312-05:00","test_run":{"path":"tests/min.tftest.hcl","run":"plan_case","status":"pass"},"type":"test_run"}
{"@level":"info","@message":"  \"apply_case\"... pass","@module":"tofu.ui","@testfile":"tests/min.tftest.hcl","@testrun":"apply_case","@timestamp":"2026-09-08T09:25:28.581323-05:00","test_run":{"path":"tests/min.tftest.hcl","run":"apply_case","status":"pass"},"type":"test_run"}
{"@level":"info","@message":"Success! 2 passed, 0 failed.","@module":"tofu.ui","@timestamp":"2026-09-08T09:25:28.582628-05:00","test_summary":{"status":"pass","passed":2,"failed":0,"errored":0,"skipped":0},"type":"test_summary"}
`

// sampleOpenTofuFailJSON is a verbatim failing `tofu test -json` stream. Note
// the assertion diagnostic arrives AFTER the run's test_run event.
const sampleOpenTofuFailJSON = `{"@level":"info","@message":"OpenTofu 1.12.5","@module":"tofu.ui","@timestamp":"2026-09-08T09:26:17.462022-05:00","tofu":"1.12.5","type":"version","ui":"1.2"}
{"@level":"info","@message":"Found 1 file and 2 run blocks","@module":"tofu.ui","@timestamp":"2026-09-08T09:26:17.464956-05:00","test_abstract":{"tests/min.tftest.hcl":["passing_case","failing_case"]},"type":"test_abstract"}
{"@level":"info","@message":"tests/min.tftest.hcl... fail","@module":"tofu.ui","@testfile":"tests/min.tftest.hcl","@timestamp":"2026-09-08T09:26:17.478520-05:00","test_file":{"path":"tests/min.tftest.hcl","status":"fail"},"type":"test_file"}
{"@level":"info","@message":"  \"passing_case\"... pass","@module":"tofu.ui","@testfile":"tests/min.tftest.hcl","@testrun":"passing_case","@timestamp":"2026-09-08T09:26:17.478559-05:00","test_run":{"path":"tests/min.tftest.hcl","run":"passing_case","status":"pass"},"type":"test_run"}
{"@level":"info","@message":"  \"failing_case\"... fail","@module":"tofu.ui","@testfile":"tests/min.tftest.hcl","@testrun":"failing_case","@timestamp":"2026-09-08T09:26:17.478570-05:00","test_run":{"path":"tests/min.tftest.hcl","run":"failing_case","status":"fail"},"type":"test_run"}
{"@level":"error","@message":"Error: Test assertion failed","@module":"tofu.ui","@testfile":"tests/min.tftest.hcl","@testrun":"failing_case","@timestamp":"2026-09-08T09:26:17.478795-05:00","diagnostic":{"severity":"error","summary":"Test assertion failed","detail":"name should be b","range":{"filename":"tests/min.tftest.hcl","start":{"line":12,"column":21,"byte":203},"end":{"line":12,"column":39,"byte":221}},"snippet":{"context":"run \"failing_case\"","code":"    condition     = output.name == \"b\"","start_line":12,"highlight_start_offset":20,"highlight_end_offset":38,"values":[{"traversal":"output.name","statement":"is \"a\""}]},"difference":{"before":"a","after":"b","after_unknown":false,"before_sensitive":false,"after_sensitive":false}},"type":"diagnostic"}
{"@level":"info","@message":"Failure! 1 passed, 1 failed.","@module":"tofu.ui","@timestamp":"2026-09-08T09:26:17.481079-05:00","test_summary":{"status":"fail","passed":1,"failed":1,"errored":0,"skipped":0},"type":"test_summary"}
`

func TestParseTestJSON_OpenTofu_AllPass(t *testing.T) {
	// OpenTofu's test_run/test_file events carry no `progress` field, so a
	// "progress == complete" gate silently discards every run: badges show the
	// summary counts while the results table and JUnit report come out empty.
	result := ParseTestJSON([]byte(sampleOpenTofuPassJSON))
	data := testJSONData(t, result)

	assert.False(t, result.HasErrors)
	assert.Equal(t, 2, data.Total)
	assert.Equal(t, 2, data.Pass)
	require.Len(t, data.Runs, 2)
	assert.Equal(t, plugin.TerraformTestRun{Name: "plan_case", File: "tests/min.tftest.hcl", Status: testStatusPass}, data.Runs[0])
	assert.Equal(t, plugin.TerraformTestRun{Name: "apply_case", File: "tests/min.tftest.hcl", Status: testStatusPass}, data.Runs[1])
	require.Len(t, data.Files, 1)
	assert.Equal(t, plugin.TerraformTestFile{Path: "tests/min.tftest.hcl", Status: testStatusPass, Pass: 2}, data.Files[0])
}

func TestParseTestJSON_OpenTofu_Failure(t *testing.T) {
	result := ParseTestJSON([]byte(sampleOpenTofuFailJSON))
	data := testJSONData(t, result)

	assert.True(t, result.HasErrors)
	assert.Equal(t, 2, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 1, data.Fail)
	require.Len(t, data.Runs, 2)
	assert.Equal(t, "passing_case", data.Runs[0].Name)
	assert.Equal(t, testStatusPass, data.Runs[0].Status)

	// The assertion diagnostic arrived after the run event; it must still be
	// attached so the results table, annotations, and JUnit carry file:line.
	failing := data.Runs[1]
	assert.Equal(t, "failing_case", failing.Name)
	assert.Equal(t, testStatusFail, failing.Status)
	assert.Equal(t, "tests/min.tftest.hcl", failing.File)
	assert.Equal(t, 12, failing.Line)
	assert.Equal(t, "Test assertion failed: name should be b", failing.Error)
	assert.Contains(t, result.Errors, "Test assertion failed: name should be b")

	require.Len(t, data.Files, 1)
	assert.Equal(t, plugin.TerraformTestFile{Path: "tests/min.tftest.hcl", Status: testStatusFail, Pass: 1, Fail: 1}, data.Files[0])
}

func TestParseTestJSON_DiagnosticAfterCompleteEvent(t *testing.T) {
	// Terraform emits assertion-failure diagnostics after the run's `complete`
	// event too (only mid-apply provider errors precede it), so attachment must
	// work in either order.
	stream := `{"@level":"info","type":"test_run","@testfile":"tests/app.tftest.hcl","@testrun":"broken","test_run":{"path":"tests/app.tftest.hcl","run":"broken","progress":"complete","status":"fail","elapsed":50}}
{"@level":"error","type":"diagnostic","@testfile":"tests/app.tftest.hcl","@testrun":"broken","diagnostic":{"severity":"error","summary":"Test assertion failed","detail":"bucket not created","range":{"filename":"tests/app.tftest.hcl","start":{"line":30,"column":5}}}}
{"@level":"info","type":"test_summary","test_summary":{"status":"fail","passed":0,"failed":1,"errored":0,"skipped":0}}
`
	result := ParseTestJSON([]byte(stream))
	data := testJSONData(t, result)

	require.Len(t, data.Runs, 1)
	assert.Equal(t, "broken", data.Runs[0].Name)
	assert.Equal(t, 30, data.Runs[0].Line)
	assert.Equal(t, "Test assertion failed: bucket not created", data.Runs[0].Error)
	assert.Contains(t, result.Errors, "Test assertion failed: bucket not created")
}

func TestToJUnit_OpenTofu(t *testing.T) {
	data := testJSONData(t, ParseTestJSON([]byte(sampleOpenTofuFailJSON)))
	report := toJUnit(data, "app")

	assert.Equal(t, 2, report.Tests)
	assert.Equal(t, 1, report.Failures)
	require.Len(t, report.Suites, 1)
	require.Len(t, report.Suites[0].Cases, 2)
	assert.Equal(t, "passing_case", report.Suites[0].Cases[0].Name)
	failing := report.Suites[0].Cases[1]
	assert.Equal(t, "failing_case", failing.Name)
	assert.Equal(t, 12, failing.Line)
	require.NotNil(t, failing.Failure)
	assert.Equal(t, "Test assertion failed: name should be b", failing.Failure.Message)
}

func TestRenderTestText_OpenTofu(t *testing.T) {
	text := RenderTestText([]byte(sampleOpenTofuFailJSON))
	assert.Contains(t, text, `✓ run "passing_case"... pass`)
	assert.Contains(t, text, `✗ run "failing_case"... fail`)
	assert.Contains(t, text, "Test assertion failed: name should be b")
	assert.Contains(t, text, "Failure! 1 passed, 1 failed, 0 skipped.")
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

func TestParseOutput_RoutesTestJSON_WithInitPreamble(t *testing.T) {
	// The captured CI stream is prefixed with terraform init/workspace human
	// preamble before the `-json` events; routing must still pick the JSON parser
	// (a leading-char check would wrongly fall through to the text parser, leaving
	// TestResult empty so badges/run-table/counts go missing).
	preamble := "Initializing provider plugins...\n" +
		"Terraform has been successfully initialized!\n" +
		"Switched to workspace \"local\".\n"
	data := testJSONData(t, ParseOutput(preamble+sampleTestJSON, "test"))
	assert.Equal(t, 4, data.Total)
	assert.Equal(t, 1, data.Pass)
	assert.Equal(t, 1, data.Fail)
	assert.Equal(t, 1, data.Error)
	assert.Equal(t, 30, data.Runs[1].Line, "file:line must survive the preamble (JSON path)")
}

func TestIsJSONStream(t *testing.T) {
	assert.True(t, isJSONStream(sampleTestJSON), "leading JSON")
	assert.True(t, isJSONStream("Initializing provider plugins...\n"+sampleTestJSON), "JSON after preamble")
	assert.False(t, isJSONStream("  run \"a\"... pass\nSuccess! 1 passed, 0 failed.\n"), "human output")
	assert.False(t, isJSONStream(`{"foo":"bar"}`), "non-terraform JSON without @level")
}

func TestCleanOutput_TestRendersJSONToCleanText(t *testing.T) {
	// In CI the test output is the `-json` stream (mixed with init preamble); the
	// job summary must show the rendered human summary, never the raw JSON.
	preamble := "Initializing provider plugins...\n"
	cleaned := cleanOutput(preamble+sampleTestJSON, "test")
	assert.NotContains(t, cleaned, `"@level"`, "raw JSON must not leak into the summary")
	assert.NotContains(t, cleaned, "Initializing provider", "init preamble must not leak")
	assert.Contains(t, cleaned, `✓ run "ok"... pass`)
	assert.Contains(t, cleaned, "Failure! 1 passed, 1 failed, 1 errored, 1 skipped.")
}

func TestCleanOutput_TestKeepsHumanOutputVerbatim(t *testing.T) {
	// Non-CI runs emit no `-json`; keep the human output as-is (trimmed).
	human := "run \"a\"... pass\nSuccess! 1 passed, 0 failed."
	assert.Equal(t, human, cleanOutput("  "+human+"\n", "test"))
}

func TestRenderTestText(t *testing.T) {
	text := RenderTestText([]byte(sampleTestJSON))
	assert.Contains(t, text, `✓ run "ok"... pass`)
	assert.Contains(t, text, `✗ run "broken"... fail`)
	assert.Contains(t, text, "Test assertion failed: bucket not created")
	assert.Contains(t, text, "Failure! 1 passed, 1 failed, 1 errored, 1 skipped.")
}

func TestRenderTestText_Empty(t *testing.T) {
	assert.Empty(t, RenderTestText([]byte("")))
	assert.Empty(t, RenderTestText([]byte("not json")))
}

func TestFileStatusFromCounts(t *testing.T) {
	tests := []struct {
		name string
		file plugin.TerraformTestFile
		want string
	}{
		{
			name: "error wins",
			file: plugin.TerraformTestFile{Pass: 1, Fail: 1, Error: 1, Skip: 1},
			want: testStatusError,
		},
		{
			name: "fail wins over skip",
			file: plugin.TerraformTestFile{Pass: 1, Fail: 1, Skip: 1},
			want: testStatusFail,
		},
		{
			name: "only skipped",
			file: plugin.TerraformTestFile{Skip: 2},
			want: testStatusSkip,
		},
		{
			name: "pass when at least one pass and skips",
			file: plugin.TerraformTestFile{Pass: 1, Skip: 1},
			want: testStatusPass,
		},
		{
			name: "empty defaults pass",
			file: plugin.TerraformTestFile{},
			want: testStatusPass,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fileStatusFromCounts(tt.file))
		})
	}
}

func TestDiagMessage(t *testing.T) {
	assert.Equal(t, "summary: detail", diagMessage(testJSONDiag{Summary: "summary", Detail: "detail"}))
	assert.Equal(t, "summary", diagMessage(testJSONDiag{Summary: "summary"}))
	assert.Equal(t, "detail", diagMessage(testJSONDiag{Detail: "detail"}))
	assert.Empty(t, diagMessage(testJSONDiag{}))
}

func TestToJUnit(t *testing.T) {
	data := testJSONData(t, ParseTestJSON([]byte(sampleTestJSON)))
	report := toJUnit(data, "app")

	require.Len(t, report.Suites, 2)
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
	require.Len(t, report.Suites[1].Cases, 1)
	require.NotNil(t, report.Suites[1].Cases[0].Error)
	assert.Equal(t, "Provider error: could not create role", report.Suites[1].Cases[0].Error.Message)

	// Report-level rollups.
	assert.Equal(t, 4, report.Tests)
	assert.Equal(t, 1, report.Failures)
	assert.Equal(t, 1, report.Errors)
	assert.False(t, report.Passed())
}
