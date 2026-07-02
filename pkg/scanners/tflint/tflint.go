package tflint

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scanners"
	"github.com/cloudposse/atmos/pkg/scanners/sarif"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	Name    = "tflint"
	Command = "tflint"
)

func DefaultArgs() []string {
	defer perf.Track(nil, "tflint.DefaultArgs")()

	return []string{
		"--chdir=$ATMOS_COMPONENT_PATH",
		"--format=sarif",
	}
}

type Options struct {
	Args          []string
	Env           map[string]string
	BaseEnv       []string
	OnFailure     string
	AtmosConfig   *schema.AtmosConfiguration
	Info          *schema.ConfigAndStacksInfo
	ToolchainPATH string
}

func Run(ctx context.Context, opts *Options) (*scanners.Output, *scanners.Context, error) {
	defer perf.Track(nil, "scanners.tflint.Run")()

	if opts == nil {
		opts = &Options{}
	}
	args := opts.Args
	if len(args) == 0 {
		args = DefaultArgs()
	}

	scan := &scanners.Context{
		Name:          Name,
		Command:       Command,
		Args:          append([]string(nil), args...),
		Env:           opts.Env,
		BaseEnv:       opts.BaseEnv,
		OnFailure:     opts.OnFailure,
		CaptureStdout: true,
		AtmosConfig:   opts.AtmosConfig,
		Info:          opts.Info,
		ToolchainPATH: opts.ToolchainPATH,
		ResultHandler: sarif.NewResultHandler(sarif.HandlerOptions{
			Kind:       Name,
			OutputPath: sarif.DefaultOutputFile,
		}),
	}

	out, err := scanners.Run(ctx, scan)
	return out, scan, err
}
