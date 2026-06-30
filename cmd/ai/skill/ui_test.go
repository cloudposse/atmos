package skill

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func setupSkillCommandUI(t *testing.T) *bytes.Buffer {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ioCtx, err := iolib.NewContext(iolib.WithStreams(testStreams{
		input:  bytes.NewBuffer(nil),
		output: &stdout,
		error:  &stderr,
	}))
	require.NoError(t, err)

	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)

	return &stderr
}
