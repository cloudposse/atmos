package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
)

const junitFixture = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites name="atmos" tests="3" failures="1" errors="0" skipped="1">
  <testsuite name="tests/app.tftest.hcl" tests="3" failures="1" errors="0" skipped="1">
    <testcase name="ok_case" classname="app"/>
    <testcase name="broken_case" classname="app" file="tests/app.tftest.hcl" line="30">
      <failure message="bucket not created">assertion failed</failure>
    </testcase>
    <testcase name="skipped_case" classname="app"><skipped/></testcase>
  </testsuite>
</testsuites>`

// stubJUnitCISeams replaces the CI seams with capturing fakes for the test.
func stubJUnitCISeams(t *testing.T) (summaries *[]string, annotations *[][]ci.Annotation) {
	t.Helper()
	origSummary, origAnnotate := writeStepSummaryFn, annotateFn
	t.Cleanup(func() { writeStepSummaryFn, annotateFn = origSummary, origAnnotate })

	var caughtSummaries []string
	var caughtAnnotations [][]ci.Annotation
	writeStepSummaryFn = func(content string) error {
		caughtSummaries = append(caughtSummaries, content)
		return nil
	}
	annotateFn = func(a []ci.Annotation) error {
		caughtAnnotations = append(caughtAnnotations, a)
		return nil
	}
	return &caughtSummaries, &caughtAnnotations
}

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "report.xml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestJUnitHandler_Registered(t *testing.T) {
	h, ok := Get(junitStepType)
	require.True(t, ok, "junit step type must be registered")
	assert.Equal(t, junitStepType, h.GetName())
}

func TestJUnitHandler_Validate(t *testing.T) {
	h := &JUnitHandler{BaseHandler: NewBaseHandler(junitStepType, CategoryOutput, false)}

	require.Error(t, h.Validate(&schema.WorkflowStep{Name: "x", Type: "junit"}), "files required")
	require.Error(t, h.Validate(&schema.WorkflowStep{Name: "x", Type: "junit", Files: []string{"a.xml"}, Action: "bogus"}))
	for _, action := range []string{"", "summary", "annotate", "all"} {
		require.NoError(t, h.Validate(&schema.WorkflowStep{Name: "x", Type: "junit", Files: []string{"a.xml"}, Action: action}))
	}
}

func TestJUnitHandler_Execute_SummaryAndAnnotations(t *testing.T) {
	summaries, annotations := stubJUnitCISeams(t)
	path := writeFixture(t, junitFixture)

	h := &JUnitHandler{BaseHandler: NewBaseHandler(junitStepType, CategoryOutput, false)}
	step := &schema.WorkflowStep{Name: "report", Type: "junit", Files: []string{path}, Title: "Terraform tests"}

	result, err := h.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, *summaries, 1)
	assert.Contains(t, (*summaries)[0], "## ❌ Terraform tests")
	assert.Contains(t, (*summaries)[0], "broken_case")

	require.Len(t, *annotations, 1)
	require.Len(t, (*annotations)[0], 1, "one annotation for the single failing case")
	ann := (*annotations)[0][0]
	assert.Equal(t, "tests/app.tftest.hcl", ann.Path)
	assert.Equal(t, 30, ann.StartLine)
	assert.Equal(t, ci.AnnotationError, ann.Level)
	assert.Equal(t, "bucket not created", ann.Message)
}

func TestJUnitHandler_Execute_SummaryOnly(t *testing.T) {
	summaries, annotations := stubJUnitCISeams(t)
	path := writeFixture(t, junitFixture)

	h := &JUnitHandler{BaseHandler: NewBaseHandler(junitStepType, CategoryOutput, false)}
	step := &schema.WorkflowStep{Name: "report", Type: "junit", Files: []string{path}, Action: "summary"}

	_, err := h.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Len(t, *summaries, 1)
	assert.Empty(t, *annotations, "annotate action not requested")
}

func TestJUnitHandler_Execute_NoMatches(t *testing.T) {
	stubJUnitCISeams(t)
	h := &JUnitHandler{BaseHandler: NewBaseHandler(junitStepType, CategoryOutput, false)}
	step := &schema.WorkflowStep{Name: "report", Type: "junit", Files: []string{filepath.Join(t.TempDir(), "nope-*.xml")}}

	_, err := h.Execute(context.Background(), step, NewVariables())
	require.Error(t, err, "no matching files should error")
}

func TestJUnitHandler_Execute_InvalidFile(t *testing.T) {
	stubJUnitCISeams(t)
	path := writeFixture(t, "not xml <<<")
	h := &JUnitHandler{BaseHandler: NewBaseHandler(junitStepType, CategoryOutput, false)}
	step := &schema.WorkflowStep{Name: "report", Type: "junit", Files: []string{path}}

	_, err := h.Execute(context.Background(), step, NewVariables())
	require.Error(t, err)
}
