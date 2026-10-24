package proexec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/git"
)

func TestInvocationCorrelation(t *testing.T) {
	previous := ExecutionID()
	id, restore := BeginInvocation()
	defer restore()
	assert.Equal(t, id, ExecutionID())
	req, err := buildRecord(&ExecRecordInput{ExecutionID: id, Command: "terraform plan", Metrics: testMetrics()}, &fakeGitRepo{info: &git.RepoInfo{}})
	require.NoError(t, err)
	assert.Equal(t, id, req.ExecutionID)
	nested, restoreNested := BeginInvocation()
	assert.NotEqual(t, id, nested)
	restoreNested()
	assert.Equal(t, id, ExecutionID())
	restore()
	assert.Equal(t, previous, ExecutionID())
}
