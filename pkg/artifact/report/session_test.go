package report_test

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSessionKeepsTheFinalAttemptOfEachOperation(t *testing.T) {
	failed := published("op-1", "components/vpc:1.2.0")
	failed.Result.Outcome = artifact.OutcomeFailed
	markdown := flushOne(
		t, enabledConfig(),
		entry("publish", "op-1", 1, failed, errors.New("transient")),
		entry("publish", "op-1", 2, failed, errors.New("transient")),
		entry("publish", "op-1", 3, published("op-1", "components/vpc:1.2.0"), nil),
	)
	assert.Contains(t, markdown, "1 operation: 1 succeeded.")
	assert.Contains(t, markdown, "✅ Published")
	assert.NotContains(t, markdown, "transient")
	assert.Contains(t, markdown, "**Attempts:** 3")
}

func TestSessionKeepsTheFinalFailedAttempt(t *testing.T) {
	markdown := flushOne(
		t, enabledConfig(),
		entry("publish", "op-1", 2, published("op-1", "components/vpc:1.2.0"), errors.New("second failure")),
		// A late arrival of an earlier attempt must not replace the final one.
		entry("publish", "op-1", 1, published("op-1", "components/vpc:1.2.0"), errors.New("first failure")),
	)
	assert.Contains(t, markdown, "second failure")
	assert.NotContains(t, markdown, "first failure")
	assert.Contains(t, markdown, "1 operation: 1 failed.")
}

func TestSessionOrdersByScheduledPositionNotCompletionOrder(t *testing.T) {
	// The branches finish in the reverse of their scheduled order; the report follows the schedule.
	markdown := flushOne(
		t, enabledConfig(),
		entry("third", "c", 1, published("c", "x/third:1"), nil, 2, 3, 0),
		entry("second", "b", 1, published("b", "x/second:1"), nil, 2, 1, 1),
		entry("first", "a", 1, published("a", "x/first:1"), nil, 2, 1, 0),
		entry("before", "z", 1, published("z", "x/before:1"), nil, 0),
		entry("unpositioned", "u", 1, published("u", "x/unpositioned:1"), nil),
	)
	assert.Equal(t, []string{"x/before:1", "x/first:1", "x/second:1", "x/third:1", "x/unpositioned:1"}, sectionRefs(markdown))
}

// sectionHeading matches a per-operation section heading and captures its reference.
var sectionHeading = regexp.MustCompile("(?m)^### \\S+ \\d+\\. \\w+ `([^`]+)`$")

// sectionRefs returns the references of the operation sections in document order.
func sectionRefs(markdown string) []string {
	var refs []string
	for _, match := range sectionHeading.FindAllStringSubmatch(markdown, -1) {
		refs = append(refs, match[1])
	}
	return refs
}

func TestSessionOrdersUnpositionedEntriesByArrival(t *testing.T) {
	markdown := flushOne(
		t, enabledConfig(),
		entry("one", "one", 1, published("one", "x/one:1"), nil),
		entry("two", "two", 1, published("two", "x/two:1"), nil),
		entry("three", "three", 1, published("three", "x/three:1"), nil),
	)
	assert.Equal(t, []string{"x/one:1", "x/two:1", "x/three:1"}, sectionRefs(markdown))
}

func TestSessionIsDeterministicUnderConcurrentRecording(t *testing.T) {
	render := func() string {
		fake := &fakeReporter{}
		session := report.NewSession(enabledConfig(), report.WithReporter(fake.factory(nil)))
		var wg sync.WaitGroup
		for index := 0; index < 16; index++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				id := fmt.Sprintf("op-%02d", index)
				session.Record(entry("branch", id, 1, published(id, fmt.Sprintf("x/%02d:1", index)), nil, 1, index))
			}()
		}
		wg.Wait()
		require.NoError(t, session.Flush())
		return fake.summaries[0]
	}
	want := render()
	for run := 0; run < 5; run++ {
		require.Equal(t, want, render())
	}
}

func TestSessionFlushWritesOnceAndEmpties(t *testing.T) {
	fake := &fakeReporter{}
	session := report.NewSession(enabledConfig(), report.WithReporter(fake.factory(nil)))
	session.Record(entry("publish", "a", 1, published("a", "x/y:1"), nil))
	require.NoError(t, session.Flush())
	require.NoError(t, session.Flush())
	assert.Len(t, fake.summaries, 1, "a second flush must not repeat the report")
}

func TestSessionWithNothingRecordedBuildsNoReporter(t *testing.T) {
	builds := 0
	fake := &fakeReporter{}
	session := report.NewSession(enabledConfig(), report.WithReporter(fake.factory(&builds)))
	require.NoError(t, session.Flush())
	assert.Zero(t, builds)
	assert.Empty(t, fake.summaries)
	assert.Empty(t, fake.outputs)
}

func TestSessionGates(t *testing.T) {
	cases := []struct {
		name          string
		config        *schema.AtmosConfiguration
		wantSummary   bool
		wantOutputs   bool
		wantReporters int
	}{
		{"nil config", nil, false, false, 0},
		{"ci disabled", &schema.AtmosConfiguration{}, false, false, 0},
		{"summary disabled", &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Summary: schema.CISummaryConfig{Enabled: ptr(false)}}}, false, true, 1},
		{"output disabled", &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Output: schema.CIOutputConfig{Enabled: ptr(false)}}}, true, false, 1},
		{"both disabled", &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Summary: schema.CISummaryConfig{Enabled: ptr(false)}, Output: schema.CIOutputConfig{Enabled: ptr(false)}}}, false, false, 0},
		{"enabled", enabledConfig(), true, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			builds := 0
			fake := &fakeReporter{}
			session := report.NewSession(tc.config, report.WithReporter(fake.factory(&builds)))
			assert.Equal(t, tc.wantSummary || tc.wantOutputs, session.Enabled())
			session.Record(entry("publish", "a", 1, published("a", "x/y:1"), nil))
			require.NoError(t, session.Flush())
			assert.Equal(t, tc.wantSummary, len(fake.summaries) == 1)
			assert.Equal(t, tc.wantOutputs, len(fake.outputs) > 0)
			assert.Equal(t, tc.wantReporters, builds)
		})
	}
}

func TestSessionIgnoresNilEntries(t *testing.T) {
	fake := &fakeReporter{}
	session := report.NewSession(enabledConfig(), report.WithReporter(fake.factory(nil)))
	session.Record(nil)
	require.NoError(t, session.Flush())
	assert.Empty(t, fake.summaries)
}

func TestSessionProviderFailureIsReportedNotRaised(t *testing.T) {
	fake := &fakeReporter{summaryErr: errUtils.ErrCISummaryWriteFailed, outputErr: errUtils.ErrCIOutputWriteFailed}
	session := report.NewSession(enabledConfig(), report.WithReporter(fake.factory(nil)))
	session.Record(entry("publish", "a", 1, published("a", "x/y:1"), nil))
	err := session.Flush()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactReportWrite)
	assert.ErrorIs(t, err, errUtils.ErrCISummaryWriteFailed)
	assert.ErrorIs(t, err, errUtils.ErrCIOutputWriteFailed)
	// The outputs are still attempted when the summary fails.
	assert.NotEmpty(t, fake.outputs)
	// The logging flush never panics or returns.
	session.Record(entry("publish", "b", 1, published("b", "x/y:2"), nil))
	assert.NotPanics(t, session.FlushAndLog)
}

func TestSessionTemplateFailureStillWritesOutputs(t *testing.T) {
	config := enabledConfig()
	config.CI.Templates = schema.CITemplatesConfig{BasePath: t.TempDir(), Artifacts: map[string]string{"summary": "missing.md"}}
	fake := &fakeReporter{}
	session := report.NewSession(config, report.WithReporter(fake.factory(nil)))
	session.Record(entry("publish", "a", 1, published("a", "x/y:1"), nil))
	err := session.Flush()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactReportRender)
	assert.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
	assert.Empty(t, fake.summaries, "no summary is written, and no other template is chosen silently")
	assert.NotEmpty(t, fake.outputs, "machine outputs do not depend on the summary template")
}

func TestEntryForUsesTheResultOperationID(t *testing.T) {
	entry := report.EntryFor(artifact.ActionPublish, published("op-9", "x/y:1"), nil)
	assert.Equal(t, "op-9", entry.OperationID)
	assert.Equal(t, 1, entry.Attempt)
	assert.Equal(t, artifact.ActionPublish, entry.Action)
	assert.Empty(t, report.EntryFor(artifact.ActionList, nil, nil).OperationID)
}
