package github

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_LogGroup_WritesWorkflowCommands(t *testing.T) {
	var buf bytes.Buffer
	prev := workflowCommandsOut
	workflowCommandsOut = &buf
	defer func() { workflowCommandsOut = prev }()

	p := NewProvider()
	require.NoError(t, p.StartLogGroup("hook policy:check, 50%\nnext"))
	require.NoError(t, p.EndLogGroup())

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "::group::hook policy:check, 50%25%0Anext", lines[0])
	assert.Equal(t, "::endgroup::", lines[1])
}

func TestProvider_LogGroup_WriteErrorPropagates(t *testing.T) {
	prev := workflowCommandsOut
	workflowCommandsOut = failWriter{}
	defer func() { workflowCommandsOut = prev }()

	p := NewProvider()
	assert.Error(t, p.StartLogGroup("hook"))
	assert.Error(t, p.EndLogGroup())
}
