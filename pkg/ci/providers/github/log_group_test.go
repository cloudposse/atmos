package github

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestProvider_LogGroup_WritesWorkflowCommands(t *testing.T) {
	stdout := &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	p := NewProvider()
	require.NoError(t, p.StartLogGroup("hook policy:check, 50%\nnext"))
	require.NoError(t, p.EndLogGroup())

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Equal(t, "::group::hook policy:check, 50%25%0Anext", lines[0])
	assert.Equal(t, "::endgroup::", lines[1])
}

func TestProvider_LogGroup_WriteErrorPropagates(t *testing.T) {
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: errWriter{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	p := NewProvider()
	assert.Error(t, p.StartLogGroup("hook"))
	assert.Error(t, p.EndLogGroup())
}
