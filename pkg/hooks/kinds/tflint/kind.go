package tflint

import (
	"context"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scanners"
	tflintscanner "github.com/cloudposse/atmos/pkg/scanners/tflint"
)

const kindName = tflintscanner.Name

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name:        kindName,
		Command:     tflintscanner.Command,
		DefaultArgs: tflintscanner.DefaultArgs(),
		OnFailure:   hooks.OnFailureWarn,
		Engine:      tflintEngine{},
	}); err != nil {
		panic("failed to register built-in tflint kind: " + err.Error())
	}
}

type tflintEngine struct{}

func (tflintEngine) Run(ctx *hooks.ExecContext) (*hooks.Output, error) {
	defer perf.Track(nil, "hooks.tflint.Run")()

	if ctx == nil || ctx.Hook == nil {
		return nil, errUtils.ErrNilParam
	}
	out, scan, err := tflintscanner.Run(context.Background(), &tflintscanner.Options{
		Args:          ctx.Hook.Args,
		Env:           ctx.Hook.Env,
		OnFailure:     ctx.Hook.OnFailure,
		AtmosConfig:   ctx.AtmosConfig,
		Info:          ctx.Info,
		ToolchainPATH: ctx.ToolchainPATH,
	})
	copyScanState(ctx, scan)
	return toHookOutput(out), err
}

func copyScanState(ctx *hooks.ExecContext, scan *scanners.Context) {
	if ctx == nil || scan == nil {
		return
	}
	ctx.OutputFile = scan.OutputFile
	ctx.OutputDir = scan.OutputDir
	ctx.ExitCode = scan.ExitCode
	ctx.CommandError = scan.CommandError
}

func toHookOutput(out *scanners.Output) *hooks.Output {
	if out == nil {
		return nil
	}
	return &hooks.Output{
		Artifact: toHookArtifact(out.Artifact),
		Summary:  toHookSummary(out.Summary),
	}
}

func toHookArtifact(a *scanners.Artifact) *hooks.Artifact {
	if a == nil {
		return nil
	}
	return &hooks.Artifact{
		Name:     a.Name,
		Body:     a.Body,
		Format:   a.Format,
		Metadata: a.Metadata,
	}
}

func toHookSummary(s *scanners.Summary) *hooks.Summary {
	if s == nil {
		return nil
	}
	return &hooks.Summary{
		Kind:     s.Kind,
		Status:   hooks.SummaryStatus(s.Status),
		Title:    s.Title,
		Counts:   s.Counts,
		Body:     s.Body,
		Findings: toHookFindings(s.Findings),
		SARIF:    s.SARIF,
	}
}

func toHookFindings(findings []scanners.Finding) []hooks.Finding {
	if len(findings) == 0 {
		return nil
	}
	out := make([]hooks.Finding, 0, len(findings))
	for _, f := range findings {
		out = append(out, hooks.Finding{
			Path:     f.Path,
			Line:     f.Line,
			Severity: f.Severity,
			RuleID:   f.RuleID,
			Message:  f.Message,
		})
	}
	return out
}
