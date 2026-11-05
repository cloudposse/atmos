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

// SuppressLogGrouping reports true only while running inside a deprecated
// marketplace action, so ci grouping is disabled there and stdout stays pure.
func TestProvider_SuppressLogGrouping(t *testing.T) {
	p := NewProvider()

	t.Run("legacy action suppresses grouping", func(t *testing.T) {
		t.Setenv("GITHUB_ACTION_REPOSITORY", "cloudposse/github-action-atmos-get-setting")
		assert.True(t, p.SuppressLogGrouping())
	})

	t.Run("current action does not suppress grouping", func(t *testing.T) {
		t.Setenv("GITHUB_ACTION_REPOSITORY", "cloudposse/github-action-setup-atmos")
		assert.False(t, p.SuppressLogGrouping())
	})

	t.Run("no action repository does not suppress grouping", func(t *testing.T) {
		t.Setenv("GITHUB_ACTION_REPOSITORY", "")
		assert.False(t, p.SuppressLogGrouping())
	})
}
