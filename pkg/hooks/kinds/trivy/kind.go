// Package trivy registers the built-in `trivy` hook kind.
//
// Defaults invoke `trivy config --format sarif --output $ATMOS_OUTPUT_FILE
// $ATMOS_COMPONENT_PATH`. Findings flow through the shared SARIF parser.
package trivy

import (
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/hooks/sarif"
)

const kindName = "trivy"

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name:    kindName,
		Command: "trivy",
		DefaultArgs: []string{
			"config",
			"--format", "sarif",
			"--output", "$ATMOS_OUTPUT_FILE",
			"--quiet",
			"$ATMOS_COMPONENT_PATH",
		},
		OnFailure: hooks.OnFailureWarn,
		Engine:    &hooks.CommandEngine{},
		ResultHandler: sarif.NewResultHandler(sarif.HandlerOptions{
			Kind:       kindName,
			OutputPath: sarif.DefaultOutputFile,
		}),
	}); err != nil {
		panic("failed to register built-in trivy kind: " + err.Error())
	}
}
