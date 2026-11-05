package releaseplan

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
)

// Repository reads committed repository objects without checking them out.
type Repository interface {
	Run(context.Context, ...string) ([]byte, error)
}

// Git reads repository objects through the shared process runner.
type Git struct{ Runner process.Runner }

// Run invokes Git with arguments kept separate from shell syntax.
func (g Git) Run(ctx context.Context, args ...string) ([]byte, error) {
	defer perf.Track(nil, "releaseplan.Git.Run")()

	runner := g.Runner
	if runner == nil {
		runner = process.NewDefaultRunner()
	}
	var output, diagnostic bytes.Buffer
	result := runner.Run(ctx, process.TaskSpec{Command: "git", Args: args, Streams: process.Streams{Stdout: &output, Stderr: &diagnostic}, SkipMetrics: true})
	if !result.Success() {
		return nil, errors.Join(fmt.Errorf("%w: git %s", errUtils.ErrPluginReleasePlan, diagnostic.String()), result.Err)
	}
	return output.Bytes(), nil
}
