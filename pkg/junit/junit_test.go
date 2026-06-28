package junit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleReport() Report {
	r := Report{
		Name: "atmos",
		Suites: []Suite{
			{
				Name: "tests/app.tftest.hcl",
				Cases: []Case{
					{Name: "bucket_name_is_namespaced", Classname: "app", Time: 0.1},
					{
						Name: "provisions_resources", Classname: "app", File: "tests/app.tftest.hcl", Line: 30,
						Failure: &Detail{Message: "bucket not created", Text: "assertion failed"},
					},
					{Name: "skipped_case", Classname: "app", Skipped: &Detail{Message: "skipped"}},
				},
			},
		},
	}
	r.Aggregate()
	return r
}

func TestAggregate_ComputesCounts(t *testing.T) {
	r := sampleReport()
	assert.Equal(t, 3, r.Tests)
	assert.Equal(t, 1, r.Failures)
	assert.Equal(t, 0, r.Errors)
	assert.Equal(t, 1, r.Skipped)
	assert.Equal(t, 3, r.Suites[0].Tests)
	assert.Equal(t, 1, r.Suites[0].Failures)
	assert.False(t, r.Passed())
}

func TestFormatParse_RoundTrip(t *testing.T) {
	original := sampleReport()

	xmlBytes, err := Format(&original)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(xmlBytes), "<?xml"), "should have an XML header")
	assert.Contains(t, string(xmlBytes), `<testsuites`)
	assert.Contains(t, string(xmlBytes), `<testcase name="provisions_resources"`)
	assert.Contains(t, string(xmlBytes), `<failure message="bucket not created"`)

	parsed, err := Parse(xmlBytes)
	require.NoError(t, err)
	require.Len(t, parsed.Suites, 1)
	require.Len(t, parsed.Suites[0].Cases, 3)
	assert.Equal(t, "fail", parsed.Suites[0].Cases[1].Status())
	assert.Equal(t, 30, parsed.Suites[0].Cases[1].Line)
	assert.Equal(t, 1, parsed.Failures)
}

func TestParse_BareTestsuiteRoot(t *testing.T) {
	// Some runners emit a single <testsuite> root rather than <testsuites>.
	data := []byte(`<testsuite name="suite-a" tests="1" failures="0">
  <testcase name="t1" classname="c"/>
</testsuite>`)

	r, err := Parse(data)
	require.NoError(t, err)
	require.Len(t, r.Suites, 1)
	assert.Equal(t, "suite-a", r.Suites[0].Name)
	require.Len(t, r.Suites[0].Cases, 1)
	assert.Equal(t, "pass", r.Suites[0].Cases[0].Status())
}

func TestParse_Invalid(t *testing.T) {
	_, err := Parse([]byte("not xml <<<"))
	require.ErrorIs(t, err, ErrParse)
}

func TestFailedCases(t *testing.T) {
	r := sampleReport()
	failed := r.FailedCases()
	require.Len(t, failed, 1)
	assert.Equal(t, FailureRef{
		Suite:   "tests/app.tftest.hcl",
		Name:    "provisions_resources",
		File:    "tests/app.tftest.hcl",
		Line:    30,
		Message: "bucket not created",
	}, failed[0])
}

func TestCaseStatus(t *testing.T) {
	cases := []struct {
		name string
		c    Case
		want string
	}{
		{"pass", Case{}, "pass"},
		{"fail", Case{Failure: &Detail{}}, "fail"},
		{"error", Case{Error: &Detail{}}, "error"},
		{"skip", Case{Skipped: &Detail{}}, "skip"},
		// Error takes precedence over failure if both somehow set.
		{"error-precedence", Case{Failure: &Detail{}, Error: &Detail{}}, "error"},
	}
	for _, tc := range cases {
		tc := tc
		assert.Equal(t, tc.want, tc.c.Status(), tc.name)
	}
}

func TestMarkdown(t *testing.T) {
	r := sampleReport()
	md := Markdown(&r, Options{Title: "Terraform tests"})

	assert.Contains(t, md, "## ❌ Terraform tests")
	assert.Contains(t, md, "TESTS-3")
	assert.Contains(t, md, "PASSED-1")
	assert.Contains(t, md, "FAILED-1")
	assert.Contains(t, md, "SKIPPED-1")
	assert.Contains(t, md, ":x: fail | `tests/app.tftest.hcl` | `provisions_resources`")
	assert.Contains(t, md, "tests/app.tftest.hcl:30")
	assert.Contains(t, md, "bucket not created")
}

func TestMarkdown_AllPass(t *testing.T) {
	r := Report{Suites: []Suite{{Name: "s", Cases: []Case{{Name: "a"}, {Name: "b"}}}}}
	md := Markdown(&r, Options{})
	assert.Contains(t, md, "## ✅ Test results")
	assert.Contains(t, md, "PASSED-2")
	assert.NotContains(t, md, "FAILED-")
	assert.NotContains(t, md, "<details><summary>Failures")
}
