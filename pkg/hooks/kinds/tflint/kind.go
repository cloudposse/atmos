// Package tflint registers the built-in `tflint` hook kind.
//
// Defaults invoke `tflint --chdir=$ATMOS_COMPONENT_PATH --format=sarif`.
// Unlike checkov/trivy/kics, tflint has no file-output flag — it writes SARIF
// to stdout — so the kind sets CaptureStdout and the engine redirects stdout
// into $ATMOS_OUTPUT_FILE. Findings then flow through the shared SARIF parser,
// rendering identically in the terminal, Atmos Pro, and PR comments, and
// (in CI) as a step summary, inline PR annotations, and a GitHub Code
// Scanning upload — all via the shared handler, no tflint-specific wiring.
package tflint

import (
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/hooks/sarif"
)

const kindName = "tflint"

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name:    kindName,
		Command: "tflint",
		DefaultArgs: []string{
			// --chdir points tflint at the component dir (the engine expands
			// $ATMOS_COMPONENT_PATH, which resolves to the provisioned workdir
			// when the workdir feature is enabled). tflint lints the builtin
			// terraform ruleset with no plugins, so no `tflint --init` is needed.
			"--chdir=$ATMOS_COMPONENT_PATH",
			"--format=sarif",
		},
		// tflint has no file-output flag: `--format=sarif` writes to stdout.
		CaptureStdout: true,
		OnFailure:     hooks.OnFailureWarn,
		Engine:        &hooks.CommandEngine{},
		ResultHandler: sarif.NewResultHandler(sarif.HandlerOptions{
			Kind:       kindName,
			OutputPath: sarif.DefaultOutputFile,
		}),
	}); err != nil {
		panic("failed to register built-in tflint kind: " + err.Error())
	}
}
