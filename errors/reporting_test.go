package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestReportingContextSnapshotAndIdentity(t *testing.T) {
	cause := ExitCodeError{Code: 42}
	info := schema.ConfigAndStacksInfo{Component: "vpc", ComponentMetadataSection: map[string]any{"labels": map[string]any{"team": "platform"}, "tags": []any{"production"}}}
	wrapped := WithReportingContext(cause, &info, "execution-id")
	require.ErrorIs(t, wrapped, cause)
	assert.Equal(t, 42, GetExitCode(wrapped))
	info.ComponentMetadataSection["labels"].(map[string]any)["team"] = "changed"
	var reported *ReportingError
	require.ErrorAs(t, wrapped, &reported)
	snapshot, id := reported.ReportingContext()
	assert.Equal(t, "execution-id", id)
	assert.Equal(t, "platform", snapshot.ComponentMetadataSection["labels"].(map[string]any)["team"])
	snapshot.ComponentMetadataSection["tags"].([]any)[0] = "changed"
	snapshot, _ = reported.ReportingContext()
	assert.Equal(t, []any{"production"}, snapshot.ComponentMetadataSection["tags"])
	assert.Equal(t, []any{"production"}, info.ComponentMetadataSection["tags"])
	nested := WithReportingContext(wrapped, &info, id)
	var outer *ReportingError
	require.ErrorAs(t, nested, &outer)
	assert.True(t, reported.Claim())
	assert.False(t, outer.Claim(), "wrapping again must not duplicate the same occurrence")
	distinct := WithReportingContext(cause, &info, id)
	require.True(t, errors.As(distinct, &outer))
	assert.True(t, outer.Claim(), "separate occurrences with identical causes remain distinct")
	assert.Nil(t, WithReportingContext(nil, nil, ""))
}
