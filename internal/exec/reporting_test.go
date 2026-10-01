package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/proexec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestComponentReportingPreservesResolvedMetadata(t *testing.T) {
	id, restore := proexec.BeginInvocation()
	defer restore()
	info := schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "prod", ComponentMetadataSection: map[string]any{"labels": map[string]any{"team": "platform"}}}
	cause := errUtils.ExitCodeError{Code: 42}
	var failure error = cause
	attachComponentReporting(&failure, &info, "terraform", "deploy")
	var reported *errUtils.ReportingError
	require.ErrorAs(t, failure, &reported)
	snapshot, executionID := reported.ReportingContext()
	assert.Equal(t, id, executionID)
	assert.Equal(t, "terraform", snapshot.ComponentType)
	assert.Equal(t, "deploy", snapshot.SubCommand)
	assert.Equal(t, "platform", snapshot.ComponentMetadataSection["labels"].(map[string]any)["team"])
	assert.Equal(t, 42, errUtils.GetExitCode(failure))
	assert.ErrorIs(t, failure, cause)
	var success error
	attachComponentReporting(&success, &info, "terraform", "plan")
	assert.NoError(t, success)
	restore()
	failure = cause
	attachComponentReporting(&failure, &info, "terraform", "plan")
	assert.Equal(t, cause, failure, "standalone callers preserve the original error")
}
