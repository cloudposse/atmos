package report_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/schema"
)

func outputMap(fake *fakeReporter) map[string]string {
	values := map[string]string{}
	for _, item := range fake.outputs {
		values[item.Key] = item.Value
	}
	return values
}

func TestOutputsUseTheNamedStepOutputs(t *testing.T) {
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake)
	outcome := published("op-1", "components/vpc:1.2.0")
	session.Record(entry("Publish VPC", "op-1", 1, outcome, nil))
	require.NoError(t, session.Flush())

	values := outputMap(fake)
	for _, pair := range invoke.OutputPairs(&outcome.Result.OperationResult) {
		assert.Equal(t, pair.Value, values[report.OutputPrefix+pair.Name], "unqualified %s", pair.Name)
		assert.Equal(t, pair.Value, values[report.OutputPrefix+"publish_vpc_"+pair.Name], "step-qualified %s", pair.Name)
	}
	for _, name := range []string{"ref", "location", "resolved_digest", "record_digest", "content_digest", "representation_digest", "size", "outcome"} {
		assert.Contains(t, values, "artifact_"+name)
	}
	assert.Equal(t, "created", values["artifact_outcome"])
	assert.Equal(t, "49152", values["artifact_size"])
}

func TestOutputsOmitValuesTheOperationDidNotProduce(t *testing.T) {
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake)
	session.Record(entry("", "a", 1, result(&artifact.OperationResult{Action: artifact.ActionDelete, Outcome: artifact.OutcomeDeleted, Ref: "x/y:1"}), nil))
	require.NoError(t, session.Flush())
	values := outputMap(fake)
	assert.Equal(t, map[string]string{"artifact_outcome": "deleted", "artifact_ref": "x/y:1"}, values)
}

func TestOutputsLastOperationOwnsTheUnqualifiedNames(t *testing.T) {
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake)
	session.Record(entry("second", "b", 1, published("b", "x/two:1"), nil, 2))
	session.Record(entry("first", "a", 1, published("a", "x/one:1"), nil, 0))
	require.NoError(t, session.Flush())
	values := outputMap(fake)
	assert.Equal(t, "x/two:1", values["artifact_ref"], "report order decides the unqualified names")
	assert.Equal(t, "x/one:1", values["artifact_first_ref"])
	assert.Equal(t, "x/two:1", values["artifact_second_ref"])
}

func TestOutputsDoNotLeakSignedUrls(t *testing.T) {
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake)
	outcome := published("a", "x/y:1")
	outcome.Result.Location = "https://bucket.example/key?X-Amz-Signature=abc"
	session.Record(entry("", "a", 1, outcome, nil))
	require.NoError(t, session.Flush())
	assert.Equal(t, "https://bucket.example/key", outputMap(fake)["artifact_location"])
}

func TestOutputsSkipCollectionsAndFailedWithoutResult(t *testing.T) {
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake)
	session.Record(&report.Entry{Action: artifact.ActionList, Attempt: 1, Outcome: &invoke.Outcome{Collection: &invoke.Collection{}}})
	session.Record(&report.Entry{Action: artifact.ActionPull, Attempt: 1})
	require.NoError(t, session.Flush())
	assert.Empty(t, fake.outputs)
}

func TestOutputsRespectTheirGate(t *testing.T) {
	disabled := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Output: schema.CIOutputConfig{Enabled: ptr(false)}}}
	fake := &fakeReporter{}
	session := newSession(disabled, fake)
	session.Record(entry("", "a", 1, published("a", "x/y:1"), nil))
	require.NoError(t, session.Flush())
	assert.Empty(t, fake.outputs, "ci.output.enabled=false writes no output variables")
	assert.Len(t, fake.summaries, 1, "the summary is independent of the output gate")
}
