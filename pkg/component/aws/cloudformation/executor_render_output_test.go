package cloudformation

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunOperation_RenderWritesTemplate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		dryRun bool
	}{
		{name: "normal"},
		{name: "dry run", dryRun: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := redirectRenderStdout(t)
			// A body larger than a pipe buffer catches truncation and added framing.
			body := "Description: " + strings.Repeat("local template ", 8192) + "\nResources: {}"
			spec := &stackSpec{StackName: "demo", TemplateBody: body}
			summary, err := runOperation(&opContext{
				Ctx:  context.Background(),
				Info: &schema.ConfigAndStacksInfo{DryRun: tc.dryRun},
			}, OperationRender, spec)
			require.NoError(t, err)
			actual, err := os.ReadFile(out.Name())
			require.NoError(t, err)
			require.Equal(t, body, string(actual))
			require.Equal(t, body, summary["template"])
		})
	}
}

func TestRunOperation_RenderWriteError(t *testing.T) {
	out := redirectRenderStdout(t)
	require.NoError(t, out.Close())

	summary, err := runOperation(&opContext{}, OperationRender, &stackSpec{
		StackName: "demo", TemplateBody: "Resources: {}\n",
	})
	require.ErrorIs(t, err, os.ErrClosed)
	require.ErrorIs(t, err, errUtils.ErrWriteToStream)
	require.Equal(t, "Resources: {}\n", summary["template"])
}

func redirectRenderStdout(t *testing.T) *os.File {
	t.Helper()

	out, err := os.CreateTemp(t.TempDir(), "render-output")
	require.NoError(t, err)
	previous := os.Stdout
	os.Stdout = out
	t.Cleanup(func() {
		os.Stdout = previous
		_ = out.Close()
	})
	return out
}
