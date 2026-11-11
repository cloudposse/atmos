package service

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/stretchr/testify/assert"
)

func TestReportExclusionsIsBounded(t *testing.T) {
	op := &operation{}
	exclusions := make([]artifact.Exclusion, maxExclusionDiagnostics+3)
	reportExclusions(op, exclusions)
	assert.Len(t, op.result.Diagnostics, maxExclusionDiagnostics+1)
	assert.Equal(t, "3 additional exclusions omitted", op.result.Diagnostics[maxExclusionDiagnostics].Message)
}
