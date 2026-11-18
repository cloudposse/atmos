package report_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
	atmosio "github.com/cloudposse/atmos/pkg/io"
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

func TestOutputsMaskRegisteredSecretsInEveryValue(t *testing.T) {
	const secret = "mask-me-output-value"
	atmosio.RegisterSecret(secret)
	outcome := published("a", "x/"+secret+":1")
	outcome.Result.Location = "oci://registry.example/" + secret + "/x?X-Amz-Signature=abc"

	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake)
	session.Record(entry("publish", "a", 1, outcome, nil))
	require.NoError(t, session.Flush())

	require.NotEmpty(t, fake.outputs)
	for _, item := range fake.outputs {
		assert.NotContains(t, item.Value, secret, "output %s is masked", item.Key)
		assert.NotContains(t, item.Value, "X-Amz-Signature", "output %s has no query string", item.Key)
	}
}

func TestOutputsDoNotShareAQualifiedNameBetweenDistinctStepNames(t *testing.T) {
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake)
	// publish-vpc and publish_vpc normalize to the same segment.
	session.Record(entry("publish-vpc", "a", 1, published("a", "x/dash:1"), nil, 0))
	session.Record(entry("publish_vpc", "b", 1, published("b", "x/underscore:1"), nil, 2))
	require.NoError(t, session.Flush())

	values := outputMap(fake)
	assert.Equal(t, "x/dash:1", values["artifact_publish_vpc_ref"], "the first step in report order keeps the qualified name")
	assert.Equal(t, "x/underscore:1", values["artifact_ref"], "the unqualified names still follow the last operation")

	qualifiedWrites := 0
	for _, item := range fake.outputs {
		if item.Key == "artifact_publish_vpc_ref" {
			qualifiedWrites++
		}
	}
	assert.Equal(t, 1, qualifiedWrites, "the qualified name is written once, never shadowed by another step")
}

func TestOutputsRepeatedStepNameKeepsItsQualifiedName(t *testing.T) {
	fake := &fakeReporter{}
	session := newSession(enabledConfig(), fake)
	session.Record(entry("publish", "a", 1, published("a", "x/one:1"), nil, 0))
	session.Record(entry("publish", "b", 1, published("b", "x/two:1"), nil, 2))
	require.NoError(t, session.Flush())
	assert.Equal(t, "x/two:1", outputMap(fake)["artifact_publish_ref"], "the same step name is one owner, and the last run wins")
}

func TestOutputsClaimQualifiedNamesPerOutputNotPerStep(t *testing.T) {
	deleted := func(id string) *invoke.Outcome {
		return result(&artifact.OperationResult{OperationID: id, Action: artifact.ActionDelete, Outcome: artifact.OutcomeDeleted})
	}
	t.Run("a delete step does not shadow a later publish step with a colliding segment", func(t *testing.T) {
		fake := &fakeReporter{}
		session := newSession(enabledConfig(), fake)
		// The delete writes only the outcome; the publish writes the digests the delete never produced.
		session.Record(entry("publish-vpc", "a", 1, deleted("a"), nil, 0))
		session.Record(entry("publish_vpc", "b", 1, published("b", "x/underscore:1"), nil, 1))
		require.NoError(t, session.Flush())

		values := outputMap(fake)
		assert.Equal(t, "deleted", values["artifact_publish_vpc_outcome"], "the first writer of a name keeps it")
		assert.Equal(t, "x/underscore:1", values["artifact_publish_vpc_ref"], "the publish step owns the names the delete never wrote")
		assert.Equal(t, string(digestOf("a")), values["artifact_publish_vpc_resolved_digest"])
	})
	t.Run("a name written by both steps stays with the first step", func(t *testing.T) {
		fake := &fakeReporter{}
		session := newSession(enabledConfig(), fake)
		session.Record(entry("publish-vpc", "a", 1, published("a", "x/dash:1"), nil, 0))
		session.Record(entry("publish_vpc", "b", 1, published("b", "x/underscore:1"), nil, 1))
		require.NoError(t, session.Flush())

		values := outputMap(fake)
		assert.Equal(t, "x/dash:1", values["artifact_publish_vpc_ref"])
		assert.Equal(t, "x/underscore:1", values["artifact_ref"], "the unqualified names follow the last operation")
	})
}
