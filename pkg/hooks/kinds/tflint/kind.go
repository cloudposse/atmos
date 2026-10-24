package tflint

import (
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/hooks/sarif"
	"github.com/cloudposse/atmos/pkg/perf"
	tflintscanner "github.com/cloudposse/atmos/pkg/scanners/tflint"
)

const kindName = tflintscanner.Name

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name:        kindName,
		Command:     tflintscanner.Command,
		DefaultArgs: tflintscanner.DefaultArgs(),
		OnFailure:   hooks.OnFailureWarn,
		// tflint emits SARIF to stdout with no file-output flag, so the engine
		// must capture stdout into ATMOS_OUTPUT_FILE for the ResultHandler
		// below to read — same mechanism trivy/checkov get for free by writing
		// their own output file.
		CaptureStdout: true,
		Engine:        tflintEngine{},
		ResultHandler: sarif.NewResultHandler(sarif.HandlerOptions{
			Kind:       kindName,
			OutputPath: sarif.DefaultOutputFile,
		}),
	}); err != nil {
		panic("failed to register built-in tflint kind: " + err.Error())
	}
}

// tflintEngine resolves tflint's dynamic `.tflint.hcl` config argument, then
// delegates to the shared CommandEngine — identical subprocess/PATH/env/CI
// handling as every other command-based hook kind (checkov/trivy/kics).
type tflintEngine struct{}

func (tflintEngine) Run(ctx *hooks.ExecContext) (*hooks.Output, error) {
	defer perf.Track(nil, "hooks.tflint.Run")()

	if ctx == nil || ctx.Hook == nil {
		return nil, errUtils.ErrNilParam
	}

	resolvedHook := *ctx.Hook
	resolvedHook.Args = tflintscanner.ResolveArgs(ctx.Hook.Args, ctx.AtmosConfig, ctx.Info)
	resolvedCtx := *ctx
	resolvedCtx.Hook = &resolvedHook

	return (&hooks.CommandEngine{}).Run(&resolvedCtx)
}
