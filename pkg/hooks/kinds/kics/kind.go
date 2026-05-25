// Package kics registers the built-in `kics` hook kind.
//
// KICS writes results to an output *directory* (`-o <dir>`) producing
// `results.sarif` (plus other files) — so the kind uses $ATMOS_OUTPUT_DIR
// and the ResultHandler reads `<dir>/results.sarif`. Findings flow through
// the shared SARIF parser.
package kics

import (
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/hooks/sarif"
)

const (
	kindName       = "kics"
	resultFileName = "results.sarif"
)

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name:    kindName,
		Command: "kics",
		DefaultArgs: []string{
			"scan",
			"-p", "$ATMOS_COMPONENT_PATH",
			"-o", "$ATMOS_OUTPUT_DIR",
			"--report-formats", "sarif",
			"--no-progress",
		},
		OnFailure: hooks.OnFailureWarn,
		Engine:    &hooks.CommandEngine{},
		ResultHandler: sarif.NewResultHandler(sarif.HandlerOptions{
			Kind: kindName,
			OutputPath: func(ctx *hooks.ExecContext) string {
				if ctx == nil || ctx.OutputDir == "" {
					return ""
				}
				return filepath.Join(ctx.OutputDir, resultFileName)
			},
		}),
	}); err != nil {
		panic("failed to register built-in kics kind: " + err.Error())
	}
}
