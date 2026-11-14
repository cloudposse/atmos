package container

import (
	"context"
	"fmt"
	"io"

	ctr "github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// buildWithProgress uses the shared viewport, falling back to live diagnostics off-TTY.
func buildWithProgress(ctx context.Context, runtime ctr.Runtime, config *ctr.BuildConfig, name string) error {
	output := step.NewOutputModeWriter(step.OutputModeViewport, fmt.Sprintf("Building %s", name), &schema.ViewportConfig{Height: 8, Padding: 1}).WithStdin(nil)
	_, _, err := output.ExecuteWithIO(func(_, diagnostic io.Writer) error {
		build := *config
		build.Output = diagnostic
		return runtime.Build(ctx, &build)
	})
	return err
}
